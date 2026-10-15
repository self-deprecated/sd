package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
)

func TestReadMirrorsAndLocalPathConfig(t *testing.T) {
	root := t.TempDir()
	mirrorsPath := filepath.Join(root, ".sd", "mirrors.yaml")
	write(t, mirrorsPath, `version: 1
mirrors:
  demo:
    source: projects/demo
    destination:
      root: public-mirrors
      path: demo
    bookmark: main
    remote: origin
    visibility: public-code
    include: ["**"]
    exclude: ["node_modules/**"]
    preserve: ["node_modules/**"]
    overlays:
      - from: CONTEXT.public.md
        to: CONTEXT.md
    checks:
      - sd demo check
`)
	mf, err := config.ReadMirrors(mirrorsPath)
	if err != nil {
		t.Fatalf("ReadMirrors: %v", err)
	}
	cfg := mf.Mirrors["demo"]
	if mf.Version != 1 || cfg.Source != "projects/demo" || cfg.Destination.Root != "public-mirrors" || cfg.Destination.Path != "demo" {
		t.Fatalf("unexpected mirror config: %#v", mf)
	}
	if len(cfg.Preserve) != 1 || cfg.Preserve[0] != "node_modules/**" {
		t.Fatalf("unexpected preserve patterns: %#v", cfg.Preserve)
	}
	if len(cfg.Overlays) != 1 || cfg.Overlays[0].From != "CONTEXT.public.md" || cfg.Overlays[0].To != "CONTEXT.md" {
		t.Fatalf("unexpected overlays: %#v", cfg.Overlays)
	}

	localPath := filepath.Join(root, ".sd", "mirrors.local.yaml")
	write(t, localPath, `version: 1
paths:
  public-mirrors: /tmp/mirrors
`)
	lf, err := config.ReadMirrorLocal(localPath)
	if err != nil {
		t.Fatalf("ReadMirrorLocal: %v", err)
	}
	if got := lf.Paths["public-mirrors"]; got != "/tmp/mirrors" {
		t.Fatalf("local path = %q", got)
	}
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
