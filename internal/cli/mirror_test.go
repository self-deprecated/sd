package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/cli"
)

func TestMirrorCommandsUseConfigurableDestinationAndNeverPush(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(dest, ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "")

	t.Run("list does not require local path resolution", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "mirror", "list")
		if code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "demo") {
			t.Fatalf("list missing mirror: %q", stdout)
		}
	})

	t.Run("missing destination variable is a hard error", func(t *testing.T) {
		_, stderr, code := execute(t, root, "mirror", "plan", "demo")
		if code != 2 || !strings.Contains(stderr, "destination root \"public-mirrors\" is not configured") || !strings.Contains(stderr, "SD_PUBLIC_MIRRORS") {
			t.Fatalf("expected missing variable usage error, code=%d stderr=%q", code, stderr)
		}
		_, stderr, code = execute(t, root, "mirror", "explain", "demo")
		if code != 2 || !strings.Contains(stderr, "destination root \"public-mirrors\" is not configured") || !strings.Contains(stderr, "sd mirror init") {
			t.Fatalf("expected explain missing variable usage error, code=%d stderr=%q", code, stderr)
		}
	})

	t.Run("plan resolves CLI mirror root without modifying destination", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "plan", "demo", "--json")
		if code != 0 {
			t.Fatalf("json plan code=%d stderr=%s", code, stderr)
		}
		var plan struct {
			Destination string                  `json:"destination"`
			Creates     []struct{ Path string } `json:"creates"`
		}
		if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
			t.Fatalf("plan json parse: %v\n%s", err, stdout)
		}
		if plan.Destination != dest || len(plan.Creates) == 0 {
			t.Fatalf("unexpected json plan: %#v", plan)
		}

		stdout, stderr, code = execute(t, root, "mirror", "--mirror-root", mirrorRoot, "plan", "demo")
		if code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, dest) || !strings.Contains(stdout, "create README.md") || !strings.Contains(stdout, "CONTEXT.public.md -> CONTEXT.md") {
			t.Fatalf("unexpected plan output: %q", stdout)
		}
		if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
			t.Fatalf("plan modified destination, stat err=%v", err)
		}
	})

	t.Run("plan text truncates long operation lists", func(t *testing.T) {
		write(t, filepath.Join(root, "projects", "demo", "extra-one.txt"), "1")
		write(t, filepath.Join(root, "projects", "demo", "extra-two.txt"), "2")
		stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "--op-limit", "1", "plan", "demo")
		if code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "more create operation") || strings.Contains(stdout, "extra-two.txt\n  create") {
			t.Fatalf("expected truncated output, got %q", stdout)
		}
		stdout, stderr, code = execute(t, root, "mirror", "--mirror-root", mirrorRoot, "--op-limit", "1", "--all-ops", "plan", "demo")
		if code != 0 {
			t.Fatalf("all ops code=%d stderr=%s", code, stderr)
		}
		if strings.Contains(stdout, "more create operation") {
			t.Fatalf("--all-ops should not truncate: %q", stdout)
		}
	})

	t.Run("check runs blacklist scan and configured checks without syncing", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "check", "demo")
		if code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "passed blacklist scan") || !strings.Contains(stdout, "configured check") {
			t.Fatalf("unexpected check output: %q", stdout)
		}
		if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
			t.Fatalf("check modified destination, stat err=%v", err)
		}
	})

	t.Run("sync rejects blacklisted exports before modifying destination", func(t *testing.T) {
		write(t, filepath.Join(root, "projects", "demo", "leaky.env"), "TOKEN=abcdefghijklmnopqrstuvwxyz123456")
		_, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "sync", "demo")
		if code != 2 || !strings.Contains(stderr, "blacklist scan") || !strings.Contains(stderr, "leaky.env") {
			t.Fatalf("expected blacklist usage error, code=%d stderr=%q", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
			t.Fatalf("failed sync modified destination, stat err=%v", err)
		}
		if err := os.Remove(filepath.Join(root, "projects", "demo", "leaky.env")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sync copies files and does not push", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "sync", "demo")
		if code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "No remote push was performed") {
			t.Fatalf("sync did not print no-push guarantee: %q", stdout)
		}
		assertContent(t, filepath.Join(dest, "README.md"), "hello")
		assertContent(t, filepath.Join(dest, "CONTEXT.md"), "public")
		assertContent(t, filepath.Join(dest, ".jj", "repo"), "repo")
		if _, err := os.Stat(filepath.Join(root, "fake-push-called")); !os.IsNotExist(err) {
			t.Fatalf("fake jj observed a push call")
		}
	})
}

