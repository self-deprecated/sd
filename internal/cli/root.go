package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/model"
	"github.com/self-deprecated/sd/internal/runner"
	"github.com/self-deprecated/sd/internal/shell"
	"github.com/self-deprecated/sd/internal/tui"
)

var Version = "dev"

var (
	isTerminalInteractive = isInteractive
	pickTasks             = tui.Pick
)

type Options struct {
	Args           []string
	WorkDir        string
	Stdout         io.Writer
	Stderr         io.Writer
	Stdin          io.Reader
	NonInteractive bool
}

type state struct {
	json     bool
	yes      bool
	mode     string
	vars     map[string]string
	logFile  string
	parseErr error
}

func Execute(opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.WorkDir == "" {
		cwd, _ := os.Getwd()
		opts.WorkDir = cwd
	}

	st := &state{}
	if directDynamicDispatch(opts.Args) {
		filteredArgs := parseDynamicFlags(opts.Args, st)
		if err := runRoot(optsWithArgs(opts, filteredArgs), st, filteredArgs); err != nil {
			fmt.Fprintln(opts.Stderr, err)
			return exitCode(err)
		}
		return 0
	}
	cmd := newRootCommand(opts, st)
	cmd.SetArgs(opts.Args)
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(opts.Stderr, err)
		return exitCode(err)
	}
	return 0
}

// exitCode is a failed task's own exit status, 2 for usage errors, and 1 for
// anything else.
func exitCode(err error) int {
	if status, ok := shell.ExitStatus(err); ok {
		return status
	}
	if runner.IsUsage(err) || errors.Is(err, discovery.ErrNoWorkspace) {
		return 2
	}
	return 1
}

func directDynamicDispatch(args []string) bool {
	first := firstNonGlobalFlag(args)
	if first == "" {
		return false
	}
	switch first {
	case "help", "--help", "-h", "--version", "-v", "completion", "projects", "list", "tasks", "explain", "mirror", "__complete", "__completeNoDesc":
		return false
	default:
		return true
	}
}

func firstNonGlobalFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json", "--yes", "-y":
			continue
		case "--mode", "--log-file", "--var":
			i++
			continue
		default:
			if strings.HasPrefix(args[i], "--mode=") || strings.HasPrefix(args[i], "--log-file=") || strings.HasPrefix(args[i], "--var=") {
				continue
			}
			return args[i]
		}
	}
	return ""
}

// parseDynamicFlags removes sd's own flags from args before task dispatch.
// Everything after `--` is left for the task. A flag missing its value is
// recorded in st.parseErr.
func parseDynamicFlags(args []string, st *state) []string {
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return append(filtered, args[i:]...)
		}
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--json":
			st.json = true
			continue
		case "--yes", "-y":
			st.yes = true
			continue
		case "--mode", "--log-file", "--var":
		default:
			filtered = append(filtered, arg)
			continue
		}
		if !hasValue {
			if i+1 >= len(args) || args[i+1] == "--" {
				st.setParseErr(fmt.Errorf("%s needs a value", name))
				continue
			}
			value = args[i+1]
			i++
		}
		switch name {
		case "--mode":
			st.mode = value
		case "--log-file":
			st.logFile = value
		case "--var":
			if err := addRuntimeVar(st, value); err != nil {
				st.setParseErr(err)
			}
		}
	}
	return filtered
}

func (st *state) setParseErr(err error) {
	if st.parseErr == nil {
		st.parseErr = err
	}
}

func optsWithArgs(opts Options, args []string) Options {
	opts.Args = args
	return opts
}

func addRuntimeVar(st *state, assignment string) error {
	name, value, ok := strings.Cut(assignment, "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return fmt.Errorf("--var expects name=value")
	}
	if st.vars == nil {
		st.vars = map[string]string{}
	}
	st.vars[name] = value
	return nil
}

func newRootCommand(opts Options, st *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sd [project] [task]",
		Short:   "Self Deprecated project task runner",
		Version: Version,
		Args:    cobra.ArbitraryArgs,
		RunE:    func(cmd *cobra.Command, args []string) error { return runRoot(opts, st, args) },
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return complete(opts.WorkDir, args, toComplete), cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.FParseErrWhitelist.UnknownFlags = true
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return runner.UsageError{Message: err.Error()}
	})
	cmd.PersistentFlags().BoolVar(&st.json, "json", false, "emit JSON")
	cmd.PersistentFlags().BoolVarP(&st.yes, "yes", "y", false, "confirm dangerous tasks")
	cmd.PersistentFlags().StringVar(&st.mode, "mode", "", "select task execution mode")
	cmd.PersistentFlags().StringVar(&st.logFile, "log-file", "", "tee sd and task output to a file")
	cmd.PersistentFlags().Func("var", "set runtime variable (name=value)", func(value string) error {
		return addRuntimeVar(st, value)
	})
	cmd.AddCommand(projectsCommand(opts, st))
	cmd.AddCommand(tasksCommand(opts, st))
	cmd.AddCommand(explainCommand(opts, st))
	cmd.AddCommand(mirrorCommand(opts, st))
	return cmd
}

