package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/tui"
)

func TestMirrorInteractivePickerChoosesMirrorAndAction(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(root, "mirrors")
	writeInteractiveMirrorFixture(t, root)
	writeInteractive(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	writeInteractive(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	writeInteractive(t, filepath.Join(mirrorRoot, "demo", ".jj", "repo"), "repo")
	installInteractiveFakeJJ(t, root)

	oldInteractive := isMirrorInteractive
	isMirrorInteractive = func(stdin io.Reader, stdout io.Writer) bool { return true }
	defer func() { isMirrorInteractive = oldInteractive }()
	oldPicker := pickMirrorChoices
	pickMirrorChoices = func(title string, choices []tui.Choice, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
		if len(choices) == 0 {
			return nil, false, nil
		}
		return choices[0].Args, true, nil
	}
	defer func() { pickMirrorChoices = oldPicker }()

	stdout, stderr, code := executeMirrorInteractive(root, "", "mirror", "--mirror-root", mirrorRoot, "plan")
	if code != 0 {
		t.Fatalf("interactive plan code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "demo") || !strings.Contains(stdout, "create README.md") {
		t.Fatalf("interactive plan did not pick mirror: %q", stdout)
	}

	stdout, stderr, code = executeMirrorInteractive(root, "", "mirror", "--mirror-root", mirrorRoot)
	if code != 0 {
		t.Fatalf("interactive bare mirror code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "create README.md") {
		t.Fatalf("interactive bare mirror did not run picked plan action: %q", stdout)
	}
}

func executeMirrorInteractive(dir, input string, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Execute(Options{Args: args, WorkDir: dir, Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(input)})
	return stdout.String(), stderr.String(), code
}

func writeInteractiveMirrorFixture(t *testing.T, root string) {
	t.Helper()
	writeInteractive(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
workspace:
  id: test-root
discover:
  - projects/*/.sd/tasks.yaml
`)
	writeInteractive(t, filepath.Join(root, ".sd", "mirrors.yaml"), `version: 1
mirrors:
  demo:
    source: projects/demo
    destination:
      root: public-mirrors
      path: demo
    visibility: public-code
    include: ["**"]
    exclude:
      - CONTEXT.public.md
    overlays:
      - from: CONTEXT.public.md
        to: CONTEXT.md
`)
	writeInteractive(t, filepath.Join(root, "projects", "demo", ".sd", "tasks.yaml"), `version: 1
project:
  id: demo
  name: Demo
tasks:
  check:
    desc: Check
    run: true
`)
}

func writeInteractive(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installInteractiveFakeJJ(t *testing.T, root string) {
	t.Helper()
	bin := filepath.Join(root, "bin-interactive")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "jj")
	content := `#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  root)
    pwd
    ;;
  st)
    echo "The working copy has no changes."
    ;;
  log)
    echo "abc def"
    ;;
esac
`
	if err := os.WriteFile(fake, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
