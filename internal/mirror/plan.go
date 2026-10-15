package mirror

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/self-deprecated/sd/internal/config"
	"github.com/self-deprecated/sd/internal/glob"
)

type FileOp struct {
	Path string `json:"path"`
}

type OverlayOp struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Plan struct {
	MirrorID      string      `json:"mirrorId"`
	WorkspaceRoot string      `json:"-"`
	Source        string      `json:"source"`
	Destination   string      `json:"destination"`
	MirrorStatus  string      `json:"mirrorStatus,omitempty"`
	Include       []string    `json:"include,omitempty"`
	Exclude       []string    `json:"exclude,omitempty"`
	Preserve      []string    `json:"preserve,omitempty"`
	Files         []FileOp    `json:"files,omitempty"`
	Creates       []FileOp    `json:"creates,omitempty"`
	Updates       []FileOp    `json:"updates,omitempty"`
	Deletes       []FileOp    `json:"deletes,omitempty"`
	Overlays      []OverlayOp `json:"overlays,omitempty"`
	Checks        []string    `json:"checks,omitempty"`
	Allow         []string    `json:"allow,omitempty"`
}

var alwaysProtected = []string{
	".git", ".git/**",
	".jj", ".jj/**",
}

func BuildPlan(mirrorID string, root string, cfg config.MirrorConfig, destination string) (Plan, error) {
	source := cfg.Source
	if source == "" {
		return Plan{}, fmt.Errorf("mirror %s source is required", mirrorID)
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(root, filepath.FromSlash(source))
	}
	source = filepath.Clean(source)
	if err := requireSeparateTrees(root, source, destination); err != nil {
		return Plan{}, fmt.Errorf("mirror %s %w", mirrorID, err)
	}
	include := cfg.Include
	if len(include) == 0 {
		include = []string{"**"}
	}
	for _, field := range []struct {
		name     string
		patterns []string
	}{{"include", include}, {"exclude", cfg.Exclude}, {"preserve", cfg.Preserve}, {"allow", cfg.Allow}} {
		if err := validatePatterns(mirrorID, field.name, field.patterns); err != nil {
			return Plan{}, err
		}
	}
	exclude := append([]string{}, cfg.Exclude...)
	exclude = append(exclude, alwaysProtected...)

	exportFiles, err := selectedFiles(source, include, exclude)
	if err != nil {
		return Plan{}, fmt.Errorf("mirror %s: %w", mirrorID, err)
	}
	// Overlay targets are exported even when the source helper file is excluded.
	var overlays []OverlayOp
	for _, overlay := range cfg.Overlays {
		op, fromAbs, err := resolveOverlay(source, overlay)
		if err != nil {
			return Plan{}, fmt.Errorf("mirror %s %w", mirrorID, err)
		}
		exportFiles[op.To] = fromAbs
		overlays = append(overlays, op)
	}

	preserve := append([]string{}, cfg.Preserve...)
	preserve = append(preserve, alwaysProtected...)
	destFiles, err := destinationFiles(destination, preserve)
	if err != nil {
		return Plan{}, fmt.Errorf("mirror %s: %w", mirrorID, err)
	}
	plan := Plan{MirrorID: mirrorID, WorkspaceRoot: root, Source: source, Destination: destination, Include: include, Exclude: exclude, Preserve: preserve, Checks: cfg.Checks, Allow: cfg.Allow}
	plan.Overlays = overlays
	for rel, srcPath := range exportFiles {
		plan.Files = append(plan.Files, FileOp{Path: rel})
		dstPath := filepath.Join(destination, filepath.FromSlash(rel))
		if _, ok := destFiles[rel]; !ok {
			plan.Creates = append(plan.Creates, FileOp{Path: rel})
			continue
		}
		same, err := sameFile(srcPath, dstPath)
		if err != nil {
			return Plan{}, err
		}
		if !same {
			plan.Updates = append(plan.Updates, FileOp{Path: rel})
		}
	}
	for rel := range destFiles {
		if _, ok := exportFiles[rel]; !ok && !isProtected(rel) {
			plan.Deletes = append(plan.Deletes, FileOp{Path: rel})
		}
	}
	sortOps(plan.Files)
	sortOps(plan.Creates)
	sortOps(plan.Updates)
	sortOps(plan.Deletes)
	sort.Slice(plan.Overlays, func(i, j int) bool { return plan.Overlays[i].To < plan.Overlays[j].To })
	return plan, nil
}

