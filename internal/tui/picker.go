package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/self-deprecated/sd/internal/model"
)

type Choice struct {
	Title        string
	Description  string
	Args         []string
	Preview      string
	PreviewParts []PreviewPart
	Danger       bool
	Mode         string
	Terminal     bool
}

type PreviewPart struct {
	Text     string
	Key      string
	Variable bool
}

func Pick(ws *model.Workspace, prefix []string, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
	choices := choicesFor(ws, prefix)
	if len(choices) == 0 {
		return nil, false, fmt.Errorf("no tasks available")
	}
	m := newPickerModel(ws, prefix)
	return runPicker(m, stdout, stdin)
}

func PickChoices(title string, choices []Choice, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
	if len(choices) == 0 {
		return nil, false, fmt.Errorf("no choices available")
	}
	terminalChoices := make([]Choice, len(choices))
	copy(terminalChoices, choices)
	for i := range terminalChoices {
		terminalChoices[i].Terminal = true
	}
	m := pickerModel{title: title, choices: terminalChoices, list: list.New(choiceItems(terminalChoices), list.NewDefaultDelegate(), 0, 0)}
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(false)
	m.updateTitle()
	return runPicker(m, stdout, stdin)
}

func runPicker(m pickerModel, stdout io.Writer, stdin io.Reader) ([]string, bool, error) {
	program := tea.NewProgram(m, tea.WithInput(stdin), tea.WithOutput(stdout))
	final, err := program.Run()
	if err != nil {
		return nil, false, err
	}
	picked, ok := final.(pickerModel)
	if !ok || !picked.selected {
		return nil, false, nil
	}
	return picked.selectedArgs, true, nil
}

type item struct {
	choice Choice
}

func (i item) FilterValue() string { return i.choice.Title + " " + i.choice.Description }
func (i item) Title() string       { return i.choice.Title }
func (i item) Description() string { return i.choice.Description }

func choiceItems(choices []Choice) []list.Item {
	items := make([]list.Item, 0, len(choices))
	for _, choice := range choices {
		items = append(items, item{choice: choice})
	}
	return items
}

type pickerModel struct {
	ws           *model.Workspace
	title        string
	prefix       []string
	history      [][]string
	choices      []Choice
	filter       string
	list         list.Model
	selected     bool
	selectedArgs []string
	mode         string
	width        int
	height       int
}

func newPickerModel(ws *model.Workspace, prefix []string) pickerModel {
	mode := defaultModeFor(ws, prefix)
	choices := choicesForMode(ws, prefix, mode)
	m := pickerModel{ws: ws, prefix: append([]string{}, prefix...), choices: choices, mode: mode, list: list.New(choiceItems(choices), list.NewDefaultDelegate(), 0, 0)}
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(false)
	m.updateTitle()
	return m
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateTitle()
		m.resizeList()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyRunes:
			m.applyFilter(m.filter + string(msg.Runes))
			return m, nil
		case tea.KeySpace:
			m.applyFilter(m.filter + " ")
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.filter != "" {
				m.applyFilter("")
				return m, nil
			}
			return m, tea.Quit
		case "tab":
			if m.cycleMode() {
				return m, nil
			}
		case "backspace":
			if m.filter != "" {
				m.applyFilter(dropLastRune(m.filter))
				return m, nil
			}
			if m.goBack() {
				return m, nil
			}
		case "left":
			if m.goBack() {
				return m, nil
			}
		case "enter":
			if selected, ok := m.list.SelectedItem().(item); ok {
				if !selected.choice.Terminal {
					m.navigate(selected.choice.Args)
					return m, nil
				}
				m.selected = true
				m.selectedArgs = append([]string{}, selected.choice.Args...)
				if selected.choice.Mode != "" {
					m.selectedArgs = append(m.selectedArgs, "--mode", selected.choice.Mode)
				}
				return m, tea.Quit
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.resizeList()
	return m, cmd
}

func (m *pickerModel) resizeList() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	contentWidth := max(20, m.width-4)
	reservedHeight := 2 // vertical margin around the list
	if m.filter != "" {
		reservedHeight++
	}
	if m.modeLine() != "" {
		reservedHeight++
	}
	if previewHeight := m.previewHeight(); previewHeight > 0 {
		reservedHeight += 1 + previewHeight // separator + preview block
	}
	listHeight := max(5, m.height-reservedHeight)
	m.list.SetSize(contentWidth, listHeight)
}

