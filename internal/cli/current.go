package cli

import (
	"path/filepath"
	"strings"

	"github.com/self-deprecated/sd/internal/model"
)

// currentProject returns the only project of a single-project workspace, or
// the innermost project whose directory contains dir.
func currentProject(ws *model.Workspace, dir string) (*model.Project, bool) {
	if len(ws.Projects) == 1 {
		return ws.Projects[0], true
	}
	var current *model.Project
	for _, project := range ws.Projects {
		rel, err := filepath.Rel(project.Path, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if current == nil || len(project.Path) > len(current.Path) {
			current = project
		}
	}
	return current, current != nil
}

// inCurrentProject prefixes args with the current project's id when args
// name one of its tasks rather than a project.
func inCurrentProject(ws *model.Workspace, dir string, args []string) ([]string, bool) {
	project, ok := currentProject(ws, dir)
	if !ok {
		return nil, false
	}
	if _, _, ok := resolveProjectTask(project, args); !ok && len(model.ChildTasks(project.Tasks, model.JoinPath(args))) == 0 {
		return nil, false
	}
	return append([]string{project.ID}, args...), true
}
