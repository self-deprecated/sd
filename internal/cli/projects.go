package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
)

func projectsCommand(opts Options, st *state) *cobra.Command {
	return &cobra.Command{
		Use:     "projects",
		Aliases: []string{"list"},
		Short:   "List discovered Projects",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := discovery.LoadWorkspace(opts.WorkDir)
			if err != nil {
				return err
			}
			return listProjects(ws, opts.Stdout, st.json)
		},
	}
}

func listProjects(ws *model.Workspace, out interface{ Write([]byte) (int, error) }, asJSON bool) error {
	if asJSON {
		return writeJSON(out, ws.SortedProjects())
	}
	currentArea := "\x00"
	for _, project := range ws.SortedProjects() {
		area := project.WorkArea
		if area == "" {
			area = "Projects"
		}
		if area != currentArea {
			if currentArea != "\x00" {
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "%s:\n", area)
			currentArea = area
		}
		path := project.Path
		if rel, err := filepath.Rel(ws.Root, project.Path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
		bits := []string{path}
		if project.Visibility != "" {
			bits = append(bits, project.Visibility)
		}
		fmt.Fprintf(out, "  %-18s %s\n", project.ID, strings.Join(bits, " · "))
	}
	return nil
}
