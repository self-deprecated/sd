package mirror

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/self-deprecated/sd/internal/config"
)

type ResolveOptions struct {
	WorkspaceRoot    string
	CLIPathOverrides map[string]string
	Env              map[string]string
	LocalPaths       map[string]string
}

type MissingPathVariableError struct {
	MirrorID string
	Variable string
}

func (e MissingPathVariableError) Error() string {
	return fmt.Sprintf("mirror %s destination root %q is not configured", e.MirrorID, e.Variable)
}

func EnvNameForPathVariable(name string) string {
	var b strings.Builder
	b.WriteString("SD_")
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	re := regexp.MustCompile(`_+`)
	return re.ReplaceAllString(b.String(), "_")
}

func ResolveDestination(mirrorID string, cfg config.MirrorConfig, opts ResolveOptions) (string, error) {
	rootName := cfg.Destination.Root
	if rootName == "" {
		return "", fmt.Errorf("mirror %s destination.root is required", mirrorID)
	}
	root := ""
	if opts.CLIPathOverrides != nil {
		root = opts.CLIPathOverrides[rootName]
	}
	if root == "" {
		if opts.Env != nil {
			root = opts.Env[EnvNameForPathVariable(rootName)]
		} else {
			root = os.Getenv(EnvNameForPathVariable(rootName))
		}
	}
	if root == "" && opts.LocalPaths != nil {
		root = opts.LocalPaths[rootName]
	}
	if root == "" {
		return "", MissingPathVariableError{MirrorID: mirrorID, Variable: rootName}
	}
	root = expandHome(root)
	path := cfg.Destination.Path
	if path == "" {
		path = mirrorID
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Clean(filepath.Join(root, path)), nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
