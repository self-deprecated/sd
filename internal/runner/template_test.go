package runner

import (
	"bytes"
	"testing"

	"github.com/self-deprecated/sd/internal/model"
)

func TestRuntimeValuesAreNeverParsedAsShellCode(t *testing.T) {
	cases := []struct {
		name, run, value, out string
	}{
		{"double quoted", `printf '%s' "{{message}}"`, `$(printf injected) "quoted" \ back`, `$(printf injected) "quoted" \ back`},
		{"single quoted", `printf '%s' '{{message}}'`, `it's $(printf injected)`, `it's $(printf injected)`},
		{"unquoted splits words", `printf '[%s]' {{message}}`, `a b`, `[a][b]`},
		{"unquoted suffix", `printf '[%s]' publish{{message}}`, ` --dry-run`, `[publish][--dry-run]`},
		{"unquoted command substitution stays literal", `printf '[%s]' {{message}}`, `$(printf injected)`, `[$(printf][injected)]`},
		{"unknown placeholder stays literal", `printf '%s' '{{nope}}'`, `x`, `{{nope}}`},
		{"apostrophe in a comment", "# Don't change the greeting.\nprintf '%s' '{{message}}'", `hello`, `hello`},
		{"single quotes inside a command substitution", `printf '%s' "$(printf '%s' '{{message}}')"`, `hello`, `hello`},
		{"ansi-c quotes", `printf '%s' $'[{{message}}]\t'`, `it's`, "[it's]\t"},
		{"quoted heredoc keeps the placeholder", "cat <<'EOF'\n{{message}}\nEOF", `$(printf x)`, "{{message}}\n"},
		{"newline in a comment", "# {{message}}\nprintf SAFE", "hello\nprintf INJECTED\n#", "SAFE"},
		{"delimiter in a quoted heredoc", "cat <<'EOF'\n{{message}}\nEOF", "hello\nEOF\nprintf INJECTED\ncat <<'EOF'\n", "{{message}}\n"},
		{"unquoted heredoc", "cat <<EOF\n[{{message}}]\nEOF", `$(printf x)`, "[$(printf x)]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ws := &model.Workspace{Root: root}
			project := &model.Project{ID: "demo", Path: root}
			task := model.Task{ID: "say", Run: tc.run, Options: map[string]model.TaskOption{"message": {Type: "string"}}}
			var out bytes.Buffer
			err := RunProjectTask(ws, project, task, nil, Options{Stdout: &out, Stderr: &out, Vars: map[string]string{"message": tc.value}})
			if err != nil || out.String() != tc.out {
				t.Fatalf("out = %q err = %v, want %q", out.String(), err, tc.out)
			}
		})
	}
}
