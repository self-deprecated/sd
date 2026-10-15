package config

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/self-deprecated/sd/internal/model"
)

type WorkspaceFile struct {
	Version   int `yaml:"version"`
	Workspace struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
	} `yaml:"workspace"`
	Discover []string `yaml:"discover"`
}

type TaskFile struct {
	Version     int                           `yaml:"version"`
	Project     model.Project                 `yaml:"project"`
	Scripts     map[string]model.SharedScript `yaml:"scripts"`
	Tasks       map[string]model.Task         `yaml:"tasks"`
	Passthrough map[string]model.Task         `yaml:"passthrough"`
}

type AggregatesFile struct {
	Version int                   `yaml:"version"`
	Tasks   map[string]model.Task `yaml:"tasks"`
}

type MirrorsFile struct {
	Version int                     `yaml:"version"`
	Mirrors map[string]MirrorConfig `yaml:"mirrors"`
}

type MirrorConfig struct {
	Source      string            `yaml:"source" json:"source"`
	Destination MirrorDestination `yaml:"destination" json:"destination"`
	Bookmark    string            `yaml:"bookmark,omitempty" json:"bookmark,omitempty"`
	Remote      string            `yaml:"remote,omitempty" json:"remote,omitempty"`
	Visibility  string            `yaml:"visibility,omitempty" json:"visibility,omitempty"`
	Include     []string          `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude     []string          `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Preserve    []string          `yaml:"preserve,omitempty" json:"preserve,omitempty"`
	Overlays    []MirrorOverlay   `yaml:"overlays,omitempty" json:"overlays,omitempty"`
	Checks      []string          `yaml:"checks,omitempty" json:"checks,omitempty"`
	// Allow lists paths whose scanner findings were reviewed and accepted,
	// such as test fixtures with fake credentials.
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty"`
}

type MirrorDestination struct {
	Root string `yaml:"root" json:"root"`
	Path string `yaml:"path" json:"path"`
}

type MirrorOverlay struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

type MirrorLocalFile struct {
	Version int               `yaml:"version"`
	Paths   map[string]string `yaml:"paths"`
}

func ReadWorkspace(path string) (WorkspaceFile, error) {
	var out WorkspaceFile
	err := readYAML(path, &out, &out.Version)
	return out, err
}

func ReadTaskFile(path string) (TaskFile, error) {
	var out TaskFile
	err := readYAML(path, &out, &out.Version)
	return out, err
}

func ReadAggregates(path string) (AggregatesFile, error) {
	var out AggregatesFile
	err := readYAML(path, &out, &out.Version)
	return out, err
}

func ReadMirrors(path string) (MirrorsFile, error) {
	var out MirrorsFile
	err := readYAML(path, &out, nil)
	return out, err
}

func ReadMirrorLocal(path string) (MirrorLocalFile, error) {
	var out MirrorLocalFile
	err := readYAML(path, &out, nil)
	return out, err
}

// readYAML decodes path into out, rejecting unknown fields so typos such as
// `dangerous: true` fail loudly, and requires `version: 1` unless version is
// nil because the caller checks it.
func readYAML(path string, out any, version *int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w", path, err)
	}
	if version == nil {
		return nil
	}
	switch *version {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("%s: missing `version: 1`", path)
	default:
		return fmt.Errorf("%s: unsupported version %d (sd supports version 1)", path, *version)
	}
}
