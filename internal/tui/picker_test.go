package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/self-deprecated/sd/internal/model"
)

func TestChoicesForRootAggregateShowsTopLevelGroup(t *testing.T) {
	ws := &model.Workspace{Aggregates: map[string]model.Task{
		"release/npm/all": {ID: "release/npm/all", Desc: "Publish everything", Run: "echo all", Danger: true},
	}}
	choices := choicesFor(ws, nil)
	if len(choices) != 1 {
		t.Fatalf("len(choices) = %d, want 1", len(choices))
	}
	if choices[0].Title != "release" || choices[0].Terminal {
		t.Fatalf("unexpected aggregate group choice: %#v", choices[0])
	}
}

func TestChoicesForProjectPrefix(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{{ID: "app", Name: "App", Tasks: map[string]model.Task{
		"mobile/build/ios-dev": {ID: "mobile/build/ios-dev", Desc: "iOS dev build"},
	}}}, Aliases: map[string]string{"app": "app"}}
	choices := choicesFor(ws, []string{"app", "mobile", "build"})
	if len(choices) != 1 {
		t.Fatalf("len(choices) = %d, want 1", len(choices))
	}
	if choices[0].Title != "app mobile build ios-dev" || choices[0].Description != "iOS dev build" {
		t.Fatalf("unexpected choice: %#v", choices[0])
	}
}

func TestChoicesForAggregatePrefix(t *testing.T) {
	ws := &model.Workspace{Aggregates: map[string]model.Task{
		"release/npm/all":    {ID: "release/npm/all", Desc: "Publish everything", Run: "echo all", Danger: true},
		"release/npm/shared": {ID: "release/npm/shared", Desc: "Publish shared", Run: "echo shared", Danger: true},
	}}
	choices := choicesFor(ws, []string{"release", "npm"})
	if len(choices) != 2 {
		t.Fatalf("len(choices) = %d, want 2", len(choices))
	}
	if choices[0].Title != "release npm all" || choices[0].Description != "Publish everything" || !choices[0].Terminal || !choices[0].Danger {
		t.Fatalf("unexpected first choice: %#v", choices[0])
	}
	if choices[1].Title != "release npm shared" || choices[1].Description != "Publish shared" || !choices[1].Terminal || choices[1].Preview != "echo shared" {
		t.Fatalf("unexpected second choice: %#v", choices[1])
	}
}

func TestChoicesForChoiceTaskIncludeRenderedPreview(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{{ID: "app", Name: "App", Path: "/tmp/app", Tasks: map[string]model.Task{
		"mobile/build": {
			ID:  "mobile/build",
			Run: "echo {{platform}} {{profile}}",
			Choices: map[string]model.TaskChoice{
				"ios-dev": {Label: "iOS development", Values: map[string]string{"platform": "ios", "profile": "development"}},
			},
		},
	}}}, Aliases: map[string]string{"app": "app"}}
	choices := choicesFor(ws, []string{"app", "mobile/build"})
	if len(choices) != 1 {
		t.Fatalf("len(choices) = %d, want 1", len(choices))
	}
	if choices[0].Title != "ios-dev" || choices[0].Description != "iOS development" {
		t.Fatalf("unexpected choice label: %#v", choices[0])
	}
	if choices[0].Preview != "echo ios development" {
		t.Fatalf("preview = %q", choices[0].Preview)
	}
	if got := variableTexts(choices[0].PreviewParts); len(got) != 0 {
		t.Fatalf("variable preview parts = %#v", got)
	}
}

