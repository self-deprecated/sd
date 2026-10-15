// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Run executes script in dir with env as the complete environment and args
// as the positional parameters.
//
// A value received on interrupts acts like Ctrl-C in bash: the external
// commands running at that moment are stopped and the shell exits through
// its EXIT trap. If only builtins are running, or on a second interrupt, the
// script stops immediately. An interrupted script returns exit status 130.
func Run(interrupts <-chan os.Signal, dir, script string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "sd")
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	commands := &runningCommands{stop: stop, running: map[*runningCommand]bool{}}
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

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-interrupts:
				commands.interrupt()
			case <-done:
				return
			}
		}
	}()

	err = runner.Run(ctx, file)
	if commands.wasInterrupted() {
		return interp.ExitStatus(130)
	}
	return err
}

// errInterrupted halts the interpreter, which then runs the EXIT trap.
var errInterrupted = errors.New("interrupted")

type runningCommand struct {
	cancel      context.CancelFunc
	interrupted bool
}

// runningCommands tracks the external commands the interpreter is running.
type runningCommands struct {
	mu          sync.Mutex
	stop        context.CancelFunc
	running     map[*runningCommand]bool
	interrupted bool
}

func (c *runningCommands) interrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.interrupted || len(c.running) == 0 {
		c.interrupted = true
		c.stop()
		return
	}
	c.interrupted = true
	for command := range c.running {
		command.interrupted = true
		command.cancel()
	}
}

func (c *runningCommands) wasInterrupted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interrupted
}

// track reports a command stopped by an interrupt as errInterrupted, so the
// shell running it exits instead of continuing with the next command.
func (c *runningCommands) track(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		command := &runningCommand{cancel: cancel}
		c.mu.Lock()
		c.running[command] = true
		c.mu.Unlock()

		err := next(ctx, args)

		c.mu.Lock()
		delete(c.running, command)
		interrupted := command.interrupted
		c.mu.Unlock()
		if interrupted {
			return errInterrupted
		}
		return err
	}
}
