package shell

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary doubles as a portable external command for scripts:
// `SD_SHELL_HELPER=1 "$TESTBIN" sleep <seconds>`.
func TestMain(m *testing.M) {
	if os.Getenv("SD_SHELL_HELPER") == "1" && len(os.Args) == 3 && os.Args[1] == "sleep" {
		d, _ := time.ParseDuration(os.Args[2] + "s")
		time.Sleep(d)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func sleepCmd(seconds string) string {
	return fmt.Sprintf(`SD_SHELL_HELPER=1 %q sleep %s`, os.Args[0], seconds)
}

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

func run(t *testing.T, script string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Run(nil, t.TempDir(), script, nil, os.Environ(), nil, &out, &out)
	return out.String(), err
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
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(out.String(), marker) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q; output %q", marker, out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	interrupts <- os.Interrupt
}

func waitInterrupted(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if err == nil || err.Error() != "exit status 130" {
			t.Fatalf("err = %v, want exit status 130", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("script did not stop")
	}
}

func TestInterruptStopsRunningCommandAndRunsExitTrapOnce(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf trapped' EXIT; printf ready; `+sleepCmd("30")+`; printf after`, interrupts, out)
	interruptWhen(t, out, "ready", interrupts)
	waitInterrupted(t, result)
	if got := out.String(); got != "readytrapped" {
		t.Fatalf("output = %q", got)
	}
}

func TestInterruptStopsTheWholeTask(t *testing.T) {
	sleep := sleepCmd("30")
	cases := []struct{ name, script, marker, out string }{
		{"builtin loop", `trap 'printf CLEAN' EXIT; printf ready; while true; do :; done; printf AFTER`, "ready", "readyCLEAN"},
		{"subshell", `trap 'printf CLEAN' EXIT; printf ready; (` + sleep + `); printf AFTER; ` + sleep, "ready", "readyCLEAN"},
		{"background job and wait", `trap 'printf CLEAN' EXIT; ` + sleep + ` & printf ready; wait; printf AFTER`, "ready", "readyCLEAN"},
		{"background job and builtin loop", sleep + ` & printf ready; while true; do :; done`, "ready", "ready"},
		{"trap sees status 130", `trap 'printf "[%s]" "$?"' EXIT; printf ready; ` + sleep, "ready", "ready[130]"},
		{"trap calls a function", `cleanup() { printf cleaned; ` + sleepCmd("0") + `; printf done; }; trap cleanup EXIT; printf ready; ` + sleep, "ready", "readycleaneddone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &syncBuffer{}
			interrupts := make(chan os.Signal, 2)
			result := runAsync(t, tc.script, interrupts, out)
			interruptWhen(t, out, tc.marker, interrupts)
			waitInterrupted(t, result)
			if got := out.String(); got != tc.out {
				t.Fatalf("output = %q, want %q", got, tc.out)
			}
		})
	}
}

// blockingReader never returns, like a terminal read that ignores cancellation.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }

func TestSecondInterruptReturnsEvenIfABuiltinIgnoresCancellation(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := make(chan error, 1)
	go func() {
		result <- Run(interrupts, t.TempDir(), `printf ready; read -r value; printf after`, nil, os.Environ(), blockingReader{}, out, out)
	}()
	interruptWhen(t, out, "ready", interrupts)
	interrupts <- os.Interrupt
	waitInterrupted(t, result)
	if got := out.String(); got != "ready" {
		t.Fatalf("output = %q", got)
	}
}

func TestInterruptDuringExitTrapDoesNotReplayIt(t *testing.T) {
	for _, registration := range []string{"trap %s EXIT", "trap %s EXIT ERR", "trap -- %s EXIT", "builtin trap %s EXIT"} {
		for _, ending := range []string{"printf body", "printf body; exit 0"} {
			script := fmt.Sprintf(registration, `'printf start; while true; do :; done'`) + "; " + ending
			t.Run(script, func(t *testing.T) {
				out := &syncBuffer{}
				interrupts := make(chan os.Signal, 2)
				result := runAsync(t, script, interrupts, out)
				// A trap is allowed to finish after one interrupt; a second stops it.
				interruptWhen(t, out, "start", interrupts)
				interrupts <- os.Interrupt
				waitInterrupted(t, result)
				if got := out.String(); got != "bodystart" {
					t.Fatalf("output = %q, want the trap to run once", got)
				}
			})
		}
	}
}

func TestSecondInterruptStopsExitTrap(t *testing.T) {
	out := &syncBuffer{}
	interrupts := make(chan os.Signal, 2)
	result := runAsync(t, `trap 'printf cleanup; while true; do :; done' EXIT; printf ready; `+sleepCmd("30"), interrupts, out)
	interruptWhen(t, out, "ready", interrupts)
	interruptWhen(t, out, "cleanup", interrupts)
	waitInterrupted(t, result)
	if got := out.String(); got != "readycleanup" {
		t.Fatalf("output = %q", got)
	}
}

func TestExitTrapSemantics(t *testing.T) {
	cases := []struct {
		name, script, out, err string
	}{
		{"normal exit", `trap 'printf trapped' EXIT; printf body`, "bodytrapped", ""},
		{"exit builtin", `trap 'printf "[trap $?]"' EXIT; printf body; exit 3; printf after`, "body[trap 3]", "exit status 3"},
		{"errexit", `set -e; trap 'printf "[trap $?]"' EXIT; false; printf after`, "[trap 1]", "exit status 1"},
		{"failing last command", `trap 'printf "[trap $?]"' EXIT; false`, "[trap 1]", "exit status 1"},
		{"reset trap", `trap 'printf trapped' EXIT; trap - EXIT; printf body`, "body", ""},
		{"saved and restored trap", `trap 'printf cleanup' EXIT; trap > saved; trap - EXIT; . ./saved; printf body`, "bodycleanup", ""},
		{"background subshell trap keeps parent trap", `trap 'printf parent' EXIT; { trap 'printf child' EXIT; exit; } & wait; printf body`, "childbodyparent", ""},
		{"background job survives until exit trap", `{ ` + sleepCmd("0.2") + `; printf done; } & pid=$!; trap 'wait "$pid"; printf ":%s" "$?"' EXIT`, "done:0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := run(t, tc.script)
			if got != tc.out {
				t.Fatalf("output = %q, want %q", got, tc.out)
			}
			if (err == nil && tc.err != "") || (err != nil && err.Error() != tc.err) {
				t.Fatalf("err = %v, want %q", err, tc.err)
			}
		})
	}
}
