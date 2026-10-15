package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
)

type listedTask struct {
	Project string `json:"project,omitempty"`
	ID      string `json:"id"`
	Desc    string `json:"desc,omitempty"`
	Danger  bool   `json:"danger,omitempty"`
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
					return fmt.Errorf("unknown project: %s", args[0])
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
	var rows []listedTask
	for _, p := range ws.SortedProjects() {
		for _, id := range p.SortedTaskIDs() {
			t := p.Tasks[id]
			rows = append(rows, listedTask{Project: p.ID, ID: id, Desc: t.Desc, Danger: t.Danger})
			for _, choiceID := range t.SortedChoiceIDs() {
				choice := t.Choices[choiceID]
				desc := choice.Desc
				if desc == "" {
					desc = choice.Label
				}
				rows = append(rows, listedTask{Project: p.ID, ID: id + " " + choiceID, Desc: desc, Danger: t.Danger || choice.Danger})
			}
		}
	}
	aggIDs := make([]string, 0, len(ws.Aggregates))
	for id := range ws.Aggregates {
		aggIDs = append(aggIDs, id)
	}
	sort.Strings(aggIDs)
	for _, id := range aggIDs {
		t := ws.Aggregates[id]
		rows = append(rows, listedTask{ID: id, Desc: t.Desc, Danger: t.Danger})
	}
	if asJSON {
		return writeJSON(opts.Stdout, rows)
	}
	for _, row := range rows {
		prefix := row.ID
		if row.Project != "" {
			prefix = row.Project + " " + row.ID
		}
		desc := row.Desc
		if row.Danger {
			desc = strings.TrimSpace(desc + " ⚠ dangerous")
		}
		if desc != "" {
			fmt.Fprintf(opts.Stdout, "%-42s %s\n", prefix, desc)
		} else {
			fmt.Fprintln(opts.Stdout, prefix)
		}
	}
	return nil
}

func listOneProjectTasks(p *model.Project, opts Options, asJSON bool) error {
	var rows []listedTask
	for _, id := range p.SortedTaskIDs() {
		t := p.Tasks[id]
		rows = append(rows, listedTask{Project: p.ID, ID: id, Desc: t.Desc, Danger: t.Danger})
		for _, choiceID := range t.SortedChoiceIDs() {
			choice := t.Choices[choiceID]
			desc := choice.Desc
			if desc == "" {
				desc = choice.Label
			}
			rows = append(rows, listedTask{Project: p.ID, ID: id + " " + choiceID, Desc: desc, Danger: t.Danger || choice.Danger})
		}
	}
	if asJSON {
		return writeJSON(opts.Stdout, rows)
	}
	for _, row := range rows {
		desc := row.Desc
		if row.Danger {
			desc = strings.TrimSpace(desc + " ⚠ dangerous")
		}
		if desc != "" {
			fmt.Fprintf(opts.Stdout, "%-28s %s\n", row.ID, desc)
		} else {
			fmt.Fprintln(opts.Stdout, row.ID)
		}
	}
	return nil
}
