package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The test binary doubles as a fake jj so mirror tests run on every OS.
const fakeJJStateEnv = "SD_FAKE_JJ_STATE"

func TestMain(m *testing.M) {
	if state := os.Getenv(fakeJJStateEnv); state != "" {
		os.Exit(fakeJJ(state, os.Getenv("SD_FAKE_JJ_MODE"), os.Args[1:]))
	}
	os.Exit(m.Run())
}

// InstallFakeJJ puts a fake jj first on PATH. Its files are written to root.
// mode "commit" reports a dirty working copy after the first status call;
// mode "trace-failure" does the same and fails `jj log`.
func InstallFakeJJ(t *testing.T, root, mode string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "jj"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := copyExecutable(self, filepath.Join(bin, name)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeJJStateEnv, root)
	t.Setenv("SD_FAKE_JJ_MODE", mode)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fakeJJ(state, mode string, args []string) int {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "root":
		wd, _ := os.Getwd()
		fmt.Println(wd)
	case "st":
		if mode != "" && fakeJJStatusCount(state) > 1 {
			fmt.Println("Modified files:")
			fmt.Println("M README.md")
		} else {
			fmt.Println("The working copy has no changes.")
		}
	case "log":
		if mode == "trace-failure" {
			fmt.Fprintln(os.Stderr, "trace unavailable")
			return 42
		}
		fmt.Println("abc def")
	case "commit":
		appendFile(filepath.Join(state, "fake-commit-called"), strings.Join(args, " ")+"\n")
	case "git":
		if len(args) > 1 && args[1] == "push" {
			appendFile(filepath.Join(state, "fake-push-called"), "")
			return 99
		}
	}
	return 0
}

func fakeJJStatusCount(state string) int {
	path := filepath.Join(state, "fake-status-count")
	data, _ := os.ReadFile(path)
	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	count++
	_ = os.WriteFile(path, []byte(strconv.Itoa(count)+"\n"), 0o644)
	return count
}

func appendFile(path, content string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(content)
}
