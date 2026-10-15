package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func flagsWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  dir: .
tasks:
  greet:
    run: printf 'hello %s' "{{name}}"
    options:
      name: {type: string, default: world}
  plain:
    run: printf '%s,' "$@"
  fail:
    run: exit 3
  mode:
    defaultMode: wet
    modes:
      wet: {values: {kind: WET}}
      dry: {values: {kind: DRY}}
    run: printf '{{kind}}'
`)
	return root
}

func TestFlagsNeedValues(t *testing.T) {
	root := flagsWorkspace(t)
	for _, args := range [][]string{{"app", "mode", "--mode"}, {"app", "plain", "--log-file"}, {"app", "greet", "--var"}} {
		_, stderr, code := execute(t, root, args...)
		if code != 2 || !strings.Contains(stderr, "needs a value") && !strings.Contains(stderr, "name=value") {
			t.Fatalf("%v: code = %d stderr = %q, want a missing-value usage error", args, code, stderr)
		}
	}
}

func TestVarWorksAnywhereBeforeDoubleDash(t *testing.T) {
	root := flagsWorkspace(t)
	for _, args := range [][]string{{"--var", "name=you", "app", "greet"}, {"app", "greet", "--var", "name=you"}, {"app", "--var=name=you", "greet"}} {
		stdout, stderr, code := execute(t, root, args...)
		if code != 0 || stdout != "hello you" {
			t.Fatalf("%v: stdout = %q code = %d stderr = %q", args, stdout, code, stderr)
		}
	}
}

func TestUndeclaredVarIsRejected(t *testing.T) {
	root := flagsWorkspace(t)
	_, stderr, code := execute(t, root, "app", "plain", "--var", "x=1")
	if code != 2 || !strings.Contains(stderr, `no runtime variable "x"`) {
		t.Fatalf("code = %d stderr = %q", code, stderr)
	}
	stdout, _, code := execute(t, root, "app", "plain", "a", "--", "--var", "x=1")
	if code != 0 || stdout != "a,--var,x=1," {
		t.Fatalf("args after -- must reach the task: stdout = %q code = %d", stdout, code)
	}
}

func TestTaskExitStatusBecomesSdExitStatus(t *testing.T) {
	root := flagsWorkspace(t)
	_, stderr, code := execute(t, root, "app", "fail")
	if code != 3 || !strings.Contains(stderr, "exit status 3") {
		t.Fatalf("code = %d stderr = %q, want 3", code, stderr)
	}
}
