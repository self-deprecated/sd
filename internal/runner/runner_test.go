package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/self-deprecated/sd/internal/model"
)

func TestRunProjectTaskSkipsWhenChangedDoesNotMatch(t *testing.T) {
	t.Setenv("SD_CHANGED_FILES", "projects/landing/src/main.ts")
	var out bytes.Buffer
	root := t.TempDir()
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "demo", Path: filepath.Join(root, "projects", "demo")}
	task := model.Task{ID: "mobile/bundle-check", Run: "exit 99", WhenChanged: []string{"apps/mobile/**"}}

	if err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v", err)
	}
	if got := out.String(); got == "" || !bytes.Contains([]byte(got), []byte("skipping demo:mobile/bundle-check")) {
		t.Fatalf("expected skip message, got %q", got)
	}
}

func TestRunProjectTaskRunsWhenChangedMatchesProjectLocalPattern(t *testing.T) {
	t.Setenv("SD_CHANGED_FILES", "projects/demo/packages/shared/src/index.ts")
	var out bytes.Buffer
	root := t.TempDir()
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "demo", Path: filepath.Join(root, "projects", "demo")}
	if err := os.MkdirAll(project.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "echo", Run: "printf ran", WhenChanged: []string{"packages/shared/**"}}

	if err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v", err)
	}
	if got := out.String(); got != "ran" {
		t.Fatalf("expected command output, got %q", got)
	}
}

func TestRunProjectTaskRunsWhenChangedMatchesWorkspaceRootPattern(t *testing.T) {
	t.Setenv("SD_CHANGED_FILES", ".sd/workspace.yaml")
	var out bytes.Buffer
	root := t.TempDir()
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "sd", Path: filepath.Join(root, "tools", "sd")}
	if err := os.MkdirAll(project.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "final-review", Run: "printf ran", WhenChanged: []string{"/.sd/**"}}

	if err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v", err)
	}
	if got := out.String(); got != "ran" {
		t.Fatalf("expected command output, got %q", got)
	}
}

func TestRunProjectTaskPromptsForMissingRuntimeInputs(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "app", Path: root}
	task := model.Task{
		ID:  "deploy",
		Run: `printf '%s/%s' "{{target}}" "{{count}}"`,
		Options: map[string]model.TaskOption{
			"target": {
				Type:   "enum",
				Prompt: "Target",
				Values: map[string]model.OptionValue{"dev": {}, "prod": {}},
			},
			"count": {Type: "number", Prompt: "Count"},
		},
	}

	err := RunProjectTask(ws, project, task, nil, Options{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("x\n3\nqa\nprod\n"), AllowPrompt: true})
	if err != nil {
		t.Fatalf("RunProjectTask returned error: %v\nstderr=%s", err, stderr.String())
	}
	if got := stdout.String(); got != "prod/3" {
		t.Fatalf("stdout = %q", got)
	}
	for _, want := range []string{"Target [dev/prod]", "variable target must be one of", "Count", "variable count must be an integer"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %s", want, stderr.String())
		}
	}
}

func TestRunProjectTaskLoadsEnvFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(strings.Join([]string{
		"SECRET=from-file",
		"QUOTED=hello world",
		"INLINE=kept#hash # stripped comment",
		"SINGLE='literal value' # stripped comment",
		"DOUBLE=\"line\\nvalue\" # stripped comment",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "app", Path: root}
	task := model.Task{
		ID:          "deploy",
		Run:         `printf '%s|%s|%s|%s|%s' "$SECRET" "$QUOTED" "$INLINE" "$SINGLE" "$DOUBLE"`,
		EnvFiles:    []string{".env"},
		RequiresEnv: []string{"SECRET"},
	}

	if err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v", err)
	}
	if got, want := out.String(), "from-file|hello world|kept#hash|literal value|line\nvalue"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestEnvFilesAreOptionalAndShellEnvOverridesFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SD_TEST_ENV_FILE_PRECEDENCE=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SD_TEST_ENV_FILE_PRECEDENCE", "from-shell")
	var out bytes.Buffer
	ws := &model.Workspace{Root: root}
	project := &model.Project{ID: "app", Path: root}
	task := model.Task{
		ID:          "deploy",
		Run:         `printf '%s/%s' "$SD_TEST_ENV_FILE_PRECEDENCE" "$SD_TEST_ENV_FILE_ONLY_IN_SHELL"`,
		EnvFiles:    []string{".env", ".env.missing"},
		RequiresEnv: []string{"SD_TEST_ENV_FILE_PRECEDENCE"},
		Env:         map[string]string{"SD_TEST_ENV_FILE_ONLY_IN_SHELL": "from-task"},
	}

	if err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("RunProjectTask returned error: %v", err)
	}
	if got, want := out.String(), "from-shell/from-task"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestChangedFilesHonorsExplicitEmptyEnvironment(t *testing.T) {
	t.Setenv("SD_CHANGED_FILES", "")
	files, err := detectChangedFiles(t.TempDir())
	if err != nil {
		t.Fatalf("detectChangedFiles returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected empty explicit changed file set, got %v", files)
	}
}

func TestMatchChangedPattern(t *testing.T) {
	cases := []struct {
		pattern string
		file    string
		want    bool
	}{
		{"projects/demo/**", "projects/demo/apps/mobile/App.tsx", true},
		{"projects/demo/packages/*/src/**", "projects/demo/packages/shared/src/index.ts", true},
		{"projects/api/**", "projects/web/flake.nix", false},
		{"projects/*", "projects/demo/flake.nix", false},
		{"projects/*/flake.nix", "projects/demo/nested/flake.nix", false},
	}
	for _, tc := range cases {
		if got := matchChangedPattern(tc.pattern, tc.file); got != tc.want {
			t.Fatalf("matchChangedPattern(%q, %q) = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}