func (m *pickerModel) applyFilter(filter string) {
	m.filter = filter
	m.list.SetItems(choiceItems(m.filteredChoices()))
	m.list.Select(0)
	m.resizeList()
}

func (m pickerModel) filteredChoices() []Choice {
	filter := strings.TrimSpace(strings.ToLower(m.filter))
	if filter == "" {
		return m.choices
	}
	type rankedChoice struct {
		choice Choice
		rank   int
		index  int
	}
	var ranked []rankedChoice
	for i, choice := range m.choices {
		if rank, ok := matchRank(choice, filter); ok {
			ranked = append(ranked, rankedChoice{choice: choice, rank: rank, index: i})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].rank != ranked[j].rank {
			return ranked[i].rank < ranked[j].rank
		}
		return ranked[i].index < ranked[j].index
	})
	out := make([]Choice, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.choice)
	}
	return out
}

func matchRank(choice Choice, filter string) (int, bool) {
	title := strings.ToLower(choice.Title)
	leaf := title
	if fields := strings.Fields(title); len(fields) > 0 {
		leaf = fields[len(fields)-1]
	}
	desc := strings.ToLower(choice.Description)
	switch {
	case leaf == filter || title == filter:
		return 0, true
	case strings.HasPrefix(leaf, filter):
		return 1, true
	case anyWordHasPrefix(title, filter):
		return 2, true
	case strings.Contains(title, filter):
		return 3, true
	case anyWordHasPrefix(desc, filter):
		return 4, true
	case strings.Contains(desc, filter):
		return 5, true
	default:
		return 0, false
	}
}

func anyWordHasPrefix(text, filter string) bool {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '/' || r == '-' || r == '_' || r == ':'
	}) {
		if strings.HasPrefix(word, filter) {
			return true
		}
	}
	return false
}

func dropLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

func (m *pickerModel) navigate(prefix []string) {
	m.history = append(m.history, append([]string{}, m.prefix...))
	m.setPrefix(prefix)
}

func (m *pickerModel) goBack() bool {
	if len(m.history) == 0 {
		return false
	}
	previous := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.setPrefix(previous)
	return true
}

func (m *pickerModel) cycleMode() bool {
	modes := modeCycleFor(m.ws, m.prefix)
	if len(modes) == 0 {
		return false
	}
	current := m.mode
	if current == "" {
		current = modes[0]
	}
	next := modes[0]
	for i, mode := range modes {
		if mode == current {
			next = modes[(i+1)%len(modes)]
			break
		}
	}
	m.mode = next
	m.choices = choicesForMode(m.ws, m.prefix, m.mode)
	m.list.SetItems(choiceItems(m.filteredChoices()))
	m.list.Select(0)
	m.updateTitle()
	m.resizeList()
	return true
}

func (m *pickerModel) setPrefix(prefix []string) {
	m.prefix = append([]string{}, prefix...)
	m.filter = ""
	m.mode = defaultModeFor(m.ws, m.prefix)
	m.choices = choicesForMode(m.ws, m.prefix, m.mode)
	m.list.SetItems(choiceItems(m.choices))
	m.list.Select(0)
	m.updateTitle()
	m.resizeList()
}

func (m *pickerModel) updateTitle() {
	title := m.title
	if title == "" {
		title = "SD tasks"
	}
	if len(m.prefix) > 0 {
		title += " › " + strings.Join(m.prefix, " › ")
	}
	if len(m.history) > 0 {
		title += "  (←/backspace back)"
	}
	m.list.Title = title
}

func (m pickerModel) modeLine() string {
	modes := modeCycleFor(m.ws, m.prefix)
	if len(modes) == 0 {
		return ""
	}
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	parts := make([]string, 0, len(modes))
	for _, mode := range modes {
		if mode == m.mode {
			parts = append(parts, "["+selectedStyle.Render(mode)+"]")
		} else {
			parts = append(parts, mode)
		}
	}
	return "Modes(Tab): " + strings.Join(parts, " | ")
}

