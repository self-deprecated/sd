package mirror

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func (p Plan) HasChanges() bool {
	return len(p.Creates) > 0 || len(p.Updates) > 0 || len(p.Deletes) > 0
}

func ApplyPlan(plan Plan) error {
	if err := os.MkdirAll(plan.Destination, 0o755); err != nil {
		return err
	}
	for _, op := range plan.Creates {
		if err := copyPlanFile(plan, op.Path); err != nil {
			return err
		}
	}
	for _, op := range plan.Updates {
		if err := copyPlanFile(plan, op.Path); err != nil {
			return err
		}
	}
	// Delete stale files after copying. Directories are cleaned up afterwards.
	for _, op := range plan.Deletes {
		if isProtected(op.Path) {
			continue
		}
		if err := os.Remove(filepath.Join(plan.Destination, filepath.FromSlash(op.Path))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return removeEmptyDirs(plan.Destination)
}

func copyPlanFile(plan Plan, rel string) error {
	var src string
	for _, overlay := range plan.Overlays {
		if overlay.To == rel {
			src = filepath.Join(plan.Source, filepath.FromSlash(overlay.From))
			break
		}
	}
	if src == "" {
		src = filepath.Join(plan.Source, filepath.FromSlash(rel))
	}
	dst := filepath.Join(plan.Destination, filepath.FromSlash(rel))
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("copy %s: is a directory", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(dst, info.Mode().Perm())
}

func removeEmptyDirs(root string) error {
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if isProtected(rel) {
				return filepath.SkipDir
			}
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
	return nil
}