func runRoot(opts Options, st *state, args []string) error {
	if st.parseErr != nil {
		return runner.UsageError{Message: st.parseErr.Error()}
	}
	var closeLog func()
	var err error
	opts, closeLog, err = withOutputLog(opts, st)
	if err != nil {
		return err
	}
	defer closeLog()
	ws, err := discovery.LoadWorkspace(opts.WorkDir)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout) {
			picked, ok, err := pickTasks(ws, nil, opts.Stdout, opts.Stdin)
			if err != nil || !ok {
				return err
			}
			picked = parseDynamicFlags(picked, st)
			return dispatch(ws, picked, opts, st)
		}
		return listProjects(ws, opts.Stdout, st.json)
	}
	return dispatch(ws, args, opts, st)
}

func withOutputLog(opts Options, st *state) (Options, func(), error) {
	if st.logFile == "" {
		return opts, func() {}, nil
	}
	path := st.logFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(opts.WorkDir, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return opts, func() {}, err
	}
	file, err := os.Create(path)
	if err != nil {
		return opts, func() {}, err
	}
	opts.Stdout = io.MultiWriter(opts.Stdout, file)
	opts.Stderr = io.MultiWriter(opts.Stderr, file)
	fmt.Fprintf(opts.Stderr, "sd: logging output to %s\n", path)
	return opts, func() { _ = file.Close() }, nil
}

func dispatch(ws *model.Workspace, args []string, opts Options, st *state) error {
	if len(args) == 0 {
		return listProjects(ws, opts.Stdout, st.json)
	}
	if task, ok := resolveAggregate(ws, args); ok {
		return runner.RunAggregate(ws, task, runOptions(opts, st))
	}
	if children := aggregateChildren(ws, model.JoinPath(args)); len(children) > 0 {
		if !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout) {
			picked, ok, err := pickTasks(ws, args, opts.Stdout, opts.Stdin)
			if err != nil || !ok {
				return err
			}
			picked = parseDynamicFlags(picked, st)
			return dispatch(ws, picked, opts, st)
		}
		if len(args) == 1 {
			return listChildren(opts.Stdout, children)
		}
		return childUsage(strings.Join(args, " "), children)
	}
	project, ok := ws.ProjectByID(args[0])
	if !ok {
		return runner.UsageError{Message: unknownProjectMessage(ws, args[0])}
	}
	if len(args) == 1 {
		if !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout) {
			picked, ok, err := pickTasks(ws, args, opts.Stdout, opts.Stdin)
			if err != nil || !ok {
				return err
			}
			picked = parseDynamicFlags(picked, st)
			return dispatch(ws, picked, opts, st)
		}
		return listOneProjectTasks(project, opts, st.json)
	}
	task, extra, ok := resolveProjectTask(project, args[1:])
	if ok {
		if len(task.Choices) > 0 {
			if len(extra) == 0 {
				if !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout) {
					picked, ok, err := pickTasks(ws, append([]string{project.ID}, strings.Split(task.ID, "/")...), opts.Stdout, opts.Stdin)
					if err != nil || !ok {
						return err
					}
					picked = parseDynamicFlags(picked, st)
					return dispatch(ws, picked, opts, st)
				}
				return choiceUsage(project.ID+" "+task.ID, task)
			}
			choiceID := extra[0]
			choiceTask, ok := task.ChoiceTask(choiceID)
			if !ok {
				return choiceUsage(project.ID+" "+task.ID, task)
			}
			return runner.RunProjectTask(ws, project, choiceTask, normalizeTaskArgs(choiceTask, extra[1:]), runOptions(opts, st))
		}
		return runner.RunProjectTask(ws, project, task, normalizeTaskArgs(task, extra), runOptions(opts, st))
	}
	prefix := model.JoinPath(args[1:])
	children := model.ChildTasks(project.Tasks, prefix)
	if len(children) == 0 && len(args) > 2 {
		prefix = model.JoinPath(args[1 : len(args)-1])
		children = model.ChildTasks(project.Tasks, prefix)
	}
	if len(children) > 0 {
		if !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout) {
			picked, ok, err := pickTasks(ws, args, opts.Stdout, opts.Stdin)
			if err != nil || !ok {
				return err
			}
			picked = parseDynamicFlags(picked, st)
			return dispatch(ws, picked, opts, st)
		}
		return childUsage(project.ID+" "+strings.ReplaceAll(prefix, "/", " "), children)
	}
	return runner.UsageError{Message: fmt.Sprintf("unknown %s task: %s", project.ID, strings.Join(args[1:], " "))}
}

