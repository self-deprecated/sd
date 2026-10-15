package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/discovery"
	"github.com/self-deprecated/sd/internal/mirror"
	"github.com/self-deprecated/sd/internal/runner"
	"github.com/self-deprecated/sd/internal/shell"
	"github.com/self-deprecated/sd/internal/tui"
)

type mirrorFlags struct {
	mirrorRoot string
	message    string
	opLimit    int
	allOps     bool
}

type mirrorExplain struct {
	Version     int      `json:"version"`
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Bookmark    string   `json:"bookmark,omitempty"`
	Remote      string   `json:"remote,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
	Include     []string `json:"include,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	Preserve    []string `json:"preserve,omitempty"`
	Overlays    any      `json:"overlays,omitempty"`
	Checks      []string `json:"checks,omitempty"`
}

func mirrorCommand(opts Options, st *state) *cobra.Command {
	flags := &mirrorFlags{}
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Export public Projects to local public mirrors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			if opts.NonInteractive || !isMirrorInteractive(opts.Stdin, opts.Stdout) || st.json {
				return listMirrors(ctx, opts, st.json)
			}
			id, err := pickMirrorID(ctx, opts)
			if err != nil {
				return err
			}
			action, err := pickMirrorAction(opts)
			if err != nil {
				return err
			}
			return runPickedMirrorAction(ctx, id, action, opts, st, flags)
		},
	}
	cmd.PersistentFlags().StringVar(&flags.mirrorRoot, "mirror-root", "", "override destination root for public-mirrors")
	cmd.PersistentFlags().IntVar(&flags.opLimit, "op-limit", 100, "maximum file operations to print per operation type in text plans")
	cmd.PersistentFlags().BoolVar(&flags.allOps, "all-ops", false, "print every file operation in text plans")
	cmd.AddCommand(mirrorListCommand(opts, st, flags))
	cmd.AddCommand(mirrorExplainCommand(opts, st, flags))
	cmd.AddCommand(mirrorInitCommand(opts, flags))
	cmd.AddCommand(mirrorPlanCommand(opts, st, flags))
	cmd.AddCommand(mirrorCheckCommand(opts, st, flags))
	cmd.AddCommand(mirrorSyncCommand(opts, st, flags))
	cmd.AddCommand(mirrorCommitCommand(opts, st, flags, "commit"))
	cmd.AddCommand(mirrorCommitCommand(opts, st, flags, "publish"))
	return cmd
}

func mirrorListCommand(opts Options, st *state, flags *mirrorFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured public mirrors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			return listMirrors(ctx, opts, st.json)
		},
	}
}

func mirrorExplainCommand(opts Options, st *state, flags *mirrorFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "explain [mirror]",
		Short: "Show public mirror configuration",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			id, err := mirrorIDFromArgs(ctx, args, opts, "explain")
			if err != nil {
				return err
			}
			return runMirrorExplain(ctx, id, opts, st)
		},
	}
}

func mirrorInitCommand(opts Options, flags *mirrorFlags) *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "init --root <path>",
		Short: "Write local public mirror path configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				return runner.UsageError{Message: "sd mirror init requires --root"}
			}
			ws, err := discovery.LoadWorkspace(opts.WorkDir)
			if err != nil {
				return err
			}
			path := filepath.Join(ws.Root, ".sd", "mirrors.local.yaml")
			content := fmt.Sprintf("version: 1\n\npaths:\n  public-mirrors: %s\n", root)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(opts.Stdout, "wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "local directory containing public mirror checkouts")
	return cmd
}

func mirrorPlanCommand(opts Options, st *state, flags *mirrorFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "plan [mirror]",
		Short: "Preview a public mirror export without modifying files",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			id, err := mirrorIDFromArgs(ctx, args, opts, "plan")
			if err != nil {
				return err
			}
			return runMirrorPlan(ctx, id, opts, st, flags)
		},
	}
}

func mirrorCheckCommand(opts Options, st *state, flags *mirrorFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "check [mirror]",
		Short: "Run blacklist scan and configured checks without syncing",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			id, err := mirrorIDFromArgs(ctx, args, opts, "check")
			if err != nil {
				return err
			}
			return runMirrorCheck(ctx, id, opts, st)
		},
	}
}

