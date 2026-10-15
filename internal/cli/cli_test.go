package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/cli"
)

func TestCLIListsChildrenExplainsJSONAndRunsTasks(t *testing.T) {
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
      - app:build/web
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
  dir: .
tasks:
  build/web:
    desc: Build web
    run: printf web
  dangerous:
    desc: Dangerous task
    danger: true
    requiresEnv: [TOKEN]
    run: printf danger
passthrough:
  echo:
    desc: Echo passthrough
    run: printf '%s' "$@"
`)

	t.Run("incomplete path lists children", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "build")
		if code != 2 {
			t.Fatalf("code = %d, want 2\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "app build needs one of") || !strings.Contains(stderr, "web") {
			t.Fatalf("expected child task help, got stdout=%q stderr=%q", stdout, stderr)
		}
	})

	t.Run("explain json includes command and safety", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "explain", "app", "dangerous", "--json")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		for _, want := range []string{`"id":"dangerous"`, `"danger":true`, `"TOKEN"`} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("explain json missing %s: %s", want, stdout)
			}
		}
	})

	t.Run("task execution streams command output", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "build", "web")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "web" {
			t.Fatalf("stdout = %q, want web", stdout)
		}
	})

	t.Run("log file tees sd and task output", func(t *testing.T) {
		logPath := filepath.Join(root, "tmp", "sd-task.log")
		stdout, stderr, code := execute(t, root, "--log-file", logPath, "app", "build", "web")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "web" {
			t.Fatalf("stdout = %q, want web", stdout)
		}
		if !strings.Contains(stderr, "sd: logging output to "+logPath) {
			t.Fatalf("stderr missing log path: %q", stderr)
		}
		logBytes, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		log := string(logBytes)
		if !strings.Contains(log, "sd: logging output to "+logPath) || !strings.Contains(log, "web") {
			t.Fatalf("log file missing expected output: %q", log)
		}
	})

	t.Run("dangerous task requires yes and environment", func(t *testing.T) {
		_, stderr, code := execute(t, root, "app", "dangerous")
		if code != 2 || !strings.Contains(stderr, "requires --yes") {
			t.Fatalf("expected --yes safety failure, code=%d stderr=%q", code, stderr)
		}
		_, stderr, code = execute(t, root, "app", "dangerous", "--yes")
		if code != 2 || !strings.Contains(stderr, "missing required environment") {
			t.Fatalf("expected missing env failure, code=%d stderr=%q", code, stderr)
		}
	})

	t.Run("aggregate task runs dependencies", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "all", "check")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "web" {
			t.Fatalf("stdout = %q, want web", stdout)
		}
	})

	t.Run("passthrough task receives args after dashdash", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "echo", "--", "hello")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "hello" {
			t.Fatalf("stdout = %q, want hello", stdout)
		}
	})

	t.Run("task args may contain flags without dashdash like old dispatcher", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "echo", "--flag")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "--flag" {
			t.Fatalf("stdout = %q, want --flag", stdout)
		}
	})

	t.Run("passthrough args after dashdash are not parsed as sd flags", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "echo", "--", "--yes")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "--yes" {
			t.Fatalf("stdout = %q, want --yes", stdout)
		}
	})

	t.Run("passthrough var-like args are preserved when task has no runtime inputs", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "echo", "--var", "name=value")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "--varname=value" {
			t.Fatalf("stdout = %q, want --varname=value", stdout)
		}
	})
}

func TestProjectAndAggregateBareCommandsListTasksForOldDispatcherParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, ".sd", "aggregates.yaml"), `version: 1
tasks:
  all/check:
    desc: Check everything
    depends:
      - app:check
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
tasks:
  check:
    desc: Check app
    run: echo check
`)

	t.Run("bare project lists tasks with success", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "check") || !strings.Contains(stdout, "Check app") {
			t.Fatalf("expected task list, got stdout=%q", stdout)
		}
	})

	t.Run("bare all lists aggregate tasks with success", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "all")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "check") || !strings.Contains(stdout, "Check everything") {
			t.Fatalf("expected aggregate task list, got stdout=%q", stdout)
		}
	})

	t.Run("list aliases projects", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "list")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "app") {
			t.Fatalf("expected projects list, got stdout=%q", stdout)
		}
	})
}

func TestChoiceTaskListsExplainsCompletesAndRunsChoicePresets(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
tasks:
  mobile/build:
    desc: Build mobile app
    run: printf "{{platform}} {{profile}} {{mode}}"
    options:
      platform:
        type: enum
        prompt: Platform
        values:
          ios:
            label: iOS
          android:
            label: Android
      profile:
        type: enum
        prompt: Profile
        values:
          development:
            label: Development
          production:
            label: Production
      mode:
        type: enum
        prompt: Build mode
        values:
          eas:
            label: EAS cloud
          local:
            label: Local
    choices:
      ios-dev:
        label: iOS development
        desc: Build iOS development profile
        values:
          platform: ios
          profile: development
          mode: eas
      ios-production:
        label: iOS production + submit
        danger: true
        values:
          platform: ios
          profile: production
          mode: eas
      android-local:
        label: Android local
        values:
          platform: android
          profile: development
          mode: local
`)

	t.Run("incomplete choice task lists choices", func(t *testing.T) {
		_, stderr, code := execute(t, root, "app", "mobile/build")
		if code != 2 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		for _, want := range []string{"app mobile/build needs a choice", "ios-dev", "iOS development", "android-local"} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("choice help missing %q: %s", want, stderr)
			}
		}
	})

	t.Run("choice renders typed option values into command", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "mobile/build", "ios-dev")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if stdout != "ios development eas" {
			t.Fatalf("stdout = %q", stdout)
		}
	})

	t.Run("choice explain shows final rendered command and safety", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "explain", "app", "mobile/build", "ios-production", "--json")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		for _, want := range []string{`"id":"mobile/build"`, `"choice":"ios-production"`, `"run":"printf \"ios production eas\""`, `"danger":true`} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("explain missing %s: %s", want, stdout)
			}
		}
	})

	t.Run("choice completion uses presets", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, cobraCompArgs("app", "mobile/build", "")...)
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "ios-dev") || !strings.Contains(stdout, "iOS development") {
			t.Fatalf("completion missing choice: stdout=%q", stdout)
		}
	})
}

