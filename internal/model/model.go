package model

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type Workspace struct {
	Root       string            `json:"root"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Projects   []*Project        `json:"projects"`
	Aggregates map[string]Task   `json:"aggregates,omitempty"`
	Aliases    map[string]string `json:"-"`
}

type Project struct {
	ID         string                  `json:"id" yaml:"id"`
	Name       string                  `json:"name" yaml:"name"`
	Aliases    []string                `json:"aliases,omitempty" yaml:"aliases"`
	WorkArea   string                  `json:"workArea,omitempty" yaml:"workArea"`
	Visibility string                  `json:"visibility,omitempty" yaml:"visibility"`
	Dir        string                  `json:"-" yaml:"dir"`
	Path       string                  `json:"path" yaml:"-"`
	Tasks      map[string]Task         `json:"tasks" yaml:"-"`
	Scripts    map[string]SharedScript `json:"scripts,omitempty" yaml:"-"`
}

type SharedScript struct {
	Desc string `json:"desc,omitempty" yaml:"desc"`
	Run  string `json:"run" yaml:"run"`
}

type Task struct {
	ID          string                `json:"id" yaml:"-"`
	ProjectID   string                `json:"projectId,omitempty" yaml:"-"`
	Desc        string                `json:"desc,omitempty" yaml:"desc"`
	Run         string                `json:"run,omitempty" yaml:"run"`
	Dir         string                `json:"dir,omitempty" yaml:"dir"`
	Env         map[string]string     `json:"env,omitempty" yaml:"env"`
	EnvFiles    []string              `json:"envFiles,omitempty" yaml:"envFiles"`
	RequiresEnv []string              `json:"requiresEnv,omitempty" yaml:"requiresEnv"`
	Danger      bool                  `json:"danger,omitempty" yaml:"danger"`
	WhenChanged []string              `json:"whenChanged,omitempty" yaml:"whenChanged"`
	Depends     []string              `json:"depends,omitempty" yaml:"depends"`
	Scripts     []string              `json:"scripts,omitempty" yaml:"scripts"`
	Options     map[string]TaskOption `json:"options,omitempty" yaml:"options"`
	Choices     map[string]TaskChoice `json:"choices,omitempty" yaml:"choices"`
	Modes       map[string]TaskMode   `json:"modes,omitempty" yaml:"modes"`
	DefaultMode string                `json:"defaultMode,omitempty" yaml:"defaultMode"`
	Choice      string                `json:"choice,omitempty" yaml:"-"`
	Mode        string                `json:"mode,omitempty" yaml:"-"`
	Values      map[string]string     `json:"values,omitempty" yaml:"-"`
	Passthrough bool                  `json:"passthrough,omitempty" yaml:"-"`
	// RunTemplate is Run before RenderTask substituted values into it.
	RunTemplate string `json:"-" yaml:"-"`
}

// TaskMode is an execution mode for a task, such as publish vs dry-run.
// Modes mainly provide template variables, but may also override command
// details when a task needs mode-specific behavior.
type TaskMode struct {
	Label       string            `json:"label,omitempty" yaml:"label"`
	Desc        string            `json:"desc,omitempty" yaml:"desc"`
	Run         string            `json:"run,omitempty" yaml:"run"`
	Dir         string            `json:"dir,omitempty" yaml:"dir"`
	Env         map[string]string `json:"env,omitempty" yaml:"env"`
	EnvFiles    []string          `json:"envFiles,omitempty" yaml:"envFiles"`
	RequiresEnv []string          `json:"requiresEnv,omitempty" yaml:"requiresEnv"`
	Danger      *bool             `json:"danger,omitempty" yaml:"danger"`
	Scripts     []string          `json:"scripts,omitempty" yaml:"scripts"`
	Values      map[string]string `json:"values,omitempty" yaml:"values"`
}

type TaskOption struct {
	Type     string                 `json:"type,omitempty" yaml:"type"`
	Prompt   string                 `json:"prompt,omitempty" yaml:"prompt"`
	Desc     string                 `json:"desc,omitempty" yaml:"desc"`
	Default  string                 `json:"default,omitempty" yaml:"default"`
	Required *bool                  `json:"required,omitempty" yaml:"required"`
	Values   map[string]OptionValue `json:"values,omitempty" yaml:"values"`
}

type OptionValue struct {
	Label string            `json:"label,omitempty" yaml:"label"`
	Desc  string            `json:"desc,omitempty" yaml:"desc"`
	Vars  map[string]string `json:"vars,omitempty" yaml:"vars"`
}

type TaskChoice struct {
	Label       string              `json:"label,omitempty" yaml:"label"`
	Desc        string              `json:"desc,omitempty" yaml:"desc"`
	Run         string              `json:"run,omitempty" yaml:"run"`
	Dir         string              `json:"dir,omitempty" yaml:"dir"`
	Env         map[string]string   `json:"env,omitempty" yaml:"env"`
	EnvFiles    []string            `json:"envFiles,omitempty" yaml:"envFiles"`
	RequiresEnv []string            `json:"requiresEnv,omitempty" yaml:"requiresEnv"`
	Danger      bool                `json:"danger,omitempty" yaml:"danger"`
	WhenChanged []string            `json:"whenChanged,omitempty" yaml:"whenChanged"`
	Depends     []string            `json:"depends,omitempty" yaml:"depends"`
	Scripts     []string            `json:"scripts,omitempty" yaml:"scripts"`
	Values      map[string]string   `json:"values,omitempty" yaml:"values"`
	Modes       map[string]TaskMode `json:"modes,omitempty" yaml:"modes"`
	DefaultMode string              `json:"defaultMode,omitempty" yaml:"defaultMode"`
}

func (w *Workspace) ProjectByID(idOrAlias string) (*Project, bool) {
	if w == nil {
		return nil, false
	}
	canonical := idOrAlias
	if w.Aliases != nil {
		if resolved, ok := w.Aliases[idOrAlias]; ok {
			canonical = resolved
		}
	}
	for _, p := range w.Projects {
		if p.ID == canonical {
			return p, true
		}
	}
	return nil, false
}

func (w *Workspace) SortedProjects() []*Project {
	projects := append([]*Project(nil), w.Projects...)
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].WorkArea == projects[j].WorkArea {
			return projects[i].ID < projects[j].ID
		}
		return projects[i].WorkArea < projects[j].WorkArea
	})
	return projects
}

func (p *Project) SortedTaskIDs() []string {
	ids := make([]string, 0, len(p.Tasks))
	for id := range p.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (p *Project) Task(id string) (Task, bool) {
	t, ok := p.Tasks[id]
	return t, ok
}

func (p *Project) TaskWorkingDir(t Task) string {
	if t.Dir == "" {
		return p.Path
	}
	if filepath.IsAbs(t.Dir) {
		return t.Dir
	}
	return filepath.Join(p.Path, t.Dir)
}

func (t Task) SortedChoiceIDs() []string {
	ids := make([]string, 0, len(t.Choices))
	for id := range t.Choices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (t Task) ChoiceTask(choiceID string) (Task, bool) {
	choice, ok := t.Choices[choiceID]
	if !ok {
		return Task{}, false
	}
	resolved := t
	resolved.Choice = choiceID
	resolved.Values = ResolveChoiceValues(t, choice)
	if choice.Desc != "" {
		resolved.Desc = choice.Desc
	} else if choice.Label != "" {
		resolved.Desc = choice.Label
	}
	if choice.Run != "" {
		resolved.Run = choice.Run
	}
	if choice.Dir != "" {
		resolved.Dir = choice.Dir
	}
	resolved.Env = mergeStringMaps(t.Env, choice.Env)
	resolved.EnvFiles = append(append([]string{}, t.EnvFiles...), choice.EnvFiles...)
	resolved.RequiresEnv = append(append([]string{}, t.RequiresEnv...), choice.RequiresEnv...)
	resolved.Danger = t.Danger || choice.Danger
	resolved.Modes = mergeTaskModes(t.Modes, choice.Modes)
	if choice.DefaultMode != "" {
		resolved.DefaultMode = choice.DefaultMode
	}
	if len(choice.WhenChanged) > 0 {
		resolved.WhenChanged = append([]string{}, choice.WhenChanged...)
	}
	if len(choice.Depends) > 0 {
		resolved.Depends = append([]string{}, choice.Depends...)
		// A choice that only lists dependencies groups other choices; it
		// must not also run the task's own command.
		if choice.Run == "" {
			resolved.Run = ""
		}
	}
	resolved.Scripts = appendUnique(t.Scripts, choice.Scripts...)
	return resolved, true
}

func (t Task) SortedModeIDs() []string {
	ids := make([]string, 0, len(t.Modes))
	for id := range t.Modes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (t Task) ModeCycleIDs() []string {
	ids := t.SortedModeIDs()
	if t.DefaultMode == "" || len(ids) == 0 {
		return ids
	}
	out := []string{t.DefaultMode}
	for _, id := range ids {
		if id != t.DefaultMode {
			out = append(out, id)
		}
	}
	return out
}

func (t Task) ApplyMode(requestedMode string) (Task, bool) {
	modeID := requestedMode
	if modeID == "" {
		modeID = t.DefaultMode
	}
	if modeID == "" {
		return t, true
	}
	mode, ok := t.Modes[modeID]
	if !ok {
		return Task{}, false
	}
	resolved := t
	resolved.Mode = modeID
	resolved.Values = mergeStringMaps(t.Values, mode.Values)
	if resolved.Values == nil {
		resolved.Values = map[string]string{}
	}
	resolved.Values["mode"] = modeID
	resolved.Env = mergeStringMaps(t.Env, mode.Env)
	resolved.EnvFiles = append(append([]string{}, t.EnvFiles...), mode.EnvFiles...)
	resolved.RequiresEnv = append(append([]string{}, t.RequiresEnv...), mode.RequiresEnv...)
	if mode.Desc != "" {
		resolved.Desc = mode.Desc
	} else if mode.Label != "" {
		resolved.Desc = mode.Label
	}
	if mode.Run != "" {
		resolved.Run = mode.Run
	}
	if mode.Dir != "" {
		resolved.Dir = mode.Dir
	}
	if mode.Danger != nil {
		resolved.Danger = *mode.Danger
	}
	resolved.Scripts = appendUnique(t.Scripts, mode.Scripts...)
	return resolved, true
}

func ResolveChoiceValues(t Task, choice TaskChoice) map[string]string {
	if len(choice.Values) == 0 {
		return nil
	}
	values := map[string]string{}
	for key, value := range choice.Values {
		values[key] = value
	}
	return values
}

// ResolveOptionValues resolves template values. Option defaults come first,
// then variables derived from each option's selected value, then the
// overlays (choice and mode values, explicit --var values) in order, so an
// explicit value always wins. Two selected values deriving different values
// for the same variable is an error.
func ResolveOptionValues(options map[string]TaskOption, overlays ...map[string]string) (map[string]string, error) {
	explicit := map[string]string{}
	for _, overlay := range overlays {
		for key, value := range overlay {
			explicit[key] = value
		}
	}
	values := map[string]string{}
	for name, option := range options {
		if option.Default != "" {
			values[name] = option.Default
		}
	}
	for key, value := range explicit {
		values[key] = value
	}
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	derived := map[string]string{}
	derivedFrom := map[string]string{}
	for _, name := range names {
		selected, ok := options[name].Values[values[name]]
		if !ok {
			continue
		}
		for key, value := range selected.Vars {
			if _, ok := explicit[key]; ok {
				continue
			}
			if previous, ok := derived[key]; ok && previous != value {
				return nil, fmt.Errorf("options %s and %s set %q to different values", derivedFrom[key], name, key)
			}
			derived[key] = value
			derivedFrom[key] = name
		}
	}
	for key, value := range derived {
		values[key] = value
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

func RenderTask(t Task, values map[string]string) Task {
	if values == nil {
		values = map[string]string{}
	}
	resolved := t
	resolved.Values = values
	resolved.RunTemplate = t.Run
	resolved.Run = RenderTemplate(resolved.Run, values)
	resolved.Dir = RenderTemplate(resolved.Dir, values)
	if len(resolved.Env) > 0 {
		resolved.Env = mergeStringMaps(nil, resolved.Env)
		for key, value := range resolved.Env {
			resolved.Env[key] = RenderTemplate(value, values)
		}
	}
	if len(resolved.EnvFiles) > 0 {
		resolved.EnvFiles = append([]string{}, resolved.EnvFiles...)
		for i, path := range resolved.EnvFiles {
			resolved.EnvFiles[i] = RenderTemplate(path, values)
		}
	}
	return resolved
}

func TemplateVariables(t Task) map[string]bool {
	refs := map[string]bool{}
	collectTemplateVariables(refs, t.Run)
	collectTemplateVariables(refs, t.Dir)
	for _, value := range t.Env {
		collectTemplateVariables(refs, value)
	}
	for _, path := range t.EnvFiles {
		collectTemplateVariables(refs, path)
	}
	return refs
}

func collectTemplateVariables(refs map[string]bool, input string) {
	for len(input) > 0 {
		start := strings.Index(input, "{{")
		if start < 0 {
			return
		}
		rest := input[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return
		}
		key := strings.TrimSpace(rest[:end])
		if key != "" {
			refs[key] = true
		}
		input = rest[end+2:]
	}
}

func RenderTemplate(input string, values map[string]string) string {
	out := input
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = strings.ReplaceAll(out, "{{"+key+"}}", values[key])
	}
	return out
}

// ShellCommand renders a run template for execution. Each {{name}} becomes a
// reference to a shell variable assigned the value, so values are never
// parsed as shell code; an unquoted placeholder still splits into words.
// The script is parsed to find each placeholder's quoting context: inside
// single quotes the reference is spliced in by closing and reopening them,
// and in comments and quoted here-documents, which the shell does not
// expand, the value is written as text.
func ShellCommand(template string, values map[string]string) string {
	type placeholder struct {
		start, end int
		key        string
	}
	var placeholders []placeholder
	for i := 0; i < len(template); {
		start := strings.Index(template[i:], "{{")
		if start < 0 {
			break
		}
		start += i
		end := strings.Index(template[start+2:], "}}")
		if end < 0 {
			break
		}
		end += start + 4
		if _, ok := values[template[start+2:end-2]]; ok {
			placeholders = append(placeholders, placeholder{start, end, template[start+2 : end-2]})
		}
		i = end
	}
	if len(placeholders) == 0 {
		return template
	}

	// Mask placeholders with same-length words so parser offsets match.
	masked := []byte(template)
	for _, p := range placeholders {
		for i := p.start; i < p.end; i++ {
			masked[i] = 'x'
		}
	}
	type span struct {
		start, end int
		dollar     bool
	}
	var singleQuoted, literal []span
	file, err := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(syntax.LangBash)).Parse(bytes.NewReader(masked), "")
	if err == nil {
		syntax.Walk(file, func(node syntax.Node) bool {
			switch n := node.(type) {
			case *syntax.SglQuoted:
				singleQuoted = append(singleQuoted, span{int(n.Pos().Offset()), int(n.End().Offset()), n.Dollar})
			case *syntax.Comment:
				literal = append(literal, span{start: int(n.Pos().Offset()), end: int(n.End().Offset())})
			case *syntax.Redirect:
				if n.Hdoc != nil && quotedWord(n.Word) {
					literal = append(literal, span{start: int(n.Hdoc.Pos().Offset()), end: int(n.Hdoc.End().Offset())})
				}
			}
			return true
		})
	}
	within := func(spans []span, offset int) (span, bool) {
		for _, s := range spans {
			if offset >= s.start && offset < s.end {
				return s, true
			}
		}
		return span{}, false
	}

	var assignments, body strings.Builder
	refs := map[string]string{}
	last := 0
	for _, p := range placeholders {
		body.WriteString(template[last:p.start])
		last = p.end
		if _, ok := within(literal, p.start); ok {
			body.WriteString(values[p.key])
			continue
		}
		ref, ok := refs[p.key]
		if !ok {
			ref = fmt.Sprintf("__sd_var_%d", len(refs))
			refs[p.key] = ref
			fmt.Fprintf(&assignments, "%s=%s\n", ref, singleQuote(values[p.key]))
		}
		if quoted, ok := within(singleQuoted, p.start); ok {
			reopen := "'"
			if quoted.dollar {
				reopen = "$'"
			}
			body.WriteString(`'"${` + ref + `}"` + reopen)
		} else {
			body.WriteString("${" + ref + "}")
		}
	}
	body.WriteString(template[last:])
	return assignments.String() + body.String()
}