func mirrorSyncCommand(opts Options, st *state, flags *mirrorFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "sync [mirror]",
		Short: "Sync a sanitized Project snapshot into a local public mirror",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			id, err := mirrorIDFromArgs(ctx, args, opts, "sync")
			if err != nil {
				return err
			}
			return runMirrorSync(ctx, id, opts, st)
		},
	}
}

func mirrorCommitCommand(opts Options, st *state, flags *mirrorFlags, name string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name + " [mirror] -m <message>",
		Short: "Sync and create a local mirror commit; does not push",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadMirrorContext(opts, flags)
			if err != nil {
				return err
			}
			id, err := mirrorIDFromArgs(ctx, args, opts, name)
			if err != nil {
				return err
			}
			message, err := mirrorCommitMessage(name, flags.message, opts)
			if err != nil {
				return err
			}
			return runMirrorCommit(ctx, id, message, opts, st)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "public mirror commit message")
	return cmd
}

type mirrorContext struct {
	root         string
	config       mirror.ConfigSet
	cliOverrides map[string]string
	vcs          mirror.VCS
}

func loadMirrorContext(opts Options, flags *mirrorFlags) (mirrorContext, error) {
	ws, err := discovery.LoadWorkspace(opts.WorkDir)
	if err != nil {
		return mirrorContext{}, err
	}
	cs, err := mirror.LoadConfig(ws.Root)
	if err != nil {
		return mirrorContext{}, err
	}
	overrides := map[string]string{}
	if flags.mirrorRoot != "" {
		overrides["public-mirrors"] = flags.mirrorRoot
	}
	return mirrorContext{root: ws.Root, config: cs, cliOverrides: overrides, vcs: mirror.JJVCS{}}, nil
}

func listMirrors(ctx mirrorContext, opts Options, asJSON bool) error {
	ids := ctx.config.SortedMirrorIDs()
	if asJSON {
		type listedMirror struct {
			ID         string `json:"id"`
			Visibility string `json:"visibility"`
		}
		mirrors := []listedMirror{}
		for _, id := range ids {
			cfg, _ := ctx.config.Mirror(id)
			mirrors = append(mirrors, listedMirror{ID: id, Visibility: cfg.Visibility})
		}
		return writeJSON(opts.Stdout, struct {
			Version int            `json:"version"`
			Mirrors []listedMirror `json:"mirrors"`
		}{1, mirrors})
	}
	for _, id := range ids {
		cfg, _ := ctx.config.Mirror(id)
		fmt.Fprintf(opts.Stdout, "%-18s %s\n", id, cfg.Visibility)
	}
	return nil
}

func mirrorIDFromArgs(ctx mirrorContext, args []string, opts Options, action string) (string, error) {
	if len(args) > 0 {
		if _, ok := ctx.config.Mirror(args[0]); !ok {
			return "", runner.UsageError{Message: fmt.Sprintf("unknown mirror: %s", args[0])}
		}
		return args[0], nil
	}
	if !opts.NonInteractive && isMirrorInteractive(opts.Stdin, opts.Stdout) {
		return pickMirrorID(ctx, opts)
	}
	return "", runner.UsageError{Message: fmt.Sprintf("sd mirror %s needs a mirror\n\nAvailable mirrors:\n  %s", action, strings.Join(ctx.config.SortedMirrorIDs(), "\n  "))}
}

var pickMirrorChoices = tui.PickChoices

func pickMirrorID(ctx mirrorContext, opts Options) (string, error) {
	var choices []tui.Choice
	for _, id := range ctx.config.SortedMirrorIDs() {
		cfg, _ := ctx.config.Mirror(id)
		choices = append(choices, tui.Choice{Title: id, Description: cfg.Visibility, Args: []string{id}})
	}
	picked, ok, err := pickMirrorChoices("SD mirrors", choices, opts.Stdout, opts.Stdin)
	if err != nil || !ok {
		return "", err
	}
	return picked[0], nil
}

