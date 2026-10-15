// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Run executes script in dir with env as the complete environment and args
// as the positional parameters. A value received on interrupts stops the
// script.
func Run(interrupts <-chan os.Signal, dir, script string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "sd")
	if err != nil {
		return err
	}
	var trapStarted atomic.Bool
	runner, err := interp.New(
		interp.CallHandler(markExitTrap(&trapStarted)),
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(stdin, stdout, stderr),
		interp.Params(append([]string{"--"}, args...)...),
	)
	if err != nil {
		return err
	}
	marker, _ := syntax.NewParser().Parse(strings.NewReader(exitTrapMarker+`() { return "$1"; }`), "sd")
	if err := runner.Run(context.Background(), marker.Stmts[0]); err != nil {
		return err
	}
	// The first interrupt stops the script; the next one stops its EXIT trap.
	scriptCtx, stopScript := context.WithCancel(context.Background())
	defer stopScript()
	trapCtx, stopTrap := context.WithCancel(context.Background())
	defer stopTrap()
	var interrupted atomic.Bool
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-interrupts:
				interrupted.Store(true)
				if scriptCtx.Err() == nil {
					stopScript()
					continue
				}
				stopTrap()
				return
			case <-done:
				return
			}
		}
	}()

	for _, stmt := range file.Stmts {
		err = runner.Run(scriptCtx, stmt)
		if runner.Exited() || scriptCtx.Err() != nil {
			break
		}
	}
	if trapStarted.Load() || (runner.Exited() && !interrupted.Load()) {
		// exit or errexit already ran the EXIT trap; never run it twice.
		return interruptedStatus(&interrupted, err)
	}
	stopScript()
	code := 130
	if !interrupted.Load() {
		code = exitCode(err)
	}
	exit, _ := syntax.NewParser().Parse(strings.NewReader(fmt.Sprintf("exit %d", code)), "sd")
	trapErr := runner.Run(trapCtx, exit)
	if _, ok := interp.IsExitStatus(err); err != nil && !ok && !interrupted.Load() {
		return err
	}
	return interruptedStatus(&interrupted, trapErr)
}

const exitTrapMarker = "__sd_exit_trap_started"

// markExitTrap prefixes EXIT traps with a call to the marker function so Run
// can tell whether the trap has started. The marker returns the status it is
// given, so the trap still sees the script's $?.
func markExitTrap(started *atomic.Bool) interp.CallHandlerFunc {
	return func(_ context.Context, args []string) ([]string, error) {
		switch {
		case args[0] == exitTrapMarker:
			started.Store(true)
		case len(args) == 3 && args[0] == "trap" && args[2] == "EXIT" && args[1] != "" && args[1] != "-":
			return []string{"trap", exitTrapMarker + ` "$?"` + "\n" + args[1], "EXIT"}, nil
		}
		return args, nil
	}
}

// interruptedStatus reports 130, as bash does after SIGINT, once interrupted.
func interruptedStatus(interrupted *atomic.Bool, err error) error {
	if interrupted.Load() {
		return interp.ExitStatus(130)
	}
	return err
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if status, ok := interp.IsExitStatus(err); ok {
		return int(status)
	}
	return 1
}
