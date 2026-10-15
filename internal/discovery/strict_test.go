package discovery_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/discovery"
)

func writeWorkspace(t *testing.T, manifests map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), "version: 1\ndiscover:\n  - projects/*/.sd/tasks.yaml\n")
	for name, content := range manifests {
		write(t, filepath.Join(root, "projects", name, ".sd", "tasks.yaml"), content)
	}
	return root
}

func TestLoadWorkspaceRejectsInvalidManifests(t *testing.T) {
	cases := []struct {
		name      string
		manifests map[string]string
		want      string
	}{
		{"misspelled task field", map[string]string{"app": "version: 1\nproject: {id: app}\ntasks:\n  deploy:\n    run: ./deploy\n    dangerous: true\n"}, "dangerous"},
		{"unsupported version", map[string]string{"app": "version: 999\nproject: {id: app}\ntasks:\n  test: {run: go test}\n"}, "version 999"},
		{"missing version", map[string]string{"app": "project: {id: app}\ntasks:\n  test: {run: go test}\n"}, "version"},
		{"duplicate project id", map[string]string{"a": "version: 1\nproject: {id: app}\n", "b": "version: 1\nproject: {id: app}\n"}, `duplicate project id "app"`},
		{"alias shadows a project", map[string]string{"app": "version: 1\nproject: {id: app}\n", "web": "version: 1\nproject: {id: web, aliases: [app]}\n"}, `alias "app"`},
		{"alias claimed twice", map[string]string{"a": "version: 1\nproject: {id: a, aliases: [x]}\n", "b": "version: 1\nproject: {id: b, aliases: [x]}\n"}, `alias "x"`},
		{"task and passthrough collide", map[string]string{"app": "version: 1\nproject: {id: app}\ntasks:\n  npm: {run: npm test}\npassthrough:\n  npm: {run: npm \"$@\"}\n"}, `"npm"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discovery.LoadWorkspace(writeWorkspace(t, tc.manifests))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