func pickMirrorAction(opts Options) (string, error) {
	choices := []tui.Choice{
		{Title: "plan", Description: "Preview export without modifying files", Args: []string{"plan"}},
		{Title: "check", Description: "Run blacklist scan and configured checks", Args: []string{"check"}},
		{Title: "sync", Description: "Sync into local mirror without committing", Args: []string{"sync"}},
		{Title: "commit", Description: "Sync and create a local mirror commit", Args: []string{"commit"}},
		{Title: "publish", Description: "Local sync+commit alias; does not push", Args: []string{"publish"}},
		{Title: "explain", Description: "Show mirror configuration", Args: []string{"explain"}},
	}
	picked, ok, err := pickMirrorChoices("SD mirror actions", choices, opts.Stdout, opts.Stdin)
	if err != nil || !ok {
		return "", err
	}
	return picked[0], nil
}

func runPickedMirrorAction(ctx mirrorContext, id, action string, opts Options, st *state, flags *mirrorFlags) error {
	switch action {
	case "explain":
		return runMirrorExplain(ctx, id, opts, st)
	case "plan":
		return runMirrorPlan(ctx, id, opts, st, flags)
	case "check":
		return runMirrorCheck(ctx, id, opts, st)
	case "sync":
		return runMirrorSync(ctx, id, opts, st)
	case "commit", "publish":
		message, err := mirrorCommitMessage(action, flags.message, opts)
		if err != nil {
			return err
		}
		return runMirrorCommit(ctx, id, message, opts, st)
	default:
		return runner.UsageError{Message: fmt.Sprintf("unknown mirror action: %s", action)}
	}
}

func mirrorCommitMessage(action, message string, opts Options) (string, error) {
	if strings.TrimSpace(message) != "" {
		return message, nil
	}
	if opts.NonInteractive || !isMirrorInteractive(opts.Stdin, opts.Stdout) {
		return "", runner.UsageError{Message: "mirror " + action + " requires -m/--message"}
	}
	fmt.Fprintf(opts.Stderr, "Commit message: ")
	line, err := bufio.NewReader(opts.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", runner.UsageError{Message: "mirror " + action + " requires a non-empty commit message"}
	}
	return line, nil
}

func runMirrorExplain(ctx mirrorContext, id string, opts Options, st *state) error {
	cfg, ok := ctx.config.Mirror(id)
	if !ok {
		return runner.UsageError{Message: fmt.Sprintf("unknown mirror: %s", id)}
	}
	destination, err := ctx.resolveDestination(id, cfg)
	if err != nil {
		return formatResolveError(err)
	}
	exp := mirrorExplain{Version: 1, ID: id, Source: cfg.Source, Destination: destination, Bookmark: cfg.Bookmark, Remote: cfg.Remote, Visibility: cfg.Visibility, Include: cfg.Include, Exclude: cfg.Exclude, Preserve: cfg.Preserve, Overlays: cfg.Overlays, Checks: cfg.Checks}
	if st.json {
		return writeJSON(opts.Stdout, exp)
	}
	fmt.Fprintf(opts.Stdout, "%s\n", exp.ID)
	fmt.Fprintf(opts.Stdout, "  source:      %s\n", exp.Source)
	fmt.Fprintf(opts.Stdout, "  destination: %s\n", exp.Destination)
	if exp.Visibility != "" {
		fmt.Fprintf(opts.Stdout, "  visibility:  %s\n", exp.Visibility)
	}
	if len(exp.Checks) > 0 {
		fmt.Fprintf(opts.Stdout, "  checks:      %s\n", strings.Join(exp.Checks, ", "))
	}
	return nil
}

func runMirrorPlan(ctx mirrorContext, id string, opts Options, st *state, flags *mirrorFlags) error {
	plan, err := ctx.buildPlan(id)
	if err != nil {
		return err
	}
	plan.MirrorStatus = ctx.destinationStatus(plan.Destination)
	if st.json {
		return writeJSON(opts.Stdout, struct {
			Version int `json:"version"`
			mirror.Plan
		}{1, plan})
	}
	return printPlan(opts.Stdout, plan, flags)
}