func TestMirrorListExplainJSONAndDestinationPrecedence(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "local-mirrors")
	envRoot := filepath.Join(root, "env-mirrors")
	cliRoot := filepath.Join(root, "cli-mirrors")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, ".sd", "mirrors.local.yaml"), "version: 1\n\npaths:\n  public-mirrors: "+localRoot+"\n")

	stdout, stderr, code := execute(t, root, "mirror", "list", "--json")
	if code != 0 {
		t.Fatalf("list json code=%d stderr=%s", code, stderr)
	}
	var list struct {
		Version int `json:"version"`
		Mirrors []struct {
			ID string `json:"id"`
		} `json:"mirrors"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("list json parse: %v\n%s", err, stdout)
	}
	if list.Version != 1 || len(list.Mirrors) != 1 || list.Mirrors[0].ID != "demo" {
		t.Fatalf("list = %#v", list)
	}

	stdout, stderr, code = execute(t, root, "mirror", "explain", "demo", "--json")
	if code != 0 {
		t.Fatalf("explain json code=%d stderr=%s", code, stderr)
	}
	var explain struct {
		Destination string `json:"destination"`
	}
	if err := json.Unmarshal([]byte(stdout), &explain); err != nil {
		t.Fatalf("explain json parse: %v\n%s", err, stdout)
	}
	if explain.Destination != filepath.Join(localRoot, "demo") {
		t.Fatalf("local destination = %q", explain.Destination)
	}

	t.Setenv("SD_PUBLIC_MIRRORS", envRoot)
	stdout, stderr, code = execute(t, root, "mirror", "explain", "demo", "--json")
	if code != 0 {
		t.Fatalf("env explain code=%d stderr=%s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &explain); err != nil {
		t.Fatalf("env explain json parse: %v\n%s", err, stdout)
	}
	if explain.Destination != filepath.Join(envRoot, "demo") {
		t.Fatalf("env destination = %q", explain.Destination)
	}

	stdout, stderr, code = execute(t, root, "mirror", "--mirror-root", cliRoot, "explain", "demo", "--json")
	if code != 0 {
		t.Fatalf("cli explain code=%d stderr=%s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &explain); err != nil {
		t.Fatalf("cli explain json parse: %v\n%s", err, stdout)
	}
	if explain.Destination != filepath.Join(cliRoot, "demo") {
		t.Fatalf("cli destination = %q", explain.Destination)
	}
}

func TestMirrorCommitCreatesLocalCommitAndDoesNotPush(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(dest, ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "commit")

	stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "commit", "demo", "-m", "feat: public snapshot")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "created local mirror commit") || !strings.Contains(stdout, "No remote push was performed") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	commitLog, err := os.ReadFile(filepath.Join(root, "fake-commit-called"))
	if err != nil {
		t.Fatalf("expected fake commit call: %v", err)
	}
	for _, want := range []string{"feat: public snapshot", "Export-Profile: demo", "Monorepo-Change: abc", "Monorepo-Commit: def"} {
		if !strings.Contains(string(commitLog), want) {
			t.Fatalf("commit message missing %q: %s", want, string(commitLog))
		}
	}
	if _, err := os.Stat(filepath.Join(root, "fake-push-called")); !os.IsNotExist(err) {
		t.Fatalf("fake jj observed a push call")
	}
}

func TestMirrorCommitWarnsWhenTraceTrailersCannotBeAdded(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(dest, ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "trace-failure")

	_, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "commit", "demo", "-m", "feat: public snapshot")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "warning: could not add monorepo trace trailers") {
		t.Fatalf("expected trace warning, stderr=%q", stderr)
	}
	commitLog, err := os.ReadFile(filepath.Join(root, "fake-commit-called"))
	if err != nil {
		t.Fatalf("expected fake commit call: %v", err)
	}
	if strings.Contains(string(commitLog), "Export-Profile") {
		t.Fatalf("trace trailers should be omitted on lookup failure: %s", string(commitLog))
	}
}

func TestMirrorPublishCreatesLocalCommitAndDoesNotPush(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(dest, ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "commit")

	stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "publish", "demo", "-m", "feat: public publish")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "created local mirror commit") || !strings.Contains(stdout, "push manually") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, "fake-push-called")); !os.IsNotExist(err) {
		t.Fatalf("fake jj observed a push call")
	}
}

func TestMirrorCommitRequiresMessageAndNoopsWhenClean(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	write(t, filepath.Join(dest, ".jj", "repo"), "repo")
	cli.InstallFakeJJ(t, root, "")

	_, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "commit", "demo")
	if code != 2 || !strings.Contains(stderr, "requires -m/--message") {
		t.Fatalf("expected missing message usage error, code=%d stderr=%q", code, stderr)
	}

	stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "commit", "demo", "-m", "feat: noop")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "no changes after sync; no commit created") {
		t.Fatalf("expected no-op commit output, got %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, "fake-commit-called")); !os.IsNotExist(err) {
		t.Fatalf("commit should not have been called for clean status")
	}
}

func TestMirrorInitWritesIgnoredLocalConfig(t *testing.T) {
	root := t.TempDir()
	writeMirrorFixture(t, root)
	stdout, stderr, code := execute(t, root, "mirror", "init", "--root", "/tmp/public-mirrors")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, filepath.Join(".sd", "mirrors.local.yaml")) {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	assertContent(t, filepath.Join(root, ".sd", "mirrors.local.yaml"), "version: 1\n\npaths:\n  public-mirrors: /tmp/public-mirrors\n")
}

func writeMirrorFixture(t *testing.T, root string) {
	t.Helper()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
workspace:
  id: test-root
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, ".sd", "mirrors.yaml"), `version: 1
mirrors:
  demo:
    source: projects/demo
    destination:
      root: public-mirrors
      path: demo
    visibility: public-code
    include: ["**"]
    exclude:
      - CONTEXT.public.md
      - .jj/**
      - .git/**
    overlays:
      - from: CONTEXT.public.md
        to: CONTEXT.md
    checks:
      - true
`)
	write(t, filepath.Join(root, "projects", "demo", ".sd", "tasks.yaml"), `version: 1
project:
  id: demo
  name: Demo
tasks:
  check:
    desc: Check
    run: true
`)
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q want %q", path, string(data), want)
	}
}

func TestMirrorSyncRequiresDestinationToBeAJJWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
	dest := filepath.Join(mirrorRoot, "demo")
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	// The destination is only a subdirectory of some other jj workspace.
	write(t, filepath.Join(mirrorRoot, ".jj", "repo"), "repo")
	write(t, filepath.Join(dest, "placeholder"), "")
	cli.InstallFakeJJ(t, root, "")

	_, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "sync", "demo")
	if code != 2 || !strings.Contains(stderr, "not a jj workcopy root") {
		t.Fatalf("expected jj workcopy root error, code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("sync modified a destination that is not a workspace root, stat err=%v", err)
	}
}

func TestMirrorPlanRejectsDestinationOverlappingTheMonorepo(t *testing.T) {
	root := t.TempDir()
	writeMirrorFixture(t, root)
	write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
	write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
	cli.InstallFakeJJ(t, root, "")

	// --mirror-root projects makes the destination the source itself.
	for _, mirrorRoot := range []string{filepath.Join(root, "projects"), filepath.Join(root, "mirrors")} {
		stdout, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, "plan", "demo", "--json")
		if code == 0 || !strings.Contains(stderr, "overlaps") {
			t.Errorf("mirror root %s: expected overlap error, code=%d stdout=%s stderr=%q", mirrorRoot, code, stdout, stderr)
		}
	}
}

func TestMirrorCommitScansDestinationContentsBeforeCommitting(t *testing.T) {
	for _, action := range []string{"commit", "publish"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			mirrorRoot := filepath.Join(t.TempDir(), "mirrors")
			dest := filepath.Join(mirrorRoot, "demo")
			writeMirrorFixture(t, root)
			// A configured check generates a secrets file inside the mirror.
			mirrors := filepath.Join(root, ".sd", "mirrors.yaml")
			data, err := os.ReadFile(mirrors)
			if err != nil {
				t.Fatal(err)
			}
			check := fmt.Sprintf("      - \"printf 'API_TOKEN=Zq8vN2kLp4Rt7Yw1Xc5Bm9Hd' > '%s'\"\n", filepath.ToSlash(filepath.Join(dest, ".env")))
			write(t, mirrors, strings.Replace(string(data), "      - true\n", check, 1))
			write(t, filepath.Join(root, "projects", "demo", "README.md"), "hello")
			write(t, filepath.Join(root, "projects", "demo", "CONTEXT.public.md"), "public")
			write(t, filepath.Join(dest, ".jj", "repo"), "repo")
			cli.InstallFakeJJ(t, root, "commit")

			_, stderr, code := execute(t, root, "mirror", "--mirror-root", mirrorRoot, action, "demo", "-m", "feat: public snapshot")
			if _, err := os.Stat(filepath.Join(dest, ".env")); err != nil {
				t.Fatalf("check did not generate .env: %v (stderr %q)", err, stderr)
			}
			if _, err := os.Stat(filepath.Join(root, "fake-commit-called")); !os.IsNotExist(err) {
				t.Errorf("jj commit ran although the mirror contains .env")
			}
			if code != 2 || !strings.Contains(stderr, "blacklist scan") || !strings.Contains(stderr, ".env: forbidden path pattern") {
				t.Fatalf("expected blacklist usage error, code=%d stderr=%q", code, stderr)
			}
		})
	}
}
