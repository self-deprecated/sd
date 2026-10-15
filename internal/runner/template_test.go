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
