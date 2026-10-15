package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// npm, corepack, and similar tools install as .cmd shims on Windows.
func TestRunExecutesWindowsBatchShimsFromPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("batch files only exist on Windows")
	}
	bin := t.TempDir()
	shim := "@echo off\r\necho shim args: %*\r\nexit /b 7\r\n"
	if err := os.WriteFile(filepath.Join(bin, "shim.cmd"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	err := Run(nil, t.TempDir(), `shim "two words" plain`, nil, env, nil, &out, &out)
	if err == nil || err.Error() != "exit status 7" {
		t.Fatalf("err = %v, want exit status 7 (output %q)", err, out.String())
	}
	if got := strings.TrimSpace(out.String()); got != `shim args: "two words" plain` {
		t.Fatalf("output = %q", got)
	}
}
