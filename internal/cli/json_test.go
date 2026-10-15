package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func jsonWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), "version: 1\ndiscover:\n  - projects/*/.sd/tasks.yaml\n")
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project: {id: app, name: App, aliases: [a], workArea: Product}
tasks:
  release:
    desc: Release
    danger: true
    defaultMode: dry
    modes:
      dry: {danger: false}
      wet: {}
    run: ./release
    choices:
      cli: {label: CLI}
  check:
    desc: Check
    run: go test
`)
	write(t, filepath.Join(root, "projects", "empty", ".sd", "tasks.yaml"), "version: 1\nproject: {id: empty}\n")
	return root
}

func decode(t *testing.T, stdout string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(stdout), out); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
}

func TestTasksJSONContract(t *testing.T) {
	root := jsonWorkspace(t)
	stdout, stderr, code := execute(t, root, "tasks", "app", "--json")
	if code != 0 {
		t.Fatalf("code = %d stderr = %q", code, stderr)
	}
	var got struct {
		Version int              `json:"version"`
		Tasks   []map[string]any `json:"tasks"`
	}
	decode(t, stdout, &got)
	if got.Version != 1 || len(got.Tasks) != 3 {
		t.Fatalf("got %+v", got)
	}
	byKey := map[string]map[string]any{}
	for _, task := range got.Tasks {
		byKey[task["id"].(string)+"|"+task["choice"].(string)] = task
	}
	release, cli := byKey["release|"], byKey["release|cli"]
	if release == nil || cli == nil || byKey["check|"] == nil {
		t.Fatalf("missing rows: %v", byKey)
	}
	if release["danger"] != false || release["defaultMode"] != "dry" || len(release["modes"].([]any)) != 2 {
		t.Fatalf("release row = %v, want danger false under the default dry mode", release)
	}
	if cli["project"] != "app" || cli["desc"] != "CLI" {
		t.Fatalf("choice row = %v", cli)
	}

	stdout, _, _ = execute(t, root, "tasks", "empty", "--json")
	if stdout != "{\"version\":1,\"tasks\":[]}\n" {
		t.Fatalf("empty project = %q", stdout)
	}
}

func TestProjectsJSONContract(t *testing.T) {
	root := jsonWorkspace(t)
	stdout, _, code := execute(t, root, "projects", "--json")
	var got struct {
		Version  int              `json:"version"`
		Projects []map[string]any `json:"projects"`
	}
	decode(t, stdout, &got)
	if code != 0 || got.Version != 1 || len(got.Projects) != 2 {
		t.Fatalf("got %+v", got)
	}
	var app, empty map[string]any
	for _, project := range got.Projects {
		if project["id"] == "app" {
			app = project
		} else {
			empty = project
		}
	}
	if app["id"] != "app" || app["path"] != filepath.Join(root, "projects", "app") || app["tasks"] != nil {
		t.Fatalf("project = %v, want public fields only", app)
	}
	if aliases, ok := empty["aliases"].([]any); !ok || len(aliases) != 0 {
		t.Fatalf("aliases should be an empty list, got %v", empty["aliases"])
	}
}

func TestExplainJSONContract(t *testing.T) {
	root := jsonWorkspace(t)
	stdout, _, code := execute(t, root, "explain", "app", "check", "--json")
	var got map[string]any
	decode(t, stdout, &got)
	if code != 0 || got["version"] != float64(1) {
		t.Fatalf("got %v", got)
	}
	for _, key := range []string{"env", "envFiles", "requiresEnv", "depends", "scripts", "values"} {
		if got[key] == nil {
			t.Fatalf("%s should be present and empty, got %v", key, got)
		}
	}
}
