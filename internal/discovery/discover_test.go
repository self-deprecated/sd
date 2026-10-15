package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/self-deprecated/sd/internal/discovery"
)

func TestLoadWorkspaceDiscoversProjectsAggregatesAndStandaloneRoot(t *testing.T) {
	t.Run("workspace discovers project manifests and aggregate tasks", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
workspace:
  id: test-root
  name: Test Root
discover:
  - projects/*/.sd/tasks.yaml
`)
		write(t, filepath.Join(root, ".sd", "aggregates.yaml"), `version: 1
tasks:
  all/check:
    desc: Check everything
    depends:
      - app:test
`)
		write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
  aliases: [application]
  workArea: Product
  dir: .
tasks:
  test:
    desc: Run tests
    run: echo test
`)

		ws, err := discovery.LoadWorkspace(filepath.Join(root, "projects", "app"))
		if err != nil {
			t.Fatalf("LoadWorkspace: %v", err)
		}
		if ws.Root != root {
			t.Fatalf("Root = %q, want %q", ws.Root, root)
		}
		if _, ok := ws.ProjectByID("app"); !ok {
			t.Fatalf("expected app project to be discovered")
		}
		if _, ok := ws.ProjectByID("application"); !ok {
			t.Fatalf("expected app alias to resolve")
		}
		if _, ok := ws.Aggregates["all/check"]; !ok {
			t.Fatalf("expected all/check aggregate")
		}
	})

	t.Run("standalone project root works without workspace manifest", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, ".sd", "tasks.yaml"), `version: 1
project:
  id: solo
  name: Solo
tasks:
  check:
    desc: Check solo
    run: echo check
`)
		ws, err := discovery.LoadWorkspace(filepath.Join(root, "subdir"))
		if err != nil {
			t.Fatalf("LoadWorkspace: %v", err)
		}
		if ws.Root != root {
			t.Fatalf("Root = %q, want %q", ws.Root, root)
		}
		if _, ok := ws.ProjectByID("solo"); !ok {
			t.Fatalf("expected solo project")
		}
	})
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