func TestRuntimeVariableInputsDefaultOverrideValidationAndExplain(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
  dir: .
tasks:
  deploy:
    desc: Deploy target
    dir: work/{{target}}
    envFiles:
      - ../../env/{{target}}.env
    env:
      COUNT: "{{count}}"
    run: printf '%s|%s|%s|%s|%s|%s' "{{target}}" "{{message}}" "$COUNT" "$FROM_ENV_FILE" "$PWD" "{{profile}}"
    options:
      target:
        type: enum
        prompt: Target
        desc: Deployment target
        default: dev
        values:
          dev:
            label: Development
            vars:
              profile: debug
          prod:
            label: Production
            vars:
              profile: release
      message:
        type: text
        prompt: Message
      count:
        type: integer
        default: "2"
`)
	for _, dir := range []string{"work/dev", "work/prod", "env"} {
		if err := os.MkdirAll(filepath.Join(root, "projects", "app", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(root, "projects", "app", "env", "dev.env"), "FROM_ENV_FILE=dev-file\n")
	write(t, filepath.Join(root, "projects", "app", "env", "prod.env"), "FROM_ENV_FILE=prod-file\n")

	t.Run("defaults and explicit CLI vars render run dir env and envFiles", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "deploy", "--var", "message=hello")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		wantSuffix := filepath.Join("projects", "app", "work", "dev")
		if !strings.Contains(stdout, "dev|hello|2|dev-file|") || !strings.HasSuffix(stdout, wantSuffix+"|debug") {
			t.Fatalf("stdout = %q, want rendered dev values ending in %q", stdout, wantSuffix+"|debug")
		}
	})

	t.Run("explicit vars override defaults and validate enum vars", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "app", "deploy", "--var", "message=ship", "--var", "target=prod", "--var", "count=7")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		wantSuffix := filepath.Join("projects", "app", "work", "prod")
		if !strings.Contains(stdout, "prod|ship|7|prod-file|") || !strings.HasSuffix(stdout, wantSuffix+"|release") {
			t.Fatalf("stdout = %q, want rendered prod values ending in %q", stdout, wantSuffix+"|release")
		}
	})

	t.Run("missing required text input fails non-interactively", func(t *testing.T) {
		_, stderr, code := execute(t, root, "app", "deploy")
		if code != 2 || !strings.Contains(stderr, "missing required runtime variable") || !strings.Contains(stderr, "--var message=value") {
			t.Fatalf("expected missing var usage, code=%d stderr=%q", code, stderr)
		}
	})

	t.Run("invalid enum and integer values fail clearly", func(t *testing.T) {
		_, stderr, code := execute(t, root, "app", "deploy", "--var", "message=hello", "--var", "target=qa")
		if code != 2 || !strings.Contains(stderr, "variable target must be one of") {
			t.Fatalf("expected enum usage, code=%d stderr=%q", code, stderr)
		}
		_, stderr, code = execute(t, root, "app", "deploy", "--var", "message=hello", "--var", "count=1.5")
		if code != 2 || !strings.Contains(stderr, "variable count must be an integer") {
			t.Fatalf("expected integer usage, code=%d stderr=%q", code, stderr)
		}
	})

	t.Run("explain renders runtime values in json", func(t *testing.T) {
		stdout, stderr, code := execute(t, root, "explain", "app", "deploy", "--var", "message=preview", "--var", "target=prod", "--json")
		if code != 0 {
			t.Fatalf("code = %d stderr=%s", code, stderr)
		}
		for _, want := range []string{`"run":"printf '%s|%s|%s|%s|%s|%s' \"prod\" \"preview\"`, `"envFiles":["../../env/prod.env"]`, `"target":"prod"`, `"profile":"release"`} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("explain missing %s: %s", want, stdout)
			}
		}
	})
}