// mirrorCheckResult is the output of `sd mirror check --json` (version 1).
type mirrorCheckResult struct {
	Version       int                  `json:"version"`
	Mirror        string               `json:"mirror"`
	Passed        bool                 `json:"passed"`
	ExportedFiles int                  `json:"exportedFiles"`
	Findings      []mirror.ScanFinding `json:"findings"`
	Allowed       []mirror.ScanFinding `json:"allowed"`
}

func runMirrorCheck(ctx mirrorContext, id string, opts Options, st *state) error {
	plan, err := ctx.buildPlan(id)
	if err != nil {
		return err
	}
	if st.json {
		return runMirrorCheckJSON(plan, opts)
	}
	if err := scanMirrorPlan(plan, opts); err != nil {
		return err
	}
	if err := runMirrorChecks(plan, opts); err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "mirror %s passed blacklist scan", id)
	if len(plan.Checks) > 0 {
		fmt.Fprintf(opts.Stdout, " and %d configured check(s)", len(plan.Checks))
	}
	fmt.Fprintln(opts.Stdout)
	return nil
}

// runMirrorCheckJSON reports the scan and checks as JSON; configured checks
// write to stderr so stdout stays parseable.
func runMirrorCheckJSON(plan mirror.Plan, opts Options) error {
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		return err
	}
	result := mirrorCheckResult{Version: 1, Mirror: plan.MirrorID, ExportedFiles: len(plan.Files), Findings: append([]mirror.ScanFinding{}, report.Findings...), Allowed: append([]mirror.ScanFinding{}, report.Allowed...)}
	var checkErr error
	if !report.HasFindings() {
		checkOpts := opts
		checkOpts.Stdout = opts.Stderr
		checkErr = runMirrorChecks(plan, checkOpts)
	}
	result.Passed = !report.HasFindings() && checkErr == nil
	if err := writeJSON(opts.Stdout, result); err != nil {
		return err
	}
	if report.HasFindings() {
		return runner.UsageError{Message: report.Error()}
	}
	return checkErr
}

func runMirrorSync(ctx mirrorContext, id string, opts Options, st *state) error {
	plan, err := ctx.sync(id, opts, st)
	if err != nil {
		return err
	}
	printSyncSummary(opts.Stdout, plan)
	fmt.Fprintln(opts.Stdout, "No remote push was performed.")
	return nil
}

func runMirrorCommit(ctx mirrorContext, id, publicMessage string, opts Options, st *state) error {
	plan, err := ctx.sync(id, opts, st)
	if err != nil {
		return err
	}
	if err := runMirrorChecks(plan, opts); err != nil {
		return err
	}
	status, err := ctx.vcs.Status(plan.Destination)
	if err != nil {
		return err
	}
	if status.Clean {
		fmt.Fprintf(opts.Stdout, "mirror %s has no changes after sync; no commit created\n", id)
		fmt.Fprintln(opts.Stdout, "No remote push was performed.")
		return nil
	}
	// Checks may have changed the mirror after the export scan, so scan what
	// the commit will actually record.
	changed, err := ctx.vcs.ChangedFiles(plan.Destination)
	if err != nil {
		return err
	}
	report, err := mirror.ScanFiles(plan.Destination, changed, plan.Allow)
	if err != nil {
		return err
	}
	if report.HasFindings() {
		return runner.UsageError{Message: fmt.Sprintf("%s\nNo commit was created; remove these files from %s first.", report.Error(), plan.Destination)}
	}
	message, traceErr := ctx.commitMessage(id, publicMessage)
	if traceErr != nil {
		fmt.Fprintf(opts.Stderr, "warning: could not add monorepo trace trailers: %v\n", traceErr)
	}
	if err := ctx.vcs.Commit(plan.Destination, message); err != nil {
		return err
	}
	fmt.Fprintf(opts.Stdout, "created local mirror commit for %s\n", id)
	fmt.Fprintln(opts.Stdout, "No remote push was performed. Review the mirror and push manually when ready.")
	return nil
}

