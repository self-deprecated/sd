package mirror

import (
	"bytes"
	"os/exec"
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
	IsJJWorkcopy(dir string) (bool, error)
	Status(dir string) (VCSStatus, error)
	Commit(dir string, message string) error
	CurrentChange(dir string) (TraceInfo, error)
}

type JJVCS struct{}

func (JJVCS) IsJJWorkcopy(dir string) (bool, error) {
	cmd := exec.Command("jj", "root")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
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
