package mirror

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type VCSStatus struct {
	Clean  bool
	Output string
}

type TraceInfo struct {
	ChangeID string
	CommitID string
}

type VCS interface {
	// IsJJWorkcopy reports whether dir is the root of a jj workspace.
	IsJJWorkcopy(dir string) (bool, error)
	Status(dir string) (VCSStatus, error)
	// ChangedFiles lists the paths, relative to dir, that the working-copy
	// change adds, modifies, or deletes.
	ChangedFiles(dir string) ([]string, error)
	Commit(dir string, message string) error
	CurrentChange(dir string) (TraceInfo, error)
}

type JJVCS struct{}

// IsJJWorkcopy reports whether dir is the root of a jj workspace, not merely
// a directory somewhere inside one.
func (JJVCS) IsJJWorkcopy(dir string) (bool, error) {
	cmd := exec.Command("jj", "root")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	rootInfo, err := os.Stat(strings.TrimSpace(string(out)))
	if err != nil {
		return false, nil
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return false, err
	}
	return os.SameFile(rootInfo, dirInfo), nil
}

func (JJVCS) Status(dir string) (VCSStatus, error) {
	cmd := exec.Command("jj", "st", "--no-pager")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return VCSStatus{Output: string(out)}, err
	}
	text := string(out)
	return VCSStatus{Clean: strings.Contains(text, "The working copy has no changes."), Output: text}, nil
}

func (JJVCS) ChangedFiles(dir string) ([]string, error) {
	cmd := exec.Command("jj", "diff", "--no-pager", "--name-only", "-r", "@")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list mirror changes: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, filepath.ToSlash(line))
		}
	}
	return paths, nil
}

func (JJVCS) Commit(dir string, message string) error {
	cmd := exec.Command("jj", "commit", "-m", message)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (JJVCS) CurrentChange(dir string) (TraceInfo, error) {
	cmd := exec.Command("jj", "log", "--no-pager", "-r", "@", "--template", `change_id.shortest() ++ " " ++ commit_id.shortest()`)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return TraceInfo{}, err
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return TraceInfo{}, nil
	}
	return TraceInfo{ChangeID: parts[0], CommitID: parts[1]}, nil
}