func (ctx mirrorContext) resolveDestination(id string, cfg config.MirrorConfig) (string, error) {
	return mirror.ResolveDestination(id, cfg, mirror.ResolveOptions{WorkspaceRoot: ctx.root, CLIPathOverrides: ctx.cliOverrides, LocalPaths: ctx.config.Local.Paths})
}

func (ctx mirrorContext) buildPlan(id string) (mirror.Plan, error) {
	cfg, ok := ctx.config.Mirror(id)
	if !ok {
		return mirror.Plan{}, runner.UsageError{Message: fmt.Sprintf("unknown mirror: %s", id)}
	}
	destination, err := ctx.resolveDestination(id, cfg)
	if err != nil {
		return mirror.Plan{}, formatResolveError(err)
	}
	return mirror.BuildPlan(id, ctx.root, cfg, destination)
}

func (ctx mirrorContext) sync(id string, opts Options, st *state) (mirror.Plan, error) {
	plan, err := ctx.buildPlan(id)
	if err != nil {
		return mirror.Plan{}, err
	}
	if err := scanMirrorPlan(plan, opts); err != nil {
		return mirror.Plan{}, err
	}
	if err := validateMirrorDestination(ctx.vcs, id, plan.Destination); err != nil {
		return mirror.Plan{}, err
	}
	if err := guardDirtyMirror(ctx.vcs, id, plan.Destination, opts, st); err != nil {
		return mirror.Plan{}, err
	}
	if err := mirror.ApplyPlan(plan); err != nil {
		return mirror.Plan{}, err
	}
	return plan, nil
}

func (ctx mirrorContext) destinationStatus(destination string) string {
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "unknown"
	}
	if !info.IsDir() {
		return "not-directory"
	}
	ok, err := ctx.vcs.IsJJWorkcopy(destination)
	if err != nil || !ok {
		return "not-jj-workcopy"
	}
	status, err := ctx.vcs.Status(destination)
	if err != nil {
		return "unknown"
	}
	if status.Clean {
		return "clean"
	}
	return "dirty"
}

func (ctx mirrorContext) commitMessage(id, message string) (string, error) {
	trace, err := ctx.vcs.CurrentChange(ctx.root)
	if err != nil {
		return message, err
	}
	if trace.ChangeID == "" && trace.CommitID == "" {
		return message, fmt.Errorf("jj trace lookup returned no change or commit id")
	}
	return strings.TrimRight(message, "\n") + fmt.Sprintf("\n\nExport-Profile: %s\nMonorepo-Change: %s\nMonorepo-Commit: %s\n", id, trace.ChangeID, trace.CommitID), nil
}

func validateMirrorDestination(vcs mirror.VCS, id, destination string) error {
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return runner.UsageError{Message: fmt.Sprintf("mirror %s destination does not exist: %s", id, destination)}
		}
		return err
	}
	if !info.IsDir() {
		return runner.UsageError{Message: fmt.Sprintf("mirror %s destination is not a directory: %s", id, destination)}
	}
	ok, err := vcs.IsJJWorkcopy(destination)
	if err != nil {
		return err
	}
	if !ok {
		return runner.UsageError{Message: fmt.Sprintf("mirror %s destination is not a jj workcopy root: %s", id, destination)}
	}
	return nil
}

func guardDirtyMirror(vcs mirror.VCS, id, destination string, opts Options, st *state) error {
	status, err := vcs.Status(destination)
	if err != nil {
		return err
	}
	if status.Clean || st.yes {
		return nil
	}
	message := fmt.Sprintf("Mirror %s has uncommitted local changes at %s.\nContinuing may mix existing local changes with this export.", id, destination)
	if opts.NonInteractive || !isMirrorInteractive(opts.Stdin, opts.Stdout) {
		return runner.UsageError{Message: message + "\nRerun with --yes to continue."}
	}
	fmt.Fprintf(opts.Stderr, "%s\nContinue? [y/N] ", message)
	answer, err := bufio.NewReader(opts.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return runner.UsageError{Message: fmt.Sprintf("cancelled mirror %s sync", id)}
	}
	return nil
}

