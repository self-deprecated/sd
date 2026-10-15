package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests drive the built binary against testdata/workspace on every OS.

var sdBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sd-smoke")
	if err != nil {
		panic(err)
	}
	sdBinary = filepath.Join(dir, "sd")
	if runtime.GOOS == "windows" {
		sdBinary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", sdBinary, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func runSD(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(sdBinary, args...)
	cmd.Dir = filepath.Join("testdata", "workspace", "projects", "demo")
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

func TestBinarySmoke(t *testing.T) {
	cases := []struct {
		name   string
		env    []string
		args   []string
		stdout string
		stderr string
		code   int
	}{
		{"version", nil, []string{"--version"}, "sd version dev\n", "", 0},
		{"choice with defaults", nil, []string{"demo", "greet", "world"}, "hello world from dev-file", "", 0},
		{"choice with var override", nil, []string{"demo", "greet", "world", "--var", "target=prod"}, "hello world from prod-file", "", 0},
		{"runtime var and mode", nil, []string{"demo", "hello", "--var", "name=you", "--mode", "loud"}, "hello you!", "", 0},
		{"whenChanged match", []string{"SD_CHANGED_FILES=projects/demo/src/main.go"}, []string{"demo", "changed"}, "changed", "", 0},
		{"whenChanged miss", []string{"SD_CHANGED_FILES=projects/demo/README.md"}, []string{"demo", "changed"}, "sd: skipping demo:changed; no changed files match src/**\n", "", 0},
		{"failing task", nil, []string{"demo", "fail"}, "", "exit status 3", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runSD(t, tc.env, tc.args...)
			if stdout != tc.stdout || code != tc.code || !strings.Contains(stderr, tc.stderr) {
				t.Fatalf("stdout=%q code=%d stderr=%q; want stdout=%q code=%d stderr containing %q", stdout, code, stderr, tc.stdout, tc.code, tc.stderr)
			}
		})
	}
}

func TestBinaryListsAndExplainsTasks(t *testing.T) {
	stdout, stderr, code := runSD(t, nil, "tasks", "--json")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, id := range []string{`"id":"greet"`, `"id":"changed"`, `"id":"fail"`} {
		if !strings.Contains(stdout, id) {
			t.Fatalf("tasks --json missing %s: %s", id, stdout)
		}
	}
	stdout, stderr, code = runSD(t, nil, "explain", "demo", "greet", "world", "--var", "target=prod", "--json")
	if code != 0 || !strings.Contains(stdout, `"run":"printf '%s %s from %s%s' \"$GREETING\" \"world\" \"$FROM_FILE\" \"\""`) {
		t.Fatalf("explain stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}
