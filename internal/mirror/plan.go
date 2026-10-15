package mirror

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
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
	include := cfg.Include
	if len(include) == 0 {
		include = []string{"**"}
	}
	for _, field := range []struct {
		name     string
		patterns []string
	}{{"include", include}, {"exclude", cfg.Exclude}, {"preserve", cfg.Preserve}} {
		if err := validatePatterns(mirrorID, field.name, field.patterns); err != nil {
			return Plan{}, err
		}
	}
	exclude := append([]string{}, cfg.Exclude...)
	exclude = append(exclude, alwaysProtected...)

	exportFiles, err := selectedFiles(source, include, exclude)
	if err != nil {
		return Plan{}, err
	}
	// Overlay targets are exported even when the source helper file is excluded.
	for _, overlay := range cfg.Overlays {
		if overlay.From == "" || overlay.To == "" {
			return Plan{}, fmt.Errorf("mirror %s overlay requires from and to", mirrorID)
		}
		fromAbs := filepath.Join(source, filepath.FromSlash(overlay.From))
		if info, err := os.Stat(fromAbs); err != nil || info.IsDir() {
			if err == nil {
				err = fmt.Errorf("is a directory")
			}
			return Plan{}, fmt.Errorf("mirror %s overlay source %s: %w", mirrorID, overlay.From, err)
		}
		exportFiles[slashPath(overlay.To)] = fromAbs
	}

	preserve := append([]string{}, cfg.Preserve...)
	preserve = append(preserve, alwaysProtected...)
	destFiles, err := destinationFiles(destination, preserve)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{MirrorID: mirrorID, WorkspaceRoot: root, Source: source, Destination: destination, Include: include, Exclude: exclude, Preserve: preserve, Checks: cfg.Checks}
	for _, overlay := range cfg.Overlays {
		plan.Overlays = append(plan.Overlays, OverlayOp{From: slashPath(overlay.From), To: slashPath(overlay.To)})
	}
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

func selectedFiles(root string, include, exclude []string) (map[string]string, error) {
	out := map[string]string{}
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
			out[rel] = path
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func destinationFiles(root string, preserve []string) (map[string]string, error) {
	out := map[string]string{}
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
			out[rel] = path
		}
		return nil
	}); err != nil {
		return nil, err
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
	rel = strings.Trim(slashPath(rel), "/")
	return rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".jj" || strings.HasPrefix(rel, ".jj/")
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