func (m pickerModel) View() string {
	body := m.list.View()
	if modeLine := m.modeLine(); modeLine != "" {
		body = insertAfterFirstLine(body, modeLine)
	}
	if m.filter != "" {
		filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
		body += "\n" + filterStyle.Render("Filter: ") + m.filter
	}
	view := lipgloss.NewStyle().Margin(1, 2).Render(body)
	availablePreviewHeight := m.maxPreviewHeight()
	if m.height > 0 {
		availablePreviewHeight = min(availablePreviewHeight, m.height-lipgloss.Height(view)-1)
	}
	if preview := m.previewBlock(availablePreviewHeight); preview != "" {
		previewWidth := max(20, m.width-4)
		view += "\n" + lipgloss.NewStyle().Margin(0, 2).Width(previewWidth).Render(preview)
	}
	return view
}

func insertAfterFirstLine(body, line string) string {
	parts := strings.SplitN(body, "\n", 2)
	if len(parts) == 1 {
		return body + "\n" + line
	}
	return parts[0] + "\n" + line + "\n" + parts[1]
}

func (m pickerModel) previewBlock(maxHeight int) string {
	selected, ok := m.list.SelectedItem().(item)
	if !ok || selected.choice.Preview == "" {
		return ""
	}
	if maxHeight <= 0 {
		return ""
	}
	previewWidth := max(20, m.width-4)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	commandStyle := lipgloss.NewStyle().MaxWidth(previewWidth)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).MaxWidth(previewWidth)
	parts := selected.choice.PreviewParts
	if len(parts) == 0 {
		parts = []PreviewPart{{Text: selected.choice.Preview}}
	}
	lines := []string{}
	if selected.choice.Danger {
		lines = append(lines, warningStyle.Render("⚠ dangerous — confirmation required"))
	}
	lines = append(lines, labelStyle.Render("Preview"))
	commandLines := strings.Split(commandStyle.Render(renderPreviewParts(parts)), "\n")
	availableCommandLines := maxHeight - len(lines)
	if availableCommandLines <= 0 {
		return ""
	}
	if len(commandLines) > availableCommandLines {
		commandLines = commandLines[:availableCommandLines]
		commandLines[len(commandLines)-1] = "…"
	}
	lines = append(lines, commandLines...)
	return strings.Join(lines, "\n")
}

func (m pickerModel) maxPreviewHeight() int {
	if m.height <= 0 {
		return 4
	}
	reservedHeight := 2 // vertical margin around the list
	if m.filter != "" {
		reservedHeight++
	}
	if m.modeLine() != "" {
		reservedHeight++
	}
	return max(0, m.height-reservedHeight-1-5) // separator + minimum list height
}

func (m pickerModel) previewHeight() int {
	return lipgloss.Height(m.previewBlock(m.maxPreviewHeight()))
}

func buildPreviewParts(command string, values map[string]string, env map[string]string) (string, []PreviewPart) {
	var parts []PreviewPart
	envKeys := make([]string, 0, len(env))
	for key := range env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		parts = append(parts,
			PreviewPart{Text: key + "="},
			PreviewPart{Text: env[key], Key: "env:" + key, Variable: true},
			PreviewPart{Text: " "},
		)
	}
	for len(command) > 0 {
		start := strings.Index(command, "{{")
		if start < 0 {
			parts = append(parts, PreviewPart{Text: command})
			break
		}
		if start > 0 {
			parts = append(parts, PreviewPart{Text: command[:start]})
		}
		rest := command[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			parts = append(parts, PreviewPart{Text: command[start:]})
			break
		}
		key := strings.TrimSpace(rest[:end])
		value, ok := values[key]
		if !ok {
			parts = append(parts, PreviewPart{Text: "{{" + rest[:end] + "}}"})
		} else {
			parts = append(parts, PreviewPart{Text: value, Key: key, Variable: true})
		}
		command = rest[end+2:]
	}
	var rendered strings.Builder
	for _, part := range parts {
		rendered.WriteString(part.Text)
	}
	return rendered.String(), parts
}

func renderPreviewParts(parts []PreviewPart) string {
	baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	if len(parts) == 0 {
		return ""
	}
	keys := make([]string, 0)
	seen := map[string]bool{}
	for _, part := range parts {
		if part.Variable && !seen[part.Key] {
			seen[part.Key] = true
			keys = append(keys, part.Key)
		}
	}
	sort.Strings(keys)
	palette := []lipgloss.Color{"13", "14", "11", "12", "9", "10"}
	styles := map[string]lipgloss.Style{}
	for i, key := range keys {
		styles[key] = lipgloss.NewStyle().Foreground(palette[i%len(palette)]).Bold(true)
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Variable {
			out.WriteString(styles[part.Key].Render(part.Text))
		} else {
			out.WriteString(baseStyle.Render(part.Text))
		}
	}
	return out.String()
}