// resolveOverlay validates an overlay and returns it with cleaned paths and
// the absolute path of its source file. The source must be a regular file
// inside the source tree and the target must stay inside the mirror and
// outside its VCS directories.
func resolveOverlay(source string, overlay config.MirrorOverlay) (OverlayOp, string, error) {
	if overlay.From == "" || overlay.To == "" {
		return OverlayOp{}, "", fmt.Errorf("overlay requires from and to")
	}
	from, err := cleanRelPath(overlay.From)
	if err != nil {
		return OverlayOp{}, "", fmt.Errorf("overlay source %w", err)
	}
	to, err := cleanRelPath(overlay.To)
	if err != nil {
		return OverlayOp{}, "", fmt.Errorf("overlay target %w", err)
	}
	for _, segment := range strings.Split(to, "/") {
		if isProtectedName(segment) {
			return OverlayOp{}, "", fmt.Errorf("overlay target %q must not be inside %s", overlay.To, segment)
		}
	}
	fromAbs := filepath.Join(source, filepath.FromSlash(from))
	info, err := os.Lstat(fromAbs)
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("is not a regular file")
	}
	if err == nil {
		err = requireInside(source, fromAbs)
	}
	if err != nil {
		return OverlayOp{}, "", fmt.Errorf("overlay source %s: %w", overlay.From, err)
	}
	return OverlayOp{From: from, To: to}, fromAbs, nil
}

