package mirror

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		dst, err := destinationPath(plan.Destination, op.Path)
		if err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return removeEmptyDirs(plan.Destination)
}

// destinationPath joins rel onto destination after checking that every
// existing path component below destination is a real directory and that an
// existing leaf is a regular file, so writes and deletes never follow a
// symlink out of the mirror.
func destinationPath(destination, rel string) (string, error) {
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("refusing to write %q: path is outside the mirror", rel)
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(local)), "/")
	current := destination
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to write %s in mirror: %s is a symlink", rel, current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("refusing to write %s in mirror: %s is not a directory", rel, current)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", fmt.Errorf("refusing to write %s in mirror: %s is not a regular file", rel, current)
		}
	}
	return filepath.Join(destination, local), nil
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
	dst, err := destinationPath(plan.Destination, rel)
	if err != nil {
		return err
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("copy %s: not a regular file", src)
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
