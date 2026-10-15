package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/mirror"
)

type fakeVCS struct {
	status     mirror.VCSStatus
	isWorkcopy bool
}

func (f fakeVCS) IsJJWorkcopy(dir string) (bool, error)              { return f.isWorkcopy, nil }
func (f fakeVCS) Status(dir string) (mirror.VCSStatus, error)        { return f.status, nil }
func (f fakeVCS) Commit(dir string, message string) error            { return nil }
func (f fakeVCS) CurrentChange(dir string) (mirror.TraceInfo, error) { return mirror.TraceInfo{}, nil }

func TestGuardDirtyMirrorErrorsUnlessYesInNonInteractiveMode(t *testing.T) {
	opts := Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader(""), NonInteractive: true}
	err := guardDirtyMirror(fakeVCS{status: mirror.VCSStatus{Clean: false, Output: "dirty"}, isWorkcopy: true}, "demo", "/tmp/mirror", opts, &state{})
	if err == nil || !strings.Contains(err.Error(), "uncommitted local changes") {
		t.Fatalf("expected dirty mirror error, got %v", err)
	}

	err = guardDirtyMirror(fakeVCS{status: mirror.VCSStatus{Clean: false, Output: "dirty"}, isWorkcopy: true}, "demo", "/tmp/mirror", opts, &state{yes: true})
	if err != nil {
		t.Fatalf("--yes should continue: %v", err)
	}
}

func TestGuardDirtyMirrorInteractivePromptAcceptsAndRejects(t *testing.T) {
	old := isMirrorInteractive
	isMirrorInteractive = func(stdin io.Reader, stdout io.Writer) bool { return true }
	defer func() { isMirrorInteractive = old }()

	var stderr bytes.Buffer
	opts := Options{Stdout: &bytes.Buffer{}, Stderr: &stderr, Stdin: strings.NewReader("y\n")}
	err := guardDirtyMirror(fakeVCS{status: mirror.VCSStatus{Clean: false, Output: "dirty"}, isWorkcopy: true}, "demo", "/tmp/mirror", opts, &state{})
	if err != nil {
		t.Fatalf("interactive yes should continue: %v", err)
	}
	if !strings.Contains(stderr.String(), "Continue? [y/N]") {
		t.Fatalf("prompt missing from stderr: %q", stderr.String())
	}

	stderr.Reset()
	opts.Stdin = strings.NewReader("\n")
	err = guardDirtyMirror(fakeVCS{status: mirror.VCSStatus{Clean: false, Output: "dirty"}, isWorkcopy: true}, "demo", "/tmp/mirror", opts, &state{})
	if err == nil || !strings.Contains(err.Error(), "cancelled mirror demo sync") {
		t.Fatalf("interactive default should cancel, got %v", err)
	}
}

func TestGuardDirtyMirrorAllowsCleanStatus(t *testing.T) {
	opts := Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader(""), NonInteractive: true}
	err := guardDirtyMirror(fakeVCS{status: mirror.VCSStatus{Clean: true, Output: "clean"}, isWorkcopy: true}, "demo", "/tmp/mirror", opts, &state{})
	if err != nil {
		t.Fatalf("clean mirror should continue: %v", err)
	}
}

func TestValidateMirrorDestinationRequiresExistingJJWorkcopy(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := validateMirrorDestination(fakeVCS{isWorkcopy: true}, "demo", missing); err == nil || !strings.Contains(err.Error(), "destination does not exist") {
		t.Fatalf("expected missing destination error, got %v", err)
	}

	root := t.TempDir()
	filePath := filepath.Join(root, "file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateMirrorDestination(fakeVCS{isWorkcopy: true}, "demo", filePath); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not directory error, got %v", err)
	}

	dir := filepath.Join(root, "mirror")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateMirrorDestination(fakeVCS{isWorkcopy: false}, "demo", dir); err == nil || !strings.Contains(err.Error(), "not a jj workcopy") {
		t.Fatalf("expected not jj workcopy error, got %v", err)
	}

	if err := validateMirrorDestination(fakeVCS{isWorkcopy: true}, "demo", dir); err != nil {
		t.Fatalf("valid mirror destination: %v", err)
	}
}
