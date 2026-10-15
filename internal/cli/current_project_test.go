package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func currentProjectWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), "version: 1\ndiscover:\n  - projects/*/.sd/tasks.yaml\n")
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), "version: 1\nproject: {id: app}\ntasks:\n  test: {desc: App tests, run: printf app-test}\n  build/web: {run: printf app-web}\n")
	write(t, filepath.Join(root, "projects", "web", ".sd", "tasks.yaml"), "version: 1\nproject: {id: web}\ntasks:\n  test: {desc: Web tests, run: printf web-test}\n")
	return root
}

func TestTasksResolveInTheCurrentProject(t *testing.T) {
	root := currentProjectWorkspace(t)
	inApp := filepath.Join(root, "projects", "app")
	cases := []struct {
		dir    string
		args   []string
		stdout string
	}{
		{inApp, []string{"test"}, "app-test"},
		{filepath.Join(inApp, ".sd"), []string{"build", "web"}, "app-web"},
		{inApp, []string{"web", "test"}, "web-test"},
		{inApp, []string{"app", "test"}, "app-test"},
	}
	for _, tc := range cases {
		stdout, stderr, code := execute(t, tc.dir, tc.args...)
		if code != 0 || stdout != tc.stdout {
			t.Fatalf("%v in %s: stdout = %q code = %d stderr = %q", tc.args, tc.dir, stdout, code, stderr)
		}
	}
	if _, stderr, code := execute(t, root, "test"); code != 2 || !strings.Contains(stderr, "unknown project: test") {
		t.Fatalf("no project is inferred at the workspace root: code = %d stderr = %q", code, stderr)
	}
}

func TestStandaloneProjectNeedsNoProjectName(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "tasks.yaml"), "version: 1\nproject: {id: tool, dir: .}\ntasks:\n  check: {run: printf checked}\n")
	stdout, stderr, code := execute(t, filepath.Join(root, ".sd"), "check")
	if code != 0 || stdout != "checked" {
		t.Fatalf("stdout = %q code = %d stderr = %q", stdout, code, stderr)
	}
}

func TestExplainAndTasksUseTheCurrentProject(t *testing.T) {
	root := currentProjectWorkspace(t)
	inApp := filepath.Join(root, "projects", "app")
	stdout, stderr, code := execute(t, inApp, "explain", "test", "--json")
	if code != 0 || !strings.Contains(stdout, `"project":"app"`) {
		t.Fatalf("explain: stdout = %q code = %d stderr = %q", stdout, code, stderr)
	}
	stdout, _, _ = execute(t, inApp, "tasks")
	if !strings.Contains(stdout, "App tests") || strings.Contains(stdout, "Web tests") {
		t.Fatalf("tasks in a project should list only that project: %q", stdout)
	}
	stdout, _, _ = execute(t, inApp, "tasks", "--all")
	if !strings.Contains(stdout, "App tests") || !strings.Contains(stdout, "Web tests") {
		t.Fatalf("tasks --all should list every project: %q", stdout)
	}
}
