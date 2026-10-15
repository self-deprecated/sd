package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectSharedScriptsRunInTaskShell(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
  dir: .
scripts:
  base:
    desc: Establish base context
    run: |
      export SHARED_ORDER=base
      shared_value() { printf '%s' "$SHARED_ORDER"; }
  choice:
    run: export SHARED_ORDER="${SHARED_ORDER}-choice"
  mode:
    run: export SHARED_ORDER="${SHARED_ORDER}-mode"
  fail:
    run: return 7
  marker:
    run: printf touched > shared-script-marker
tasks:
  composed:
    scripts: [base]
    run: shared_value
    modes:
      release:
        scripts: [mode, base]
    choices:
      selected:
        scripts: [choice, base]
  failing:
    scripts: [fail]
    run: printf should-not-run
  dangerous:
    danger: true
    scripts: [marker]
    run: printf danger
`)

	t.Run("task choice and mode scripts share exports and preserve order", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "composed", "selected", "--mode", "release")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "base-choice-mode" {
			t.Fatalf("stdout = %q, want base-choice-mode", stdout)
		}
	})

	t.Run("explain names scripts without executing them", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "explain", "app", "composed", "selected", "--mode", "release")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "scripts:     base, choice, mode") {
			t.Fatalf("explain did not list composed scripts: %q", stdout)
		}
	})

	t.Run("script failure stops the task", func(t *testing.T) {
		stdout, _, code := execute(t, root, "app", "failing")
		if code == 0 {
			t.Fatal("failing shared script unexpectedly succeeded")
		}
		if strings.Contains(stdout, "should-not-run") {
			t.Fatalf("task ran after shared script failure: %q", stdout)
		}
	})

	t.Run("danger confirmation happens before shared scripts", func(t *testing.T) {
		marker := filepath.Join(root, "shared-script-marker")
		_, stderr, code := execute(t, root, "app", "dangerous")
		if code != 2 || !strings.Contains(stderr, "requires --yes") {
			t.Fatalf("expected danger refusal, code=%d stderr=%q", code, stderr)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("shared script ran before danger confirmation: %v", err)
		}
	})
}

func TestUnknownProjectSharedScriptFailsManifestLoading(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "tasks.yaml"), `version: 1
project:
  id: app
tasks:
  build:
    scripts: [missing]
    run: printf build
`)

	_, stderr, code := execute(t, root, "app", "build")
	if code == 0 || !strings.Contains(stderr, `references unknown shared script "missing"`) {
		t.Fatalf("expected unknown script error, code=%d stderr=%q", code, stderr)
	}
}
