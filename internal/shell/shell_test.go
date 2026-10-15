package shell

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestRunStopsOnCancelAndRunsExitTrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	err := Run(ctx, t.TempDir(), `trap 'printf trapped' EXIT; while true; do :; done; printf after`, nil, os.Environ(), nil, &out, &out)
	if err == nil || err.Error() != "exit status 130" {
		t.Fatalf("err = %v, want exit status 130", err)
	}
	if got := out.String(); got != "trapped" {
		t.Fatalf("output = %q, want EXIT trap output only", got)
	}
}

func TestRunRunsExitTrapOnceOnNormalExit(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), `trap 'printf trapped' EXIT; printf body`, nil, os.Environ(), nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "bodytrapped" {
		t.Fatalf("output = %q", got)
	}
}