// cleanRelPath normalises a configured relative path, accepting / or \ as
// separators, and rejects empty, absolute, drive-qualified, and escaping paths.
func cleanRelPath(p string) (string, error) {
	slashed := strings.ReplaceAll(p, `\`, "/")
	// Drive letters are rejected on every OS because configs are shared.
	driveLetter := len(slashed) >= 2 && slashed[1] == ':'
	if slashed == "" || strings.HasPrefix(slashed, "/") || driveLetter || filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", fmt.Errorf("%q must be a relative path", p)
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || !filepath.IsLocal(filepath.FromSlash(cleaned)) {
		return "", fmt.Errorf("%q must name a file inside the tree", p)
	}
	return cleaned, nil
}

// requireInside returns an error unless target, with symlinks resolved, is
// inside root.
func requireInside(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	if !isWithin(realTarget, realRoot) {
		return fmt.Errorf("resolves outside %s", root)
	}
	return nil
}

// requireSeparateTrees returns an error when destination is, contains, or is
// inside the workspace root or the source, comparing paths with symlinks
// resolved.
func requireSeparateTrees(root, source, destination string) error {
	dest := resolvePath(destination)
	for _, other := range []struct{ name, path string }{{"workspace root", root}, {"source", source}} {
		resolved := resolvePath(other.path)
		if isWithin(dest, resolved) || isWithin(resolved, dest) {
			return fmt.Errorf("destination %s overlaps the %s %s; mirrors must be separate checkouts outside the monorepo", destination, other.name, other.path)
		}
	}
	return nil
}

// resolvePath returns p as an absolute path with symlinks resolved in its
// longest existing prefix, so paths that do not exist yet can be compared.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	missing := ""
	for current := abs; ; {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(real, missing)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		missing = filepath.Join(filepath.Base(current), missing)
		current = parent
	}
}

// isWithin reports whether path is root or inside it.
func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// selectedFiles returns the regular files to export, keyed by relative path.
// Symlinks are never followed or exported.
func selectedFiles(root string, include, exclude []string) (map[string]string, error) {
	out := map[string]string{}
	var irregular []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = slashPath(rel)
		if d.IsDir() {
			if isProtected(rel) || matchesAny(rel, exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(rel, include) && !matchesAny(rel, exclude) && !isProtected(rel) {
			if !d.Type().IsRegular() {
				irregular = append(irregular, rel)
				return nil
			}
			out[rel] = path
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(irregular) > 0 {
		return nil, fmt.Errorf("source has symlinks or special files that would be exported; exclude them or replace them with regular files:\n  %s", strings.Join(irregular, "\n  "))
	}
	return out, nil
}

// destinationFiles returns the regular files in the mirror that are not
// preserved. Mirrors may only hold symlinks inside preserved paths.
func destinationFiles(root string, preserve []string) (map[string]string, error) {
	out := map[string]string{}
	var irregular []string
	if _, err := os.Stat(root); err != nil {
		return out, nil
	}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = slashPath(rel)
		if d.IsDir() {
			if isProtected(rel) || matchesAny(rel, preserve) {
				return filepath.SkipDir
			}
			return nil
		}
		if !matchesAny(rel, preserve) {
			if !d.Type().IsRegular() {
				irregular = append(irregular, rel)
				return nil
			}
			out[rel] = path
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(irregular) > 0 {
		return nil, fmt.Errorf("destination has symlinks or special files outside preserved paths; remove them before syncing:\n  %s", strings.Join(irregular, "\n  "))
	}
	return out, nil
}

// sameFile reports whether b already has a's content and, outside Windows,
// a's executable bit. Content is compared in chunks rather than read whole.
func sameFile(a, b string) (bool, error) {
	ainfo, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	binfo, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if ainfo.Size() != binfo.Size() {
		return false, nil
	}
	if runtime.GOOS != "windows" && (ainfo.Mode()&0o111 == 0) != (binfo.Mode()&0o111 == 0) {
		return false, nil
	}
	af, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer af.Close()
	bf, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer bf.Close()
	abuf := make([]byte, 64*1024)
	bbuf := make([]byte, 64*1024)
	for {
		an, aerr := io.ReadFull(af, abuf)
		bn, berr := io.ReadFull(bf, bbuf)
		if !bytes.Equal(abuf[:an], bbuf[:bn]) {
			return false, nil
		}
		aend := aerr == io.EOF || aerr == io.ErrUnexpectedEOF
		bend := berr == io.EOF || berr == io.ErrUnexpectedEOF
		if aerr != nil && !aend {
			return false, aerr
		}
		if berr != nil && !bend {
			return false, berr
		}
		if aend || bend {
			return aend && bend, nil
		}
	}
}

func sortOps(ops []FileOp) {
	sort.Slice(ops, func(i, j int) bool { return ops[i].Path < ops[j].Path })
}

func slashPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func isProtected(rel string) bool {
	first, _, _ := strings.Cut(strings.Trim(slashPath(rel), "/"), "/")
	return isProtectedName(first)
}

// isProtectedName reports whether a path segment names a VCS directory,
// allowing for case-insensitive file systems and Windows dropping trailing
// dots and spaces.
func isProtectedName(segment string) bool {
	segment = strings.TrimRight(strings.ToLower(segment), ". ")
	return segment == ".git" || segment == ".jj"
}

// matchesAny reports whether rel matches any pattern. Patterns are validated
// by validatePatterns before use, so malformed patterns never match.
func matchesAny(rel string, patterns []string) bool {
	rel = strings.Trim(slashPath(rel), "/")
	for _, pattern := range patterns {
		if ok, _ := glob.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

func validatePatterns(mirrorID, field string, patterns []string) error {
	for _, pattern := range patterns {
		if err := glob.Validate(pattern); err != nil {
			return fmt.Errorf("mirror %s %s: %w", mirrorID, field, err)
		}
	}
	return nil
}
