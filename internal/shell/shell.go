// Package shell runs task scripts with an embedded bash-compatible
// interpreter, so tasks behave the same on Linux, macOS, and Windows without
// a bash binary.
package shell

import (
	"context"
	"io"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Run executes script in dir with env as the complete environment and args
// as the positional parameters.
func Run(ctx context.Context, dir, script string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
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
	err = runner.Run(ctx, file)
	if ctx.Err() == nil {
		return err
	}
	// The interpreter skips the EXIT trap once ctx is cancelled. Exit as bash
	// does on SIGINT so cleanup traps still run.
	interrupted, _ := syntax.NewParser().Parse(strings.NewReader("exit 130"), "sd")
	return runner.Run(context.WithoutCancel(ctx), interrupted)
}