func scanMirrorPlan(plan mirror.Plan, opts Options) error {
	report, err := mirror.ScanPlan(plan)
	if err != nil {
		return err
	}
	if report.HasFindings() {
		return runner.UsageError{Message: report.Error()}
	}
	fmt.Fprintf(opts.Stdout, "mirror %s blacklist scan passed (%d exported files)\n", plan.MirrorID, len(plan.Files))
	for _, finding := range report.Allowed {
		fmt.Fprintf(opts.Stdout, "  allowed by config: %s: %s\n", finding.Path, finding.Reason)
	}
	return nil
}

func runMirrorChecks(plan mirror.Plan, opts Options) error {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	for _, check := range plan.Checks {
		if err := shell.Run(interrupts, plan.WorkspaceRoot, check, nil, os.Environ(), opts.Stdin, opts.Stdout, opts.Stderr); err != nil {
			return err
		}
	}
	return nil
}

func formatResolveError(err error) error {
	var missing mirror.MissingPathVariableError
	if errors.As(err, &missing) {
		envName := mirror.EnvNameForPathVariable(missing.Variable)
		return runner.UsageError{Message: fmt.Sprintf("%s\n\nConfigure one of:\n  sd mirror init --root /path/to/mirrors\n  %s=/path/to/mirrors sd mirror plan %s", err.Error(), envName, missing.MirrorID)}
	}
	return err
}

func printPlan(out io.Writer, plan mirror.Plan, flags *mirrorFlags) error {
	fmt.Fprintf(out, "%s\n", plan.MirrorID)
	fmt.Fprintf(out, "  source:      %s\n", plan.Source)
	fmt.Fprintf(out, "  destination: %s\n", plan.Destination)
	if plan.MirrorStatus != "" {
		fmt.Fprintf(out, "  mirror:      %s\n", plan.MirrorStatus)
	}
	if len(plan.Include) > 0 {
		fmt.Fprintf(out, "  include:     %s\n", strings.Join(plan.Include, ", "))
	}
	if len(plan.Exclude) > 0 {
		fmt.Fprintf(out, "  exclude:     %s\n", strings.Join(plan.Exclude, ", "))
	}
	if len(plan.Preserve) > 0 {
		fmt.Fprintf(out, "  preserve:    %s\n", strings.Join(plan.Preserve, ", "))
	}
	fmt.Fprintf(out, "  creates:     %d\n", len(plan.Creates))
	fmt.Fprintf(out, "  updates:     %d\n", len(plan.Updates))
	fmt.Fprintf(out, "  deletes:     %d\n", len(plan.Deletes))
	if len(plan.Overlays) > 0 {
		var overlays []string
		for _, op := range plan.Overlays {
			overlays = append(overlays, op.From+" -> "+op.To)
		}
		fmt.Fprintf(out, "  overlays:    %s\n", strings.Join(overlays, ", "))
	}
	if len(plan.Checks) > 0 {
		fmt.Fprintf(out, "  checks:      %s\n", strings.Join(plan.Checks, ", "))
	}
	printOpList(out, "create", plan.Creates, flags)
	printOpList(out, "update", plan.Updates, flags)
	printOpList(out, "delete", plan.Deletes, flags)
	return nil
}

func printOpList(out io.Writer, label string, ops []mirror.FileOp, flags *mirrorFlags) {
	limit := len(ops)
	if flags != nil && !flags.allOps && flags.opLimit >= 0 && limit > flags.opLimit {
		limit = flags.opLimit
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintf(out, "  %s %s\n", label, ops[i].Path)
	}
	if limit < len(ops) {
		fmt.Fprintf(out, "  ... %d more %s operation(s); rerun with --all-ops or raise --op-limit\n", len(ops)-limit, label)
	}
}

func printSyncSummary(out io.Writer, plan mirror.Plan) {
	fmt.Fprintf(out, "synced %s: %d created, %d updated, %d deleted\n", plan.MirrorID, len(plan.Creates), len(plan.Updates), len(plan.Deletes))
}

var isMirrorInteractive = isInteractive
