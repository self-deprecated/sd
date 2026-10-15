package mirror

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/self-deprecated/sd/internal/config"
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

func sameFile(a, b string) (bool, error) {
	adata, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bdata, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(adata, bdata), nil
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

func matchesAny(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(rel, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(rel, pattern string) bool {
	rel = strings.Trim(slashPath(rel), "/")
	pattern = strings.Trim(slashPath(pattern), "/")
	if pattern == "" {
		return false
	}
	if pattern == "**" {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/") || strings.HasSuffix(rel, "/"+prefix) || strings.Contains(rel, "/"+prefix+"/")
	}
	if strings.HasPrefix(pattern, "**/") {
		tail := strings.TrimPrefix(pattern, "**/")
		if ok, _ := path.Match(tail, path.Base(rel)); ok {
			return true
		}
		return strings.HasSuffix(rel, "/"+tail)
	}
	if strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, rel)
		return ok
	}
	if ok, _ := path.Match(pattern, path.Base(rel)); ok {
		return true
	}
	ok, _ := path.Match(pattern, rel)
	return ok
}