func choicesFor(ws *model.Workspace, prefix []string) []Choice {
	return choicesForMode(ws, prefix, defaultModeFor(ws, prefix))
}

func choicesForMode(ws *model.Workspace, prefix []string, mode string) []Choice {
	if len(prefix) == 0 {
		var choices []Choice
		for _, p := range ws.SortedProjects() {
			choices = append(choices, Choice{Title: p.ID, Description: p.Name, Args: []string{p.ID}})
		}
		for _, child := range model.ChildTasks(ws.Aggregates, "") {
			args := strings.Split(child.ID, "/")
			choice := choiceForTask(strings.Join(args, " "), child, args, mode)
			if _, ok := ws.Aggregates[child.ID]; ok {
				choice.Terminal = true
			}
			choices = append(choices, choice)
		}
		return choices
	}
	if p, ok := ws.ProjectByID(prefix[0]); ok {
		path := model.JoinPath(prefix[1:])
		if task, ok := p.Task(path); ok && len(task.Choices) > 0 {
			choices := make([]Choice, 0, len(task.Choices))
			for _, id := range task.SortedChoiceIDs() {
				choiceTask, _ := task.ChoiceTask(id)
				resolvedChoice := choiceForTask(id, choiceTask, append(append([]string{p.ID}, strings.Split(task.ID, "/")...), id), mode)
				resolvedChoice.Terminal = true
				choices = append(choices, resolvedChoice)
			}
			return choices
		}
		children := model.ChildTasks(p.Tasks, path)
		choices := make([]Choice, 0, len(children))
		for _, child := range children {
			parts := strings.Split(child.ID, "/")
			args := append([]string{p.ID}, parts...)
			terminal := false
			if task, ok := p.Task(child.ID); ok && len(task.Choices) == 0 {
				terminal = true
			}
			choice := choiceForTask(strings.Join(args, " "), child, args, mode)
			choice.Terminal = terminal
			choices = append(choices, choice)
		}
		return choices
	}
	path := model.JoinPath(prefix)
	children := model.ChildTasks(ws.Aggregates, path)
	choices := make([]Choice, 0, len(children))
	for _, child := range children {
		args := strings.Split(child.ID, "/")
		terminal := false
		if _, ok := ws.Aggregates[child.ID]; ok {
			terminal = true
		}
		choice := choiceForTask(strings.Join(args, " "), child, args, mode)
		choice.Terminal = terminal
		choices = append(choices, choice)
	}
	return choices
}

func choiceForTask(title string, task model.Task, args []string, mode string) Choice {
	resolved, ok := task.ApplyMode(mode)
	if ok {
		task = resolved
	}
	values, _ := model.ResolveOptionValues(task.Options, task.Values)
	task = model.RenderTask(task, values)
	preview, previewParts := buildPreviewParts(task.Run, nil, task.Env)
	return Choice{Title: title, Description: task.Desc, Args: args, Preview: preview, PreviewParts: previewParts, Danger: task.Danger, Mode: task.Mode}
}

func defaultModeFor(ws *model.Workspace, prefix []string) string {
	modes := modeCycleFor(ws, prefix)
	if len(modes) == 0 {
		return ""
	}
	return modes[0]
}

func modeCycleFor(ws *model.Workspace, prefix []string) []string {
	if len(prefix) == 0 {
		return nil
	}
	if p, ok := ws.ProjectByID(prefix[0]); ok {
		path := model.JoinPath(prefix[1:])
		if task, ok := p.Task(path); ok {
			return task.ModeCycleIDs()
		}
		children := model.ChildTasks(p.Tasks, path)
		return commonModeCycle(children)
	}
	path := model.JoinPath(prefix)
	children := model.ChildTasks(ws.Aggregates, path)
	return commonModeCycle(children)
}

func commonModeCycle(tasks []model.Task) []string {
	if len(tasks) == 0 {
		return nil
	}
	return tasks[0].ModeCycleIDs()
}
