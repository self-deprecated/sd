package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/model"
)

var ErrNoWorkspace = errors.New("no .sd workspace or task manifest found")

func LoadWorkspace(start string) (*model.Workspace, error) {
	root, mode, err := FindRoot(start)
	if err != nil {
		return nil, err
	}
	if mode == "workspace" {
		return loadMonorepo(root)
	}
	return loadStandalone(root)
}

func FindRoot(start string) (root string, mode string, err error) {
	if start == "" {
		start = "."
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	abs = existingAncestor(abs)
	var taskRoot string
	for {
		if fileExists(filepath.Join(abs, ".sd", "workspace.yaml")) {
			return abs, "workspace", nil
		}
		if taskRoot == "" && fileExists(filepath.Join(abs, ".sd", "tasks.yaml")) {
			taskRoot = abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			if taskRoot != "" {
				return taskRoot, "standalone", nil
			}
			return "", "", ErrNoWorkspace
		}
		abs = parent
	}
}

func loadMonorepo(root string) (*model.Workspace, error) {
	wf, err := config.ReadWorkspace(filepath.Join(root, ".sd", "workspace.yaml"))
	if err != nil {
		return nil, err
	}
	ws := &model.Workspace{
		Root:       root,
		ID:         wf.Workspace.ID,
		Name:       wf.Workspace.Name,
		Aggregates: map[string]model.Task{},
		Aliases:    map[string]string{},
	}
	if ws.ID == "" {
		ws.ID = filepath.Base(root)
	}
	if ws.Name == "" {
		ws.Name = ws.ID
	}

	patterns := wf.Discover
	if len(patterns) == 0 {
		patterns = []string{".sd/tasks.yaml", "projects/*/.sd/tasks.yaml", "projects/*/*/.sd/tasks.yaml", "projects/*/tools/*/.sd/tasks.yaml"}
	}
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, fmt.Errorf("bad discover pattern %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if seen[match] {
				continue
			}
			seen[match] = true
			project, err := loadProject(match)
			if err != nil {
				return nil, err
			}
			ws.Projects = append(ws.Projects, project)
		}
	}
	if fileExists(filepath.Join(root, ".sd", "aggregates.yaml")) {
		af, err := config.ReadAggregates(filepath.Join(root, ".sd", "aggregates.yaml"))
		if err != nil {
			return nil, err
		}
		for id, task := range af.Tasks {
			task.ID = id
			ws.Aggregates[id] = task
		}
	}
	indexAliases(ws)
	return ws, nil
}

func loadStandalone(root string) (*model.Workspace, error) {
	project, err := loadProject(filepath.Join(root, ".sd", "tasks.yaml"))
	if err != nil {
		return nil, err
	}
	ws := &model.Workspace{
		Root:       root,
		ID:         project.ID,
		Name:       project.Name,
		Projects:   []*model.Project{project},
		Aggregates: map[string]model.Task{},
		Aliases:    map[string]string{},
	}
	indexAliases(ws)
	return ws, nil
}

func loadProject(path string) (*model.Project, error) {
	tf, err := config.ReadTaskFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	base := filepath.Dir(filepath.Dir(path))
	project := tf.Project
	if project.ID == "" {
		project.ID = filepath.Base(base)
	}
	if project.Name == "" {
		project.Name = project.ID
	}
	projectRoot := base
	if project.Dir != "" {
		if filepath.IsAbs(project.Dir) {
			projectRoot = project.Dir
		} else {
			projectRoot = filepath.Join(base, project.Dir)
		}
	}
	project.Path = filepath.Clean(projectRoot)
	project.Scripts = tf.Scripts
	for name, script := range project.Scripts {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(script.Run) == "" {
			return nil, fmt.Errorf("project %s shared script %q has no run command", project.ID, name)
		}
	}
	project.Tasks = map[string]model.Task{}
	for id, task := range tf.Tasks {
		if err := validateTaskScripts(project.ID+":"+id, task, project.Scripts); err != nil {
			return nil, err
		}
		task.ID = id
		task.ProjectID = project.ID
		project.Tasks[id] = task
	}
	for id, task := range tf.Passthrough {
		if err := validateTaskScripts(project.ID+":"+id, task, project.Scripts); err != nil {
			return nil, err
		}
		task.ID = id
		task.ProjectID = project.ID
		task.Passthrough = true
		project.Tasks[id] = task
	}
	return &project, nil
}

func validateTaskScripts(label string, task model.Task, scripts map[string]model.SharedScript) error {
	validate := func(context string, names []string) error {
		for _, name := range names {
			if _, ok := scripts[name]; !ok {
				return fmt.Errorf("task %s%s references unknown shared script %q", label, context, name)
			}
		}
		return nil
	}
	if err := validate("", task.Scripts); err != nil {
		return err
	}
	for choiceID, choice := range task.Choices {
		if err := validate(" choice "+choiceID, choice.Scripts); err != nil {
			return err
		}
		for modeID, mode := range choice.Modes {
			if err := validate(" choice "+choiceID+" mode "+modeID, mode.Scripts); err != nil {
				return err
			}
		}
	}
	for modeID, mode := range task.Modes {
		if err := validate(" mode "+modeID, mode.Scripts); err != nil {
			return err
		}
	}
	return nil
}

func indexAliases(ws *model.Workspace) {
	if ws.Aliases == nil {
		ws.Aliases = map[string]string{}
	}
	for _, project := range ws.Projects {
		ws.Aliases[project.ID] = project.ID
		for _, alias := range project.Aliases {
			ws.Aliases[alias] = project.ID
		}
	}
}

func existingAncestor(path string) string {
	for {
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
