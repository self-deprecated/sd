// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"io"
	"os"
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
		return run(ctx, args)
	}
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
