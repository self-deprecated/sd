package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/cli"
)

func mirrorJSONFixture(t *testing.T) (root, mirrorRoot string) {
	t.Helper()
	root = t.TempDir()
	mirrorRoot = filepath.Join(t.TempDir(), "mirrors")
	writeMirrorFixture(t, root)
	config := filepath.Join(root, ".sd", "mirrors.yaml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	write(t, config, strings.Replace(string(data), "      - true", "      - printf noisy", 1))
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(mirrorRoot, "demo", ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "")
	return root, mirrorRoot
}

type mirrorCheckJSON struct {
	Version       int    `json:"version"`
	Mirror        string `json:"mirror"`
	Passed        bool   `json:"passed"`
	ExportedFiles int    `json:"exportedFiles"`
	Allowed       []any  `json:"allowed"`
	Findings      []struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	} `json:"findings"`
}

func TestMirrorCheckJSON(t *testing.T) {
	root, mirrorRoot := mirrorJSONFixture(t)
	stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "check", "demo", "--json")
	var got mirrorCheckJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q (stderr %q)", stdout, stderr)
	}
	if code != 0 || got.Version != 1 || got.Mirror != "demo" || !got.Passed || got.ExportedFiles == 0 || got.Findings == nil || got.Allowed == nil {
		t.Fatalf("code = %d got = %+v", code, got)
	}
	if !strings.Contains(stderr, "noisy") {
		t.Fatalf("configured check output should go to stderr, stderr = %q", stderr)
	}

	write(t, filepath.Join(root, "projects", "demo", "leaky.env"), "TOKEN=abcdefghijklmnopqrstuvwxyz123456")
	stdout, _, code = execute(t, root, "mirror", "--mirror-root", mirrorRoot, "check", "demo", "--json")
	got = mirrorCheckJSON{}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if code != 2 || got.Passed || len(got.Findings) == 0 || got.Findings[0].Path != "leaky.env" {
		t.Fatalf("code = %d got = %+v", code, got)
	}
}

func TestMirrorJSONIsVersioned(t *testing.T) {
	root, mirrorRoot := mirrorJSONFixture(t)
	for _, args := range [][]string{{"mirror", "list", "--json"}, {"mirror", "--mirror-root", mirrorRoot, "explain", "demo", "--json"}, {"mirror", "--mirror-root", mirrorRoot, "plan", "demo", "--json"}} {
		stdout, stderr, code := execute(t, root, args...)
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); err != nil || code != 0 || got["version"] != float64(1) {
			t.Fatalf("%v: code = %d stdout = %q stderr = %q", args, code, stdout, stderr)
		}
	}
	stdout, _, _ := execute(t, root, "mirror", "list", "--json")
	if !strings.Contains(stdout, `"mirrors":[{"id":"demo","visibility":"public-code"}]`) {
		t.Fatalf("list = %q", stdout)
	}
}
