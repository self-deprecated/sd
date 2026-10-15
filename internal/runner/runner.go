package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/self-deprecated/sd/internal/model"
	"github.com/self-deprecated/sd/internal/shell"
)

type Options struct {
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	Yes         bool
	AllowPrompt bool
	Mode        string
	Vars        map[string]string

	// inheritedMode is the effective mode of the task that depends on this
	// one; stack holds the dependency chain being run.
	inheritedMode string
	stack         []string
}

// enter adds label to the dependency chain, rejecting cycles.
func enter(label string, opts Options) (Options, error) {
	for i, running := range opts.stack {
		if running == label {
			cycle := append(append([]string{}, opts.stack[i:]...), label)
			return opts, UsageError{Message: "dependency cycle: " + strings.Join(cycle, " → ")}
		}
	}
	opts.stack = append(append([]string{}, opts.stack...), label)
	return opts, nil
}

// dependencyOptions returns the options for running task's dependencies:
// they inherit its effective mode but not its runtime variables.
func dependencyOptions(task model.Task, opts Options) Options {
	deps := opts
	deps.Vars = nil
	deps.Mode = ""
	if task.Mode != "" {
		deps.inheritedMode = task.Mode
	}
	return deps
}

func RunProjectTask(ws *model.Workspace, project *model.Project, task model.Task, args []string, opts Options) error {
	label := fmt.Sprintf("%s:%s", project.ID, task.ID)
	if task.Choice != "" {
		label += " " + task.Choice
	}
	opts, err := enter(label, opts)
	if err != nil {
		return err
	}
	task, err = resolveMode(label, task, opts)
	if err != nil {
		return err
	}
	task, err = resolveRuntimeInputs(label, task, opts)
	if err != nil {
		return err
	}
	if shouldSkip, err := skipProjectUnchanged(ws.Root, project.Path, label, task.WhenChanged, opts); shouldSkip || err != nil {
		return err
	}
	if err := confirmDanger(label, task.Danger, opts); err != nil {
		return err
	}
	workDir := project.TaskWorkingDir(task)
	commandEnv, err := commandEnvironment(workDir, task.EnvFiles, task.Env)
	if err != nil {
		return err
	}
	missing := missingEnv(commandEnv, task.RequiresEnv)
	if len(missing) > 0 {
		return UsageError{Message: fmt.Sprintf("task %s:%s missing required environment: %s", project.ID, task.ID, strings.Join(missing, ", "))}
	}
	for _, dep := range task.Depends {
		if err := RunReference(ws, dep, nil, dependencyOptions(task, opts)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(task.Run) == "" {
		if len(task.Depends) > 0 {
			return nil
		}
		return UsageError{Message: fmt.Sprintf("task %s:%s has no run command", project.ID, task.ID)}
	}
	command, err := composeSharedScripts(project, task)
	if err != nil {
		return err
	}
	return runShell(workDir, command, args, commandEnv, opts)
}

func RunAggregate(ws *model.Workspace, task model.Task, opts Options) error {
	opts, err := enter(task.ID, opts)
	if err != nil {
		return err
	}
	task, err = resolveMode(task.ID, task, opts)
	if err != nil {
		return err
	}
	task, err = resolveRuntimeInputs(task.ID, task, opts)
	if err != nil {
		return err
	}
	if shouldSkip, err := skipUnchanged(ws.Root, task.ID, task.WhenChanged, opts); shouldSkip || err != nil {
		return err
	}
	if err := confirmDanger(task.ID, task.Danger, opts); err != nil {
		return err
	}
	commandEnv, err := commandEnvironment(ws.Root, task.EnvFiles, task.Env)
	if err != nil {
		return err
	}
	missing := missingEnv(commandEnv, task.RequiresEnv)
	if len(missing) > 0 {
		return UsageError{Message: fmt.Sprintf("task %s missing required environment: %s", task.ID, strings.Join(missing, ", "))}
	}
	for _, dep := range task.Depends {
		if err := RunReference(ws, dep, nil, dependencyOptions(task, opts)); err != nil {
			return err
		}
	}
	if task.Run != "" {
		return runShell(ws.Root, task.Run, nil, commandEnv, opts)
	}
	return nil
}

func RunReference(ws *model.Workspace, ref string, args []string, opts Options) error {
	if strings.Contains(ref, ":") {
		parts := strings.SplitN(ref, ":", 2)
		project, ok := ws.ProjectByID(parts[0])
		if !ok {
			return UsageError{Message: fmt.Sprintf("unknown project in dependency %q", ref)}
		}
		task, ok := resolveProjectReference(project, parts[1])
		if !ok {
			return UsageError{Message: fmt.Sprintf("unknown task in dependency %q", ref)}
		}
		return RunProjectTask(ws, project, task, args, opts)
	}
	if task, ok := ws.Aggregates[ref]; ok {
		return RunAggregate(ws, task, opts)
	}
	return UsageError{Message: fmt.Sprintf("unknown dependency %q", ref)}
}

func resolveProjectReference(project *model.Project, ref string) (model.Task, bool) {
	fields := strings.Fields(ref)
	if len(fields) == 0 {
		return model.Task{}, false
	}
	for i := len(fields); i >= 1; i-- {
		id := model.JoinPath(fields[:i])
		task, ok := project.Task(id)
		if !ok {
			continue
		}
		extra := fields[i:]
		if len(extra) == 0 {
			return task, true
		}
		if len(task.Choices) == 0 || len(extra) > 1 {
			return model.Task{}, false
		}
		return task.ChoiceTask(extra[0])
	}
	return model.Task{}, false
}

// resolveMode applies the requested mode, or for a dependency with modes the
// mode inherited from the task that depends on it.
func resolveMode(label string, task model.Task, opts Options) (model.Task, error) {
	mode := opts.Mode
	if mode == "" && len(task.Modes) > 0 {
		mode = opts.inheritedMode
	}
	resolved, ok := task.ApplyMode(mode)
	if ok {
		return resolved, nil
	}
	return model.Task{}, UsageError{Message: fmt.Sprintf("task %s has no mode %q", label, mode)}
}

func ResolveRuntimeInputs(label string, task model.Task, opts Options) (model.Task, error) {
	return resolveRuntimeInputs(label, task, opts)
}

func resolveRuntimeInputs(label string, task model.Task, opts Options) (model.Task, error) {
	if err := validateRuntimeOverrides(label, task, opts.Vars); err != nil {
		return model.Task{}, err
	}
	values, err := model.ResolveOptionValues(task.Options, task.Values, opts.Vars)
	if err != nil {
		return model.Task{}, UsageError{Message: fmt.Sprintf("task %s: %s", label, err)}
	}
	if err := validateRuntimeValues(label, task, values); err != nil {
		return model.Task{}, err
	}
	missing := missingRuntimeInputs(task, values)
	if len(missing) > 0 {
		if !opts.AllowPrompt {
			return model.Task{}, UsageError{Message: missingRuntimeMessage(label, missing)}
		}
		prompted, err := promptRuntimeInputs(label, task, values, missing, opts)
		if err != nil {
			return model.Task{}, err
		}
		values = prompted
		if err := validateRuntimeValues(label, task, values); err != nil {
			return model.Task{}, err
		}
	}
	return model.RenderTask(task, values), nil
}

func validateRuntimeOverrides(label string, task model.Task, overrides map[string]string) error {
	for name, value := range overrides {
		option, ok := task.Options[name]
		if !ok {
			return UsageError{Message: fmt.Sprintf("task %s has no runtime variable %q", label, name)}
		}
		if err := validateRuntimeValue(name, value, option); err != nil {
			return UsageError{Message: fmt.Sprintf("task %s %s", label, err.Error())}
		}
	}
	return nil
}

func validateRuntimeValues(label string, task model.Task, values map[string]string) error {
	for name, option := range task.Options {
		value := values[name]
		if value == "" {
			continue
		}
		if err := validateRuntimeValue(name, value, option); err != nil {
			return UsageError{Message: fmt.Sprintf("task %s %s", label, err.Error())}
		}
	}
	return nil
}

func missingRuntimeInputs(task model.Task, values map[string]string) []string {
	refs := model.TemplateVariables(task)
	var missing []string
	for name, option := range task.Options {
		if values[name] != "" {
			continue
		}
		required := refs[name]
		if option.Required != nil {
			required = *option.Required
		}
		if required {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func missingRuntimeMessage(label string, missing []string) string {
	parts := make([]string, 0, len(missing))
	for _, name := range missing {
		parts = append(parts, fmt.Sprintf("%s (set with --var %s=value)", name, name))
	}
	return fmt.Sprintf("task %s missing required runtime variable: %s", label, strings.Join(parts, ", "))
}

func promptRuntimeInputs(label string, task model.Task, values map[string]string, missing []string, opts Options) (map[string]string, error) {
	reader := bufio.NewReader(opts.Stdin)
	prompted := map[string]string{}
	for key, value := range values {
		prompted[key] = value
	}
	for _, name := range missing {
		option := task.Options[name]
		for {
			fmt.Fprint(opts.Stderr, runtimePrompt(name, option))
			answer, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			answer = strings.TrimSpace(answer)
			if answer == "" && option.Default != "" {
				answer = option.Default
			}
			if answer == "" {
				fmt.Fprintf(opts.Stderr, "value required for %s\n", name)
				if errors.Is(err, io.EOF) {
					return nil, UsageError{Message: fmt.Sprintf("task %s missing required runtime variable: %s", label, name)}
				}
				continue
			}
			if err := validateRuntimeValue(name, answer, option); err != nil {
				fmt.Fprintln(opts.Stderr, err)
				if errors.Is(err, io.EOF) {
					return nil, UsageError{Message: fmt.Sprintf("task %s %s", label, err.Error())}
				}
				continue
			}
			prompted[name] = answer
			if prompted, err = model.ResolveOptionValues(task.Options, prompted); err != nil {
				return nil, UsageError{Message: fmt.Sprintf("task %s: %s", label, err)}
			}
			break
		}
	}
	return prompted, nil
}

func runtimePrompt(name string, option model.TaskOption) string {
	label := option.Prompt
	if label == "" {
		label = name
	}
	if option.Desc != "" {
		label += " — " + option.Desc
	}
	if len(option.Values) > 0 {
		label += " [" + strings.Join(sortedOptionValues(option), "/") + "]"
	}
	if option.Default != "" {
		label += " (default " + option.Default + ")"
	}
	return label + ": "
}

func sortedOptionValues(option model.TaskOption) []string {
	ids := make([]string, 0, len(option.Values))
	for id := range option.Values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func validateRuntimeValue(name, value string, option model.TaskOption) error {
	switch runtimeOptionType(option) {
	case "enum":
		if _, ok := option.Values[value]; !ok {
			return fmt.Errorf("variable %s must be one of: %s", name, strings.Join(sortedOptionValues(option), ", "))
		}
	case "string", "text":
		return nil
	case "integer", "number":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("variable %s must be an integer", name)
		}
	case "":
		return nil
	default:
		return fmt.Errorf("variable %s has unsupported type %q", name, option.Type)
	}
	return nil
}

func runtimeOptionType(option model.TaskOption) string {
	typ := strings.ToLower(strings.TrimSpace(option.Type))
	if typ == "" && len(option.Values) > 0 {
		return "enum"
	}
	return typ
}

type UsageError struct{ Message string }

func (e UsageError) Error() string { return e.Message }

func IsUsage(err error) bool {
	var usage UsageError
	return errors.As(err, &usage)
}

func composeSharedScripts(project *model.Project, task model.Task) (string, error) {
	if len(task.Scripts) == 0 {
		return task.Run, nil
	}
	var command strings.Builder
	for index, name := range task.Scripts {
		script, ok := project.Scripts[name]
		if !ok {
			return "", UsageError{Message: fmt.Sprintf("task %s:%s references unknown shared script %q", project.ID, task.ID, name)}
		}
		functionName := fmt.Sprintf("__sd_shared_script_%d", index)
		fmt.Fprintf(&command, "%s() {\n%s\n}\n", functionName, script.Run)
		fmt.Fprintf(&command, "%s \"$@\"\n", functionName)
		command.WriteString("__sd_shared_status=$?\n")
		fmt.Fprintf(&command, "unset -f %s\n", functionName)
		command.WriteString("if (( __sd_shared_status != 0 )); then exit \"$__sd_shared_status\"; fi\n")
	}
	command.WriteString(task.Run)
	return command.String(), nil
}

func runShell(dir, script string, args []string, env map[string]string, opts Options) error {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	return shell.Run(interrupts, dir, script, args, envList(env), opts.Stdin, opts.Stdout, opts.Stderr)
}

func commandEnvironment(dir string, envFiles []string, taskEnv map[string]string) (map[string]string, error) {
	env := map[string]string{}
	for _, path := range envFiles {
		fileEnv, err := readEnvFile(dir, path)
		if err != nil {
			return nil, err
		}
		for key, value := range fileEnv {
			env[key] = value
		}
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if value != "" || env[key] == "" {
			env[key] = value
		}
	}
	for key, value := range taskEnv {
		env[key] = value
	}
	return env, nil
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func readEnvFile(dir, name string) (map[string]string, error) {
	if name == "" {
		return nil, nil
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read env file %s: %w", name, err)
	}
	defer file.Close()
	values, err := parseEnv(file)
	if err != nil {
		return nil, fmt.Errorf("parse env file %s: %w", name, err)
	}
	return values, nil
}

func parseEnv(r io.Reader) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=value", lineNo)
		}
		key = strings.TrimSpace(key)
		if !validEnvName(key) {
			return nil, fmt.Errorf("line %d: invalid environment variable name %q", lineNo, key)
		}
		value, err := parseEnvValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func parseEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "\"") {
		end := closingDoubleQuote(raw)
		if end < 0 {
			return "", fmt.Errorf("unterminated double-quoted value")
		}
		if err := validateQuotedEnvRemainder(raw[end+1:]); err != nil {
			return "", err
		}
		value, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return "", err
		}
		return value, nil
	}
	if strings.HasPrefix(raw, "'") {
		end := strings.Index(raw[1:], "'")
		if end < 0 {
			return "", fmt.Errorf("unterminated single-quoted value")
		}
		end++
		if err := validateQuotedEnvRemainder(raw[end+1:]); err != nil {
			return "", err
		}
		return raw[1:end], nil
	}
	return stripEnvComment(raw), nil
}

func closingDoubleQuote(raw string) int {
	escaped := false
	for i := 1; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch raw[i] {
		case '\\':
			escaped = true
		case '"':
			return i
		}
	}
	return -1
}

func validateQuotedEnvRemainder(raw string) error {
	remainder := strings.TrimSpace(raw)
	if remainder == "" || strings.HasPrefix(remainder, "#") {
		return nil
	}
	return fmt.Errorf("unexpected content after quoted value")
}

func stripEnvComment(raw string) string {
	for i, r := range raw {
		if r == '#' && (i == 0 || raw[i-1] == ' ' || raw[i-1] == '\t') {
			return strings.TrimSpace(raw[:i])
		}
	}
	return strings.TrimSpace(raw)
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func confirmDanger(label string, danger bool, opts Options) error {
	if !danger || opts.Yes {
		return nil
	}
	if !opts.AllowPrompt {
		return UsageError{Message: fmt.Sprintf("task %s is marked dangerous and requires --yes", label)}
	}
	fmt.Fprintf(opts.Stderr, "Task %s is marked dangerous. Continue? [y/N] ", label)
	answer, err := bufio.NewReader(opts.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return UsageError{Message: fmt.Sprintf("cancelled dangerous task %s", label)}
	}
	return nil
}

func missingEnv(env map[string]string, required []string) []string {
	var missing []string
	for _, name := range required {
		if env[name] == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

var changedFilesCache sync.Map

func skipUnchanged(root, label string, patterns []string, opts Options) (bool, error) {
	return skipChangedFiles(root, root, label, patterns, opts)
}

func skipProjectUnchanged(root, projectRoot, label string, patterns []string, opts Options) (bool, error) {
	return skipChangedFiles(root, projectRoot, label, patterns, opts)
}

func skipChangedFiles(root, patternRoot, label string, patterns []string, opts Options) (bool, error) {
	if len(patterns) == 0 {
		return false, nil
	}
	changed, err := changedFiles(root)
	if err != nil {
		return false, err
	}
	for _, file := range changed {
		for _, pattern := range patterns {
			if matchesChangedPattern(root, patternRoot, pattern, file) {
				return false, nil
			}
		}
	}
	fmt.Fprintf(opts.Stdout, "sd: skipping %s; no changed files match %s\n", label, strings.Join(patterns, ", "))
	return true, nil
}

func matchesChangedPattern(root, patternRoot, pattern, file string) bool {
	if strings.HasPrefix(pattern, "/") {
		return matchChangedPattern(strings.TrimPrefix(pattern, "/"), file)
	}
	projectFile, ok := relativeChangedFile(root, patternRoot, file)
	return ok && matchChangedPattern(pattern, projectFile)
}

func relativeChangedFile(root, patternRoot, file string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPatternRoot, err := filepath.Abs(patternRoot)
	if err != nil {
		return "", false
	}
	absFile := filepath.Join(absRoot, filepath.FromSlash(normalizePath(file)))
	rel, err := filepath.Rel(absPatternRoot, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return normalizePath(rel), true
}

func changedFiles(root string) ([]string, error) {
	if value, ok := changedFilesCache.Load(root); ok {
		return value.([]string), nil
	}
	files, err := detectChangedFiles(root)
	if err != nil {
		return nil, err
	}
	changedFilesCache.Store(root, files)
	return files, nil
}

func detectChangedFiles(root string) ([]string, error) {
	if env, ok := os.LookupEnv("SD_CHANGED_FILES"); ok {
		return parseChangedFiles(env), nil
	}
	if base := os.Getenv("SD_CHANGE_BASE"); base != "" {
		if files, ok := runChangedFilesCommand(root, "jj", "diff", "--from", base, "--name-only"); ok {
			return files, nil
		}
		if files, ok := runChangedFilesCommand(root, "git", "diff", "--name-only", base+"...HEAD"); ok {
			return files, nil
		}
		return nil, UsageError{Message: fmt.Sprintf("could not resolve SD_CHANGE_BASE %q with jj or git", base)}
	}
	if files, ok := runChangedFilesCommand(root, "jj", "diff", "--name-only"); ok {
		return files, nil
	}
	if files, ok := runChangedFilesCommand(root, "git", "diff", "--name-only", "HEAD"); ok {
		return files, nil
	}
	return nil, UsageError{Message: "could not determine changed files for whenChanged task; set SD_CHANGED_FILES or SD_CHANGE_BASE"}
}

func runChangedFilesCommand(root, name string, args ...string) ([]string, bool) {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	return parseChangedFiles(string(out)), true
}

func parseChangedFiles(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	files := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := normalizePath(field)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

func matchChangedPattern(pattern, file string) bool {
	pattern = normalizePath(pattern)
	file = normalizePath(file)
	if pattern == file {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "**")
		if !strings.ContainsAny(prefix, "*?") {
			return strings.HasPrefix(file, prefix)
		}
	}
	if ok, _ := path.Match(pattern, file); ok {
		return true
	}
	regex := globToRegexp(pattern)
	return regex.MatchString(file)
}

func normalizePath(value string) string {
	return strings.Trim(strings.ReplaceAll(value, "\\", "/"), " /")
}

func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if strings.HasPrefix(pattern[i:], "**/") {
				// `**/` also matches no directories at all.
				b.WriteString("(?:.*/)?")
				i += 2
			} else if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
