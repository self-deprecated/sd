// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const (
	// killAfter is how long a command has to exit after the interrupt
	// signal before it is killed.
	killAfter = 2 * time.Second
	// abandonAfter bounds how long Run waits for the interpreter to stop
	// after an interrupt, since a builtin such as a silent terminal read
	// ignores cancellation. It outlasts killAfter so commands are killed
	// before Run returns.
	abandonAfter = killAfter + time.Second
)

// Run executes script in dir with env as the complete environment and args
// as the positional parameters.
//
// A value received on interrupts stops the whole script at once, including
// subshells, background jobs, and a running EXIT trap, and Run returns exit
// status 130. EXIT traps do not run after an interrupt.
func Run(interrupts <-chan os.Signal, dir, script string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "sd")
	if err != nil {
		return err
	}
	commands := newCommandTracker()
	runner, err := interp.New(
		interp.ExecHandlers(commands.track),
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(stdin, stdout, stderr),
		interp.Params(append([]string{"--"}, args...)...),
	)
	if err != nil {
		return err
	}
	restoreTerminal := saveTerminal(stdin)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	result := make(chan error, 1)
	go func() { result <- runner.Run(ctx, file) }()
	select {
	case err := <-result:
		return err
	case <-interrupts:
		stop()
	}
	// Background commands can outlive the interpreter, so wait for both.
	stopped := make(chan struct{})
	go func() {
		<-result
		<-commands.idle()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(abandonAfter):
		restoreTerminal()
	}
	return interp.ExitStatus(130)
}

// commandTracker counts the external commands that are running.
type commandTracker struct {
	mu      sync.Mutex
	running int
	idleCh  chan struct{} // closed while no command is running
}

func newCommandTracker() *commandTracker {
	idle := make(chan struct{})
	close(idle)
	return &commandTracker{idleCh: idle}
}

// track runs commands with the default handler, which interrupts them on
// cancellation and kills them killAfter later.
func (c *commandTracker) track(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	run := interp.DefaultExecHandler(killAfter)
	return func(ctx context.Context, args []string) error {
		c.mu.Lock()
		if c.running == 0 {
			c.idleCh = make(chan struct{})
		}
		c.running++
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			c.running--
			if c.running == 0 {
				close(c.idleCh)
			}
			c.mu.Unlock()
		}()
		if script, ok := shellScript(ctx, args[0]); ok {
			return c.runScript(ctx, script, args)
		}
		return run(ctx, args)
	}
}

// shellScript reads the file a command name resolves to if the kernel would
// refuse to execute it (ENOEXEC): a file without a "#!" line that is not a
// binary. Other shells, and the interpreter's own fallback, run such files
// as shell scripts; sd does so itself so their commands are tracked too.
func shellScript(ctx context.Context, name string) ([]byte, bool) {
	if runtime.GOOS == "windows" {
		return nil, false
	}
	hc := interp.HandlerCtx(ctx)
	path, err := interp.LookPathDir(hc.Dir, hc.Env, name)
	if err != nil {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	header := make([]byte, 512)
	n, _ := io.ReadFull(file, header)
	header = header[:n]
	// Like bash, treat a null byte before the first newline as a binary.
	line, _, _ := bytes.Cut(header, []byte("\n"))
	if bytes.HasPrefix(header, []byte("#!")) || bytes.IndexByte(line, 0) >= 0 {
		return nil, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return content, true
}

// runScript runs a script in a new shell that inherits only exported
// variables, as other shells do, with the same command tracking.
func (c *commandTracker) runScript(ctx context.Context, script []byte, args []string) error {
	hc := interp.HandlerCtx(ctx)
	file, err := syntax.NewParser().Parse(bytes.NewReader(script), args[0])
	if err != nil {
		fmt.Fprintln(hc.Stderr, err)
		return interp.ExitStatus(2)
	}
	// Later bindings override earlier ones; unset removes an inherited one.
	exported := map[string]string{}
	for name, vr := range hc.Env.Each {
		if !vr.IsSet() {
			delete(exported, name)
		} else if vr.Exported && vr.Kind == expand.String {
			exported[name] = vr.String()
		}
	}
	var env []string
	for name, value := range exported {
		env = append(env, name+"="+value)
	}
	runner, err := interp.New(
		interp.ExecHandlers(c.track),
		interp.Dir(hc.Dir),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(hc.Stdin, hc.Stdout, hc.Stderr),
		interp.Params(append([]string{"--"}, args[1:]...)...),
	)
	if err != nil {
		return err
	}
	return runner.Run(ctx, file)
}

func (c *commandTracker) idle() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.idleCh
}

// saveTerminal records the state of stdin if it is a terminal and returns a
// function that restores it.
func saveTerminal(stdin io.Reader) func() {
	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return func() {}
	}
	state, err := term.GetState(int(file.Fd()))
	if err != nil {
		return func() {}
	}
	return func() { _ = term.Restore(int(file.Fd()), state) }
}
