// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// abandonAfter bounds how long Run waits for the interpreter to stop after an
// interrupt, since a builtin such as a silent terminal read ignores
// cancellation.
const abandonAfter = time.Second

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
	runner, err := interp.New(
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
	select {
	case <-result:
	case <-time.After(abandonAfter):
		restoreTerminal()
	}
	return interp.ExitStatus(130)
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
