package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/self-deprecated/sd/internal/model"
)

// Tasks must not depend on a bash binary so they run the same on Windows.
func TestRunProjectTaskRunsWithoutBashOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "demo", Path: root, Scripts: map[string]model.SharedScript{
		"context": {Run: `export GREETING=hello; greet() { printf '%s %s' "$GREETING" "$1"; }`},
	}}
	task := model.Task{ID: "greet", Scripts: []string{"context"}, Run: `set -euo pipefail; greet "$1"; [[ $# -eq 2 ]] && printf ' (%s args)' "$#"`}

	var out bytes.Buffer
	if err := RunProjectTask(ws, project, task, []string{"world", "extra"}, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v (output %q)", err, out.String())
	}
	if got := out.String(); got != "hello world (2 args)" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunProjectTaskReportsExitStatus(t *testing.T) {
	root := t.TempDir()
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "demo", Path: filepath.Join(root)}
	err := RunProjectTask(ws, project, model.Task{ID: "fail", Run: "exit 3"}, nil, Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("err = %v, want exit status 3", err)
	}
}

func TestRunShellTurnsInterruptIntoExitTrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot send os.Interrupt to a process on Windows")
	}
	out := &lockedBuffer{}
	result := make(chan error, 1)
	go func() {
		result <- runShell(t.TempDir(), `trap 'printf cleaned' EXIT; printf ready; while true; do :; done`, nil, nil, Options{Stdout: out, Stderr: out})
	}()
	// runShell registers for os.Interrupt before the script prints ready.
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(out.String(), "ready"); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("script never became ready; output %q", out.String())
		}
	}
	self, _ := os.FindProcess(os.Getpid())
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil || err.Error() != "exit status 130" {
			t.Fatalf("err = %v, want exit status 130", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task did not stop after interrupt")
	}
	if out.String() != "readycleaned" {
		t.Fatalf("output = %q", out.String())
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
