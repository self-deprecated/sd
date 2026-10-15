package cli

import (
	"strings"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
)

func complete(workdir string, args []string, toComplete string) []string {
	ws, err := discovery.LoadWorkspace(workdir)
	if err != nil {
		return nil
	}
	if len(args) == 0 {
		var out []string
		for _, p := range ws.SortedProjects() {
			if strings.HasPrefix(p.ID, toComplete) {
				out = append(out, p.ID+"\t"+p.Name)
			}
		}
		for _, name := range []string{"projects", "tasks", "explain", "help"} {
			if strings.HasPrefix(name, toComplete) {
				out = append(out, name)
			}
		}
		for _, child := range aggregateChildren(ws, "") {
			name := strings.Split(child.ID, "/")[0]
			if strings.HasPrefix(name, toComplete) {
				out = append(out, name+"\t"+child.Desc)
			}
		}
		return dedupe(out)
	}
	if p, ok := ws.ProjectByID(args[0]); ok {
		if task, _, ok := resolveProjectTask(p, args[1:]); ok && len(task.Choices) > 0 {
			var out []string
			for _, id := range task.SortedChoiceIDs() {
				choice := task.Choices[id]
				desc := choice.Desc
				if desc == "" {
					desc = choice.Label
				}
				if strings.HasPrefix(id, toComplete) {
					out = append(out, id+"\t"+desc)
				}
			}
			return dedupe(out)
		}
		prefix := model.JoinPath(args[1:])
		children := model.ChildTasks(p.Tasks, prefix)
		var out []string
		for _, child := range children {
			name := lastSegment(child.ID)
			if strings.HasPrefix(name, toComplete) {
				out = append(out, name+"\t"+child.Desc)
			}
		}
		return dedupe(out)
	}
	prefix := model.JoinPath(args)
	var out []string
	for _, child := range aggregateChildren(ws, prefix) {
		name := lastSegment(child.ID)
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name+"\t"+child.Desc)
		}
	}
	return dedupe(out)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