func TestPickerViewListsModesAboveOptions(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{{ID: "app", Name: "App", Path: "/tmp/app", Tasks: map[string]model.Task{
		"release/npm": {
			ID:          "release/npm",
			Desc:        "Release npm packages",
			DefaultMode: "dry-run",
			Modes: map[string]model.TaskMode{
				"dry-run": {},
				"publish": {},
			},
		},
	}}}, Aliases: map[string]string{"app": "app"}}
	m := newPickerModel(ws, []string{"app", "release"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	view := updated.(pickerModel).render()
	for _, want := range []string{"app › release", "Modes(Tab):", "dry-run", "publish", "release npm"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if strings.Contains(updated.(pickerModel).list.Title, "Modes(Tab):") {
		t.Fatalf("mode line should not be hidden in the title: %q", updated.(pickerModel).list.Title)
	}
}

func TestPickerViewStaysWithinWindowHeightWhenPreviewIsVisible(t *testing.T) {
	choices := []Choice{{Title: "ios-production", Description: "Production build", Preview: "eas build --platform ios --profile production --auto-submit", Danger: true, Args: []string{"app", "mobile/build", "ios-production"}}}
	items := []list.Item{item{choice: choices[0]}}
	m := pickerModel{choices: choices, list: list.New(items, list.NewDefaultDelegate(), 0, 0)}
	m.list.Title = "SD tasks"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	view := updated.(pickerModel).render()
	if got := lipgloss.Height(view); got > 20 {
		t.Fatalf("view height = %d, want <= 20\n%s", got, view)
	}
}

func TestPickerViewStaysWithinWindowHeightWhenPreviewIsMultiline(t *testing.T) {
	preview := strings.Join([]string{
		"set -euo pipefail",
		"for platform in ios android; do",
		"  out=$(mktemp -d)",
		"  cleanup() { rm -rf \"$out\"; }",
		"  trap cleanup EXIT",
		"  CI=1 corepack pnpm exec expo export --platform \"$platform\" --output-dir \"$out\"",
		"  cleanup",
		"  trap - EXIT",
		"done",
	}, "\n")
	choices := []Choice{{Title: "bundle-check", Description: "Bundle app", Preview: preview, Args: []string{"app", "mobile/bundle-check"}}}
	items := []list.Item{item{choice: choices[0]}}
	m := pickerModel{choices: choices, list: list.New(items, list.NewDefaultDelegate(), 0, 0)}
	m.list.Title = "SD tasks"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	view := updated.(pickerModel).render()
	if got := lipgloss.Height(view); got > 18 {
		t.Fatalf("view height = %d, want <= 18\n%s", got, view)
	}
	if !strings.Contains(view, "…") {
		t.Fatalf("expected truncated multiline preview, got:\n%s", view)
	}
}

func TestPreviewHighlightsOnlyTemplateSubstitutions(t *testing.T) {
	preview, parts := buildPreviewParts("eas-cli build --mode {{mode}} --platform {{platform}}", map[string]string{"mode": "eas", "platform": "ios"}, nil)
	if preview != "eas-cli build --mode eas --platform ios" {
		t.Fatalf("preview = %q", preview)
	}
	if got := variableTexts(parts); strings.Join(got, ",") != "eas,ios" {
		t.Fatalf("variable preview parts = %#v", got)
	}
	if parts[0].Variable || !strings.Contains(parts[0].Text, "eas-cli") {
		t.Fatalf("expected eas-cli to stay base text, got %#v", parts[0])
	}
}

func TestPreviewIncludesEnvAssignmentsSoSimilarCommandsDiffer(t *testing.T) {
	localPreview, localParts := buildPreviewParts("bash scripts/mobile-build-android-local.sh", nil, nil)
	apkPreview, apkParts := buildPreviewParts("bash scripts/mobile-build-android-local.sh", nil, map[string]string{"EAS_BUILD_PROFILE": "preview"})
	if localPreview == apkPreview {
		t.Fatalf("expected env to distinguish previews, got %q", localPreview)
	}
	if apkPreview != "EAS_BUILD_PROFILE=preview bash scripts/mobile-build-android-local.sh" {
		t.Fatalf("apk preview = %q", apkPreview)
	}
	if got := variableTexts(localParts); len(got) != 0 {
		t.Fatalf("local variable parts = %#v", got)
	}
	if got := variableTexts(apkParts); strings.Join(got, ",") != "preview" {
		t.Fatalf("apk variable parts = %#v", got)
	}
}

func variableTexts(parts []PreviewPart) []string {
	var out []string
	for _, part := range parts {
		if part.Variable {
			out = append(out, part.Text)
		}
	}
	return out
}

func sendKeys(m tea.Model, text string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range text {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m, cmd
}

func selectedTitle(m pickerModel) string {
	if selected, ok := m.list.SelectedItem().(item); ok {
		return selected.choice.Title
	}
	return ""
}

func TestPickerFilterPrefersTaskTitleBeforeDescriptionAndShowsTypedText(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{{ID: "landing", Name: "Landing", Path: "/tmp/landing", Tasks: map[string]model.Task{
		"install": {ID: "install", Desc: "Install dependencies", Run: "npm ci"},
		"deploy":  {ID: "deploy", Desc: "Build and deploy via FTP", Run: "npm run deploy"},
		"preview": {ID: "preview", Desc: "Preview built site", Run: "npm run preview"},
	}}}, Aliases: map[string]string{"landing": "landing"}}
	m := newPickerModel(ws, []string{"landing"})
	updated, _ := sendKeys(m, "dep")
	filtered := updated.(pickerModel)
	if got := selectedTitle(filtered); got != "landing deploy" {
		t.Fatalf("selected item after typing dep = %q, want landing deploy", got)
	}
	if view := ansi.Strip(filtered.render()); !strings.Contains(view, "Filter: dep") {
		t.Fatalf("view does not show typed filter: %q", view)
	}
}

func TestPickerTypingNarrowsEnterNavigatesThenRunsLeaf(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{
		{ID: "demo", Name: "Demo", Path: "/tmp/demo", Tasks: map[string]model.Task{
			"check": {ID: "check", Desc: "Check Demo", Run: "echo check"},
		}},
		{ID: "landing", Name: "Landing", Path: "/tmp/landing", Tasks: map[string]model.Task{
			"build":   {ID: "build", Desc: "Build site", Run: "npm run build"},
			"preview": {ID: "preview", Desc: "Preview built site", Run: "npm run preview"},
		}},
	}, Aliases: map[string]string{"demo": "demo", "landing": "landing"}}
	m := newPickerModel(ws, nil)
	updated, _ := sendKeys(m, "land")
	filteredRoot := updated.(pickerModel)
	if got := selectedTitle(filteredRoot); got != "landing" {
		t.Fatalf("selected root item after typing land = %q", got)
	}
	updated, _ = filteredRoot.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	landingStage := updated.(pickerModel)
	if got := strings.Join(landingStage.prefix, "/"); got != "landing" {
		t.Fatalf("prefix after enter = %q", got)
	}
	if landingStage.filter != "" {
		t.Fatalf("filter after navigation = %q, want empty", landingStage.filter)
	}

	updated, _ = sendKeys(landingStage, "prev")
	filteredLanding := updated.(pickerModel)
	if got := selectedTitle(filteredLanding); got != "landing preview" {
		t.Fatalf("selected landing item after typing prev = %q", got)
	}
	updated, _ = filteredLanding.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	picked := updated.(pickerModel)
	if !picked.selected {
		t.Fatalf("expected final selection")
	}
	if got := strings.Join(picked.selectedArgs, " "); got != "landing preview" {
		t.Fatalf("selected args = %q", got)
	}
}

func TestPickerCanNavigateBackToPreviousScreen(t *testing.T) {
	ws := &model.Workspace{Projects: []*model.Project{{ID: "app", Name: "App", Path: "/tmp/app", Tasks: map[string]model.Task{
		"mobile/build": {
			ID:  "mobile/build",
			Run: "echo {{platform}}",
			Choices: map[string]model.TaskChoice{
				"ios-dev": {Label: "iOS development", Values: map[string]string{"platform": "ios"}},
			},
		},
	}}}, Aliases: map[string]string{"app": "app"}}
	m := newPickerModel(ws, []string{"app", "mobile"})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	navigated := updated.(pickerModel)
	if got := strings.Join(navigated.prefix, "/"); got != "app/mobile/build" {
		t.Fatalf("prefix after enter = %q", got)
	}
	updated, _ = navigated.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	back := updated.(pickerModel)
	if got := strings.Join(back.prefix, "/"); got != "app/mobile" {
		t.Fatalf("prefix after back = %q", got)
	}
}