func TestTaskModesRenderVariablesAndCanLowerDanger(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
tasks:
  release/npm:
    desc: Release package
    danger: true
    defaultMode: publish
    modes:
      publish:
        values:
          args: ""
      dry-run:
        danger: false
        values:
          args: "--dry-run"
    run: printf "publish {{mode}} {{args}}"
    choices:
      all:
        label: All
        depends:
          - app:release/npm cli
      cli:
        label: CLI
`)

	stdout, stderr, code := execute(t, root, "app", "release/npm", "cli", "--mode", "dry-run")
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr)
	}
	if stdout != "publish dry-run --dry-run" {
		t.Fatalf("stdout = %q", stdout)
	}

	stdout, stderr, code = execute(t, root, "app", "release/npm", "all", "--mode", "dry-run")
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr)
	}
	if stdout != "publish dry-run --dry-run" {
		t.Fatalf("dependency stdout = %q", stdout)
	}

	_, stderr, code = execute(t, root, "app", "release/npm", "cli")
	if code != 2 || !strings.Contains(stderr, "requires --yes") {
		t.Fatalf("expected publish mode to require --yes, code=%d stderr=%s", code, stderr)
	}
}

func TestDynamicCompletionUsesDiscoveredTasks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".sd", "workspace.yaml"), `version: 1
discover:
  - projects/*/.sd/tasks.yaml
`)
	write(t, filepath.Join(root, "projects", "app", ".sd", "tasks.yaml"), `version: 1
project:
  id: app
  name: App
tasks:
  mobile/build/ios-dev:
    desc: iOS dev build
    run: echo ios
`)
	stdout, stderr, code := execute(t, root, cobraCompArgs("app", "mobile", "build", "")...)
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "ios-dev") || !strings.Contains(stdout, "iOS dev build") {
		t.Fatalf("completion missing ios-dev: stdout=%q", stdout)
	}
}

func cobraCompArgs(args ...string) []string {
	return append([]string{"__complete"}, args...)
}

func execute(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Execute(cli.Options{Args: args, WorkDir: dir, Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(""), NonInteractive: true})
	return stdout.String(), stderr.String(), code
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
