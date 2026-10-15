package cli

import (
	"bytes"
	"testing"

	"github.com/self-deprecated/sd/internal/mirror"
)

func TestRunMirrorChecksRunsWithoutBashOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	plan := mirror.Plan{WorkspaceRoot: t.TempDir(), Checks: []string{`[[ -d . ]] && printf first`, `printf ' second'`}}
	if err := runMirrorChecks(plan, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("runMirrorChecks: %v (output %q)", err, out.String())
	}
	if out.String() != "first second" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunMirrorChecksStopsAtFirstFailure(t *testing.T) {
	var out bytes.Buffer
	plan := mirror.Plan{WorkspaceRoot: t.TempDir(), Checks: []string{`exit 4`, `printf unreachable`}}
	err := runMirrorChecks(plan, Options{Stdout: &out, Stderr: &out})
	if err == nil || err.Error() != "exit status 4" || out.Len() != 0 {
		t.Fatalf("err = %v output = %q", err, out.String())
	}
}
