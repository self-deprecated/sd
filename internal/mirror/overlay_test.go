package mirror_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func overlayFixture(t *testing.T) (root, source, dest string) {
	t.Helper()
	root = t.TempDir()
	source = filepath.Join(root, "project")
	dest = filepath.Join(root, "mirror")
	write(t, filepath.Join(source, "README.md"), "hello")
	write(t, filepath.Join(source, "CONTEXT.public.md"), "public")
	write(t, filepath.Join(root, "private.txt"), "private")
	write(t, filepath.Join(dest, ".git", "config"), "mirror git config")
	return root, source, dest
}

func TestBuildPlanRejectsOverlayTargetsOutsideTheMirror(t *testing.T) {
	for _, to := range []string{
		"../escaped.txt",
		`..\escaped.txt`,
		"docs/../../escaped.txt",
		"/tmp/escaped.txt",
		`\escaped.txt`,
		`\\server\share\escaped.txt`,
		"C:/escaped.txt",
		`C:\escaped.txt`,
		"c:escaped.txt",
		".",
		"docs/..",
		".git/config",
		".jj/repo/store",
		`.git\config`,
		".GIT/config",
		"sub/.git/config",
	} {
		t.Run(to, func(t *testing.T) {
			root, source, dest := overlayFixture(t)
			cfg := config.MirrorConfig{Source: source, Overlays: []config.MirrorOverlay{{From: "CONTEXT.public.md", To: to}}}
			plan, err := mirror.BuildPlan("app", source, cfg, dest)
			if err == nil {
				_ = mirror.ApplyPlan(plan)
			}
			assertAbsent(t, filepath.Join(root, "escaped.txt"))
			assertFile(t, filepath.Join(dest, ".git", "config"), "mirror git config")
			if err == nil || !strings.Contains(err.Error(), "overlay") {
				t.Fatalf("BuildPlan accepted overlay target %q (err %v)", to, err)
			}
		})
	}
}

func TestBuildPlanRejectsOverlaySourcesOutsideTheSource(t *testing.T) {
	root, source, dest := overlayFixture(t)
	for _, from := range []string{"../private.txt", `..\private.txt`, filepath.Join(root, "private.txt")} {
		cfg := config.MirrorConfig{Source: source, Overlays: []config.MirrorOverlay{{From: from, To: "CONTEXT.md"}}}
		if _, err := mirror.BuildPlan("app", source, cfg, dest); err == nil || !strings.Contains(err.Error(), "overlay") {
			t.Errorf("BuildPlan accepted overlay source %q (err %v)", from, err)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "elsewhere", "private.txt"), "private")
	symlink(t, filepath.Join("..", "elsewhere"), filepath.Join(source, "linked"))
	cfg := config.MirrorConfig{Source: source, Exclude: []string{"linked"}, Overlays: []config.MirrorOverlay{{From: "linked/private.txt", To: "CONTEXT.md"}}}
	if _, err := mirror.BuildPlan("app", source, cfg, dest); err == nil || !strings.Contains(err.Error(), "overlay") {
		t.Errorf("BuildPlan accepted overlay source through a symlinked directory (err %v)", err)
	}
}

func TestBuildPlanNormalisesOverlayPaths(t *testing.T) {
	_, source, dest := overlayFixture(t)
	write(t, filepath.Join(source, "docs", "public.md"), "docs")
	cfg := config.MirrorConfig{Source: source, Exclude: []string{"docs/**"}, Overlays: []config.MirrorOverlay{{From: `docs\public.md`, To: `./site\index.md`}}}
	plan, err := mirror.BuildPlan("app", source, cfg, dest)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	assertOps(t, "creates", plan.Creates, []string{"CONTEXT.public.md", "README.md", "site/index.md"})
	if err := mirror.ApplyPlan(plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	assertFile(t, filepath.Join(dest, "site", "index.md"), "docs")
}
