// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// abandonAfter bounds how long Run waits for the interpreter after a second
// interrupt, since a builtin such as a terminal password read ignores
// cancellation.
const abandonAfter = time.Second

// Run executes script in dir with env as the complete environment and args
// as the positional parameters.
//
// A value received on interrupts acts like Ctrl-C in bash: running external
// commands stop with status 130 and every shell, including subshells and
// background jobs, exits with status 130 at its next command, running its
// EXIT trap. A second interrupt stops everything immediately.
func Run(interrupts <-chan os.Signal, dir, script string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "sd")
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	state := &interruptState{stop: stop, running: map[*runningCommand]bool{}}
	runner, err := interp.New(
		interp.CallHandler(state.exitWhenInterrupted),
		interp.ExecHandlers(state.track),
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(stdin, stdout, stderr),
		interp.Params(append([]string{"--"}, args...)...),
	)
	if err != nil {
		return err
	}

	result := make(chan error, 1)
	go func() { result <- runner.Run(ctx, file) }()
	var abandon <-chan time.Time
	for {
		select {
		case err := <-result:
			if state.wasInterrupted() {
				return interp.ExitStatus(130)
			}
			return err
		case <-interrupts:
			if state.interrupt() {
				abandon = time.After(abandonAfter)
			}
		case <-abandon:
			return interp.ExitStatus(130)
		}
	}
}

type runningCommand struct {
	cancel      context.CancelFunc
	interrupted bool
}

type interruptState struct {
	mu          sync.Mutex
	stop        context.CancelFunc
	running     map[*runningCommand]bool
	interrupted bool
}

// interrupt stops the running external commands on the first call and
// everything on later calls, which it reports by returning true.
func (s *interruptState) interrupt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted {
		s.stop()
		return true
	}
	s.interrupted = true
	for command := range s.running {
		command.interrupted = true
		command.cancel()
	}
	return false
}

func (s *interruptState) wasInterrupted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interrupted
}

// exitWhenInterrupted replaces each command after an interrupt with
// `exit 130`, so every shell exits through its own EXIT trap. Commands run by
// a trap are left alone so cleanup can finish.
func (s *interruptState) exitWhenInterrupted(_ context.Context, args []string) ([]string, error) {
	if s.wasInterrupted() && !inTrap() {
		return []string{"exit", "130"}, nil
	}
	return args, nil
}

// track gives each external command its own context so an interrupt can
// stop it, and reports a stopped command as exit status 130.
func (s *interruptState) track(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		command := &runningCommand{cancel: cancel}
		s.mu.Lock()
		s.running[command] = true
		s.mu.Unlock()

		err := next(ctx, args)

		s.mu.Lock()
		delete(s.running, command)
		interrupted := command.interrupted
		s.mu.Unlock()
		if interrupted {
			return interp.ExitStatus(130)
		}
		return err
	}
}

// inTrap reports whether the calling goroutine is running a trap. The
// interpreter has no public API for this, so it looks for the interpreter's
// trap callback on the stack; TestInterruptStopsTheWholeTask covers it.
func inTrap() bool {
	pcs := make([]uintptr, 512)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, "interp.(*Runner).trapCallback") {
			return true
		}
		if !more {
			return false
		}
	}
}