// normalizeTaskArgs drops the `--` that separates task arguments from sd's
// own flags.
func normalizeTaskArgs(task model.Task, args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return append(append([]string{}, args[:i]...), args[i+1:]...)
		}
	}
	return args
}

func resolveProjectTask(project *model.Project, parts []string) (model.Task, []string, bool) {
	for i := len(parts); i >= 1; i-- {
		id := model.JoinPath(parts[:i])
		if task, ok := project.Task(id); ok {
			return task, parts[i:], true
		}
	}
	return model.Task{}, nil, false
}

func resolveAggregate(ws *model.Workspace, args []string) (model.Task, bool) {
	id := model.JoinPath(args)
	task, ok := ws.Aggregates[id]
	return task, ok
}

func aggregateChildren(ws *model.Workspace, prefix string) []model.Task {
	return model.ChildTasks(ws.Aggregates, prefix)
}

func runOptions(opts Options, st *state) runner.Options {
	return runner.Options{Stdout: opts.Stdout, Stderr: opts.Stderr, Stdin: opts.Stdin, Yes: st.yes, AllowPrompt: !opts.NonInteractive && isTerminalInteractive(opts.Stdin, opts.Stdout), Mode: st.mode, Vars: st.vars}
}

func listChildren(out io.Writer, children []model.Task) error {
	for _, child := range children {
		name := lastSegment(child.ID)
		if child.Desc != "" {
			fmt.Fprintf(out, "%-18s %s\n", name, child.Desc)
		} else {
			fmt.Fprintln(out, name)
		}
	}
	return nil
}

func childUsage(prefix string, children []model.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s needs one of:\n\n", strings.TrimSpace(prefix))
	for _, child := range children {
		name := lastSegment(child.ID)
		if child.Desc != "" {
			fmt.Fprintf(&b, "  %-18s %s\n", name, child.Desc)
		} else {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}
	fmt.Fprintf(&b, "\nRun `sd %s <task>`.\n", strings.TrimSpace(prefix))
	return runner.UsageError{Message: b.String()}
}

func choiceUsage(prefix string, task model.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s needs a choice:\n\n", strings.TrimSpace(prefix))
	for _, id := range task.SortedChoiceIDs() {
		choice := task.Choices[id]
		desc := choice.Desc
		if desc == "" {
			desc = choice.Label
		}
		if choice.Danger {
			desc = strings.TrimSpace(desc + " ⚠ dangerous")
		}
		if desc != "" {
			fmt.Fprintf(&b, "  %-18s %s\n", id, desc)
		} else {
			fmt.Fprintf(&b, "  %s\n", id)
		}
	}
	fmt.Fprintf(&b, "\nRun `sd %s <choice>`.\n", strings.TrimSpace(prefix))
	return runner.UsageError{Message: b.String()}
}

func lastSegment(id string) string {
	parts := strings.Split(strings.Trim(id, "/"), "/")
	return parts[len(parts)-1]
}

func unknownProjectMessage(ws *model.Workspace, got string) string {
	ids := make([]string, 0, len(ws.Projects))
	for _, p := range ws.Projects {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	message := fmt.Sprintf("unknown project: %s", got)
	if suggestion := nearest(got, ids); suggestion != "" {
		message += fmt.Sprintf("\n\nDid you mean `%s`?", suggestion)
	}
	return message + fmt.Sprintf("\n\nAvailable projects:\n  %s", strings.Join(ids, "\n  "))
}

func nearest(got string, candidates []string) string {
	best := ""
	bestDistance := 999
	for _, candidate := range candidates {
		distance := editDistance(got, candidate)
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	if bestDistance <= 3 || bestDistance <= len(got)/2 {
		return best
	}
	return ""
}

func editDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	dp := make([][]int, len(ar)+1)
	for i := range dp {
		dp[i] = make([]int, len(br)+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}
	for i := 1; i <= len(ar); i++ {
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			dp[i][j] = min(dp[i-1][j]+1, dp[i][j-1]+1, dp[i-1][j-1]+cost)
		}
	}
	return dp[len(ar)][len(br)]
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(value)
}

func isInteractive(stdin io.Reader, stdout io.Writer) bool {
	inFile, inOK := stdin.(*os.File)
	outFile, outOK := stdout.(*os.File)
	if !inOK || !outOK {
		return false
	}
	inStat, err := inFile.Stat()
	if err != nil || inStat.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	outStat, err := outFile.Stat()
	return err == nil && outStat.Mode()&os.ModeCharDevice != 0
}
