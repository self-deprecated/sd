package shell

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets a test read output while the interpreter writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runAsync runs script and returns a channel with its result.
func runAsync(t *testing.T, script string, interrupts chan os.Signal, out *syncBuffer) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- Run(interrupts, t.TempDir(), script, nil, os.Environ(), nil, out, out) }()
	return result
}

// interruptWhen sends an interrupt once out contains marker.
func interruptWhen(t *testing.T, out *syncBuffer, marker string, interrupts chan os.Signal) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), marker) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q; output %q", marker, out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	interrupts <- os.Interrupt
}

func waitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("script did not stop")
		return nil
	}
}

func TestRunStopsOnInterruptAndRunsExitTrap(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf trapped' EXIT; printf ready; while true; do :; done; printf after`, interrupts, out)
	interruptWhen(t, out, "ready", interrupts)
	if err := waitResult(t, result); err == nil || err.Error() != "exit status 130" {
		t.Fatalf("err = %v, want exit status 130", err)
	}
	if got := out.String(); got != "readytrapped" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunRunsExitTrapOnceOnNormalExit(t *testing.T) {
	var out bytes.Buffer
	if err := Run(nil, t.TempDir(), `trap 'printf trapped' EXIT; printf body`, nil, os.Environ(), nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "bodytrapped" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunDoesNotReplayExitTrapInterruptedDuringCleanup(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf start; while true; do :; done' EXIT; printf body`, interrupts, out)
	interruptWhen(t, out, "start", interrupts)
	if err := waitResult(t, result); err == nil || err.Error() != "exit status 130" {
		t.Fatalf("err = %v, want exit status 130", err)
	}
	if got := out.String(); got != "bodystart" {
		t.Fatalf("output = %q, want the trap to run once", got)
	}
}

func TestRunSecondInterruptStopsExitTrap(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf cleanup; while true; do :; done' EXIT; printf ready; while true; do :; done`, interrupts, out)
	interruptWhen(t, out, "ready", interrupts)
	interruptWhen(t, out, "cleanup", interrupts)
	if err := waitResult(t, result); err == nil || err.Error() != "exit status 130" {
		t.Fatalf("err = %v, want exit status 130", err)
	}
	if got := out.String(); got != "readycleanup" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunExitTrapRunsOnceWithScriptStatus(t *testing.T) {
	cases := []struct {
		name, script, out, err string
	}{
		{"exit builtin", `trap 'printf "[trap $?]"' EXIT; printf body; exit 3; printf after`, "body[trap 3]", "exit status 3"},
		{"errexit", `set -e; trap 'printf "[trap $?]"' EXIT; false; printf after`, "[trap 1]", "exit status 1"},
		{"failing last command", `trap 'printf "[trap $?]"' EXIT; false`, "[trap 1]", "exit status 1"},
		{"reset trap", `trap 'printf trapped' EXIT; trap - EXIT; printf body`, "body", ""},
		{"no trap", `printf body`, "body", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Run(nil, t.TempDir(), tc.script, nil, os.Environ(), nil, &out, &out)
			if got := out.String(); got != tc.out {
				t.Fatalf("output = %q, want %q", got, tc.out)
			}
			if (err == nil && tc.err != "") || (err != nil && err.Error() != tc.err) {
				t.Fatalf("err = %v, want %q", err, tc.err)
			}
		})
	}
}

func TestRunInterruptDuringTrapStartedByExitDoesNotReplayIt(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf start; while true; do :; done' EXIT; exit 4`, interrupts, out)
	interruptWhen(t, out, "start", interrupts)
	if err := waitResult(t, result); err == nil || err.Error() != "exit status 130" {
		t.Fatalf("err = %v, want exit status 130", err)
	}
	if got := out.String(); got != "start" {
		t.Fatalf("output = %q, want the trap to run once", got)
	}
}
