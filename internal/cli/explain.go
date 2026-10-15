package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
	"github.com/self-deprecated/sd/internal/runner"
)

// explanation is the output of `sd explain --json` (schema version 1).
// Project is empty for workspace aggregates.
type explanation struct {
	Version     int               `json:"version"`
	Project     string            `json:"project"`
	ID          string            `json:"id"`
	Choice      string            `json:"choice"`
	Mode        string            `json:"mode"`
	Desc        string            `json:"desc"`
	Run         string            `json:"run"`
	Dir         string            `json:"dir"`
	Env         map[string]string `json:"env"`
	EnvFiles    []string          `json:"envFiles"`
	RequiresEnv []string          `json:"requiresEnv"`
	Danger      bool              `json:"danger"`
	Depends     []string          `json:"depends"`
	Scripts     []string          `json:"scripts"`
	Values      map[string]string `json:"values"`
}

func explainCommand(opts Options, st *state) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <project> <task...>",
		Short: "Show what a task will run",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := discovery.LoadWorkspace(opts.WorkDir)
			if err != nil {
				return err
			}
			return explain(ws, args, opts, st)
		},
	}
}

func explain(ws *model.Workspace, args []string, opts Options, st *state) error {
	if task, ok := resolveAggregate(ws, args); ok {
		resolved, ok := task.ApplyMode(st.mode)
		if !ok {
			return fmt.Errorf("task %s has no mode %q", task.ID, st.mode)
		}
		task = resolved
		var err error
		task, err = runner.ResolveRuntimeInputs(task.ID, task, runOptions(opts, st))
		if err != nil {
			return err
		}
		exp := explanation{ID: task.ID, Mode: task.Mode, Desc: task.Desc, Run: task.Run, Dir: ws.Root, Env: task.Env, EnvFiles: task.EnvFiles, RequiresEnv: task.RequiresEnv, Danger: task.Danger, Depends: task.Depends, Scripts: task.Scripts, Values: task.Values}
		return writeExplanation(exp, opts, st.json)
	}
	project, ok := ws.ProjectByID(args[0])
	if !ok {
		if inProject, ok := inCurrentProject(ws, opts.WorkDir, args); ok {
			return explain(ws, inProject, opts, st)
		}
		return runner.UsageError{Message: unknownProjectMessage(ws, args[0])}
	}
	if len(args) == 1 {
		return childUsage(project.ID, model.ChildTasks(project.Tasks, ""))
	}
	task, extra, ok := resolveProjectTask(project, args[1:])
	if !ok {
		return childUsage(project.ID+" "+strings.Join(args[1:], " "), model.ChildTasks(project.Tasks, model.JoinPath(args[1:])))
	}
	if len(task.Choices) > 0 {
		if len(extra) == 0 {
			return choiceUsage(project.ID+" "+task.ID, task)
		}
		choiceTask, ok := task.ChoiceTask(extra[0])
		if !ok {
			return choiceUsage(project.ID+" "+task.ID, task)
		}
		task = choiceTask
	}
	resolved, ok := task.ApplyMode(st.mode)
	if !ok {
		return fmt.Errorf("task %s:%s has no mode %q", project.ID, task.ID, st.mode)
	}
	task = resolved
	var err error
	task, err = runner.ResolveRuntimeInputs(project.ID+":"+task.ID, task, runOptions(opts, st))
	if err != nil {
		return err
	}
	exp := explanation{Project: project.ID, ID: task.ID, Choice: task.Choice, Mode: task.Mode, Desc: task.Desc, Run: task.Run, Dir: project.TaskWorkingDir(task), Env: task.Env, EnvFiles: task.EnvFiles, RequiresEnv: task.RequiresEnv, Danger: task.Danger, Depends: task.Depends, Scripts: task.Scripts, Values: task.Values}
	return writeExplanation(exp, opts, st.json)
}

func writeExplanation(exp explanation, opts Options, asJSON bool) error {
	if asJSON {
		exp.Version = 1
		exp.Env = nonNilMap(exp.Env)
		exp.Values = nonNilMap(exp.Values)
		for _, list := range []*[]string{&exp.EnvFiles, &exp.RequiresEnv, &exp.Depends, &exp.Scripts} {
			if *list == nil {
				*list = []string{}
			}
		}
		return writeJSON(opts.Stdout, exp)
	}
	if exp.Project != "" {
		if exp.Choice != "" {
			fmt.Fprintf(opts.Stdout, "%s:%s %s\n", exp.Project, exp.ID, exp.Choice)
		} else {
			fmt.Fprintf(opts.Stdout, "%s:%s\n", exp.Project, exp.ID)
		}
	} else {
		fmt.Fprintln(opts.Stdout, exp.ID)
	}
	if exp.Mode != "" {
		fmt.Fprintf(opts.Stdout, "  mode:        %s\n", exp.Mode)
	}
	if exp.Desc != "" {
		fmt.Fprintf(opts.Stdout, "  description: %s\n", exp.Desc)
	}
	if exp.Dir != "" {
		if rel, err := filepath.Rel(opts.WorkDir, exp.Dir); err == nil && !strings.HasPrefix(rel, "..") {
			fmt.Fprintf(opts.Stdout, "  directory:   %s\n", rel)
		} else {
			fmt.Fprintf(opts.Stdout, "  directory:   %s\n", exp.Dir)
		}
	}
	if exp.Run != "" {
		fmt.Fprintf(opts.Stdout, "  command:     %s\n", exp.Run)
	}
	if len(exp.Depends) > 0 {
		fmt.Fprintf(opts.Stdout, "  depends:     %s\n", strings.Join(exp.Depends, ", "))
	}
	if len(exp.Scripts) > 0 {
		fmt.Fprintf(opts.Stdout, "  scripts:     %s\n", strings.Join(exp.Scripts, ", "))
	}
	if len(exp.EnvFiles) > 0 {
		fmt.Fprintf(opts.Stdout, "  envFiles:    %s\n", strings.Join(exp.EnvFiles, ", "))
	}
	if exp.Danger {
		fmt.Fprintln(opts.Stdout, "  danger:      true (--yes required non-interactively)")
	}
	if len(exp.RequiresEnv) > 0 {
		fmt.Fprintf(opts.Stdout, "  requiresEnv: %s\n", strings.Join(exp.RequiresEnv, ", "))
	}
	if len(exp.Values) > 0 {
		fmt.Fprintf(opts.Stdout, "  values:      %s\n", formatValues(exp.Values))
	}
	return nil
}

func formatValues(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ", ")
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
