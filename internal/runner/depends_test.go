package runner

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/self-deprecated/sd/internal/model"
)

func dependsWorkspace(t *testing.T, aggregates map[string]model.Task, tasks map[string]model.Task) *model.Workspace {
	t.Helper()
	root := t.TempDir()
	for id, task := range aggregates {
		task.ID = id
		aggregates[id] = task
	}
	for id, task := range tasks {
		task.ID = id
		task.ProjectID = "app"
		tasks[id] = task
	}
	return &model.Workspace{
		Root:       root,
		Aggregates: aggregates,
		Projects:   []*model.Project{{ID: "app", Path: root, Tasks: tasks}},
		Aliases:    map[string]string{"app": "app"},
	}
}

func runRef(t *testing.T, ws *model.Workspace, ref string, opts Options) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Stdout, opts.Stderr = &out, &out
	result := make(chan error, 1)
	go func() { result <- RunReference(ws, ref, nil, opts) }()
	select {
	case err := <-result:
		return out.String(), err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not finish", ref)
		return "", nil
	}
}

func TestDependencyCyclesAreReported(t *testing.T) {
	ws := dependsWorkspace(t, map[string]model.Task{
		"self": {Depends: []string{"self"}},
		"a":    {Depends: []string{"b"}},
		"b":    {Depends: []string{"app:c"}},
	}, map[string]model.Task{
		"c": {Depends: []string{"a"}, Run: "printf c"},
	})
	for ref, want := range map[string]string{"self": "dependency cycle: self → self", "a": "dependency cycle: a → b → app:c → a"} {
		if _, err := runRef(t, ws, ref, Options{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err = %v, want %q", ref, err, want)
		}
	}
}

func TestProjectTaskRunsItsCommandAfterDependencies(t *testing.T) {
	ws := dependsWorkspace(t, nil, map[string]model.Task{
		"check": {Run: "printf check"},
		"build": {Depends: []string{"app:check"}, Run: "printf %s -build"},
	})
	out, err := runRef(t, ws, "app:build", Options{})
	if err != nil || out != "check-build" {
		t.Fatalf("out = %q err = %v", out, err)
	}
}

func TestDependenciesInheritTheEffectiveMode(t *testing.T) {
	modes := map[string]model.TaskMode{"dry": {}, "wet": {}}
	ws := dependsWorkspace(t, map[string]model.Task{
		"release":         {DefaultMode: "dry", Modes: modes, Depends: []string{"app:publish", "app:check"}},
		"release/strict":  {DefaultMode: "dry", Modes: modes, Depends: []string{"app:wetonly"}},
		"release/default": {Depends: []string{"app:publish"}},
	}, map[string]model.Task{
		"publish": {DefaultMode: "wet", Modes: modes, Run: "printf '[publish {{mode}}]'"},
		"check":   {Run: "printf '[check]'"},
		"wetonly": {DefaultMode: "wet", Modes: map[string]model.TaskMode{"wet": {}}, Run: "printf wet"},
	})
	cases := []struct {
		name, ref, mode, out, err string
	}{
		{"default mode reaches dependencies", "release", "", "[publish dry][check]", ""},
		{"explicit mode reaches dependencies", "release", "wet", "[publish wet][check]", ""},
		{"dependency without the mode is an error", "release/strict", "", "", `has no mode "dry"`},
		{"parent without modes leaves dependency defaults", "release/default", "", "[publish wet]", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runRef(t, ws, tc.ref, Options{Mode: tc.mode})
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil || out != tc.out {
				t.Fatalf("out = %q err = %v, want %q", out, err, tc.out)
			}
		})
	}
}