// quotedWord reports whether a here-document delimiter is quoted, which
// stops the shell expanding the here-document.
func quotedWord(word *syntax.Word) bool {
	for _, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		case *syntax.Lit:
			if strings.Contains(part.Value, "\\") {
				return true
			}
		}
	}
	return false
}

// singleQuote quotes value for the shell. It writes an embedded quote as
// '"'"' because the interpreter keeps the backslash of '\'' in assignments.
func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func mergeTaskModes(a, b map[string]TaskMode) map[string]TaskMode {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := map[string]TaskMode{}
	for key, value := range a {
		out[key] = value
	}
	for key, value := range b {
		base := out[key]
		out[key] = mergeTaskMode(base, value)
	}
	return out
}

func mergeTaskMode(a, b TaskMode) TaskMode {
	out := a
	if b.Label != "" {
		out.Label = b.Label
	}
	if b.Desc != "" {
		out.Desc = b.Desc
	}
	if b.Run != "" {
		out.Run = b.Run
	}
	if b.Dir != "" {
		out.Dir = b.Dir
	}
	out.Env = mergeStringMaps(a.Env, b.Env)
	out.EnvFiles = append(append([]string{}, a.EnvFiles...), b.EnvFiles...)
	out.RequiresEnv = append(append([]string{}, a.RequiresEnv...), b.RequiresEnv...)
	out.Scripts = appendUnique(a.Scripts, b.Scripts...)
	if b.Danger != nil {
		out.Danger = b.Danger
	}
	out.Values = mergeStringMaps(a.Values, b.Values)
	return out
}

