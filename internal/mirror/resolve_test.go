package mirror_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/mirror"
)

func TestResolveDestinationPrecedenceAndMissingVariable(t *testing.T) {
	cfg := config.MirrorConfig{Destination: config.MirrorDestination{Root: "public-mirrors", Path: "demo"}}

	got, err := mirror.ResolveDestination("demo", cfg, mirror.ResolveOptions{
		CLIPathOverrides: map[string]string{"public-mirrors": "/cli"},
		Env:              map[string]string{"SD_PUBLIC_MIRRORS": "/env"},
		LocalPaths:       map[string]string{"public-mirrors": "/local"},
	})
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	if got != filepath.Clean("/cli/demo") {
		t.Fatalf("CLI override destination = %q", got)
	}

	got, err = mirror.ResolveDestination("demo", cfg, mirror.ResolveOptions{
		Env:        map[string]string{"SD_PUBLIC_MIRRORS": "/env"},
		LocalPaths: map[string]string{"public-mirrors": "/local"},
	})
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	if got != filepath.Clean("/env/demo") {
		t.Fatalf("env destination = %q", got)
	}

	got, err = mirror.ResolveDestination("demo", cfg, mirror.ResolveOptions{
		Env:        map[string]string{},
		LocalPaths: map[string]string{"public-mirrors": "/local"},
	})
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	if got != filepath.Clean("/local/demo") {
		t.Fatalf("local destination = %q", got)
	}

	_, err = mirror.ResolveDestination("demo", cfg, mirror.ResolveOptions{Env: map[string]string{}})
	if _, ok := err.(mirror.MissingPathVariableError); !ok {
		t.Fatalf("missing variable error = %T %v", err, err)
	}
}

func TestResolveDestinationExpandsHome(t *testing.T) {
	cfg := config.MirrorConfig{Destination: config.MirrorDestination{Root: "public-mirrors", Path: "demo"}}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	got, err := mirror.ResolveDestination("demo", cfg, mirror.ResolveOptions{
		Env:        map[string]string{},
		LocalPaths: map[string]string{"public-mirrors": "~/public-mirrors"},
	})
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	wantSuffix := filepath.Join("public-mirrors", "demo")
	if !strings.HasPrefix(got, home) || !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("expanded destination = %q, want under %q ending %q", got, home, wantSuffix)
	}
}

func TestEnvNameForPathVariable(t *testing.T) {
	if got := mirror.EnvNameForPathVariable("public-mirrors"); got != "SD_PUBLIC_MIRRORS" {
		t.Fatalf("env name = %q", got)
	}
}
