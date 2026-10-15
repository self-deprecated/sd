package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/model"
)

func TestBareSDFromProjectDirectoryUsesWorkspaceRootContext(t *testing.T) {
	root := t.TempDir()
	writeRootContextFixture(t, root)

	projectDir := filepath.Join(root, "projects", "app")
	var stdout, stderr bytes.Buffer
	code := Execute(Options{WorkDir: projectDir, Stdout: &stdout, Stderr: &stderr, NonInteractive: true})
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Projects:") || !strings.Contains(stdout.String(), "app") {
		t.Fatalf("expected workspace project list, got stdout=%q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "other") {
		t.Fatalf("expected all workspace projects, got stdout=%q", stdout.String())
	}
	if strings.Contains(stdout.String(), "Check app") {
		t.Fatalf("expected workspace root rather than project tasks, got stdout=%q", stdout.String())
	}
}

func TestBareInteractiveSDFromWorkspaceRootOpensWorkspacePicker(t *testing.T) {
	root := t.TempDir()
	writeRootContextFixture(t, root)

	oldInteractive := isTerminalInteractive
	oldPick := pickTasks
	defer func() {
		isTerminalInteractive = oldInteractive
		pickTasks = oldPick
	}()

	isTerminalInteractive = func(stdin io.Reader, stdout io.Writer) bool { return true }
	var pickedPrefix []string
	pickTasks = func(ws *model.Workspace, prefix []string, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
		pickedPrefix = append([]string{}, prefix...)
		return nil, false, nil
	}

	var stdout, stderr bytes.Buffer
	code := Execute(Options{WorkDir: root, Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr.String())
	}
	if len(pickedPrefix) != 0 {
		t.Fatalf("picker prefix = %#v, want empty", pickedPrefix)
	}
}

func TestBareInteractiveSDFromProjectDirectoryOpensWorkspacePicker(t *testing.T) {
	root := t.TempDir()
	writeRootContextFixture(t, root)

	oldInteractive := isTerminalInteractive
	oldPick := pickTasks
	defer func() {
		isTerminalInteractive = oldInteractive
		pickTasks = oldPick
	}()

	isTerminalInteractive = func(stdin io.Reader, stdout io.Writer) bool { return true }
	var pickedPrefix []string
	pickTasks = func(ws *model.Workspace, prefix []string, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
		pickedPrefix = append([]string{}, prefix...)
		return []string{"app", "check"}, true, nil
	}

	projectDir := filepath.Join(root, "projects", "app", "src")
	if err := os.MkdirAll(filepath.Join(projectDir), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Execute(Options{WorkDir: projectDir, Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr.String())
	}
	if len(pickedPrefix) != 0 {
		t.Fatalf("picker prefix = %#v, want empty workspace root", pickedPrefix)
	}
	if stdout.String() != "check" {
		t.Fatalf("stdout = %q, want check", stdout.String())
	}
}

func writeRootContextFixture(t *testing.T, root string) {
	t.Helper()
	writeRootContext(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	writeRootContext(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
  dir: .
tasks:
  check:
    desc: Check app
    run: printf check
`)
	writeRootContext(t, filepath.Join(root, "projects", "other", ".sd", "tasks.yaml"), `version: 1
project:
  id: other
  name: Other
tasks:
  check:
    desc: Check other
    run: printf other
`)
}

func writeRootContext(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