func appendUnique(base []string, additions ...string) []string {
	if len(base) == 0 && len(additions) == 0 {
		return nil
	}
	out := make([]string, 0, len(base)+len(additions))
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, base...), additions...) {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func mergeStringMaps(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range a {
		out[key] = value
	}
	for key, value := range b {
		out[key] = value
	}
	return out
}

func JoinPath(parts []string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, segment := range strings.Split(part, "/") {
			if segment != "" {
				clean = append(clean, segment)
			}
		}
	}
	return strings.Join(clean, "/")
}

func ChildTasks(tasks map[string]Task, prefix string) []Task {
	seen := map[string]Task{}
	prefix = strings.Trim(prefix, "/")
	for id, task := range tasks {
		if prefix != "" {
			if id == prefix || !strings.HasPrefix(id, prefix+"/") {
				continue
			}
		}
		rest := id
		if prefix != "" {
			rest = strings.TrimPrefix(id, prefix+"/")
		}
		child := strings.SplitN(rest, "/", 2)[0]
		if child == "" {
			continue
		}
		childID := child
		if prefix != "" {
			childID = prefix + "/" + child
		}
		candidate := task
		candidate.ID = childID
		if _, nested := strings.CutPrefix(rest, child+"/"); nested && candidate.Desc == task.Desc {
			candidate.Desc = ""
		}
		if existing, ok := seen[childID]; !ok || existing.Desc == "" {
			seen[childID] = candidate
		}
	}
	children := make([]Task, 0, len(seen))
	for _, child := range seen {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
	return children
}
