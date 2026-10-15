package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
	"github.com/self-deprecated/sd/internal/runner"
)

// listedTask is one row of `sd tasks --json` (schema version 1). Danger is
// the task's danger under its default mode.
type listedTask struct {
	Project     string   `json:"project"`
	ID          string   `json:"id"`
	Choice      string   `json:"choice"`
	Desc        string   `json:"desc"`
	Danger      bool     `json:"danger"`
	Modes       []string `json:"modes"`
	DefaultMode string   `json:"defaultMode"`
}

type taskList struct {
	Version int          `json:"version"`
	Tasks   []listedTask `json:"tasks"`
}

func listTask(project string, task model.Task) listedTask {
	modes := task.SortedModeIDs()
	effective, ok := task.ApplyMode("")
	if !ok {
		effective = task
	}
	return listedTask{Project: project, ID: task.ID, Choice: task.Choice, Desc: task.Desc, Danger: effective.Danger, Modes: modes, DefaultMode: task.DefaultMode}
}

func projectTaskRows(p *model.Project) []listedTask {
	rows := []listedTask{}
	for _, id := range p.SortedTaskIDs() {
		task := p.Tasks[id]
		rows = append(rows, listTask(p.ID, task))
		for _, choiceID := range task.SortedChoiceIDs() {
			choiceTask, _ := task.ChoiceTask(choiceID)
			rows = append(rows, listTask(p.ID, choiceTask))
		}
	}
	return rows
}

func (row listedTask) name() string {
	return strings.TrimSpace(row.ID + " " + row.Choice)
}

func tasksCommand(opts Options, st *state) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "tasks [project]",
		Short: "List tasks (of the current project, if any)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := discovery.LoadWorkspace(opts.WorkDir)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				p, ok := ws.ProjectByID(args[0])
				if !ok {
					return runner.UsageError{Message: unknownProjectMessage(ws, args[0])}
				}
				return listOneProjectTasks(p, opts, st.json)
			}
			if p, ok := currentProject(ws, opts.WorkDir); ok && !all {
				return listOneProjectTasks(p, opts, st.json)
			}
			return listAllTasks(ws, opts, st.json)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "list every project's tasks and the workspace aggregates")
	return cmd
}

func listAllTasks(ws *model.Workspace, opts Options, asJSON bool) error {
	rows := []listedTask{}
	for _, p := range ws.SortedProjects() {
		rows = append(rows, projectTaskRows(p)...)
	}
	aggIDs := make([]string, 0, len(ws.Aggregates))
	for id := range ws.Aggregates {
		aggIDs = append(aggIDs, id)
	}
	sort.Strings(aggIDs)
	for _, id := range aggIDs {
		rows = append(rows, listTask("", ws.Aggregates[id]))
	}
	if asJSON {
		return writeJSON(opts.Stdout, taskList{Version: 1, Tasks: rows})
	}
	for _, row := range rows {
		prefix := row.name()
		if row.Project != "" {
			prefix = row.Project + " " + prefix
		}
		printTaskRow(opts, "%-42s %s\n", prefix, row)
	}
	return nil
}

func listOneProjectTasks(p *model.Project, opts Options, asJSON bool) error {
	rows := projectTaskRows(p)
	if asJSON {
		return writeJSON(opts.Stdout, taskList{Version: 1, Tasks: rows})
	}
	for _, row := range rows {
		printTaskRow(opts, "%-28s %s\n", row.name(), row)
	}
	return nil
}

func printTaskRow(opts Options, format, name string, row listedTask) {
	desc := row.Desc
	if row.Danger {
		desc = strings.TrimSpace(desc + " ⚠ dangerous")
	}
	if desc == "" {
		fmt.Fprintln(opts.Stdout, name)
		return
	}
	fmt.Fprintf(opts.Stdout, format, name, desc)
}
