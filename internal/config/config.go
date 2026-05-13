package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DirName     = ".gonv"
	DBFileName  = "gonv.db"
	VersionsDir = "versions"
	NodeSubdir  = "node"
	ShimsDir    = "shims"
)

func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DirName), nil
}

func DBPath() (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, DBFileName), nil
}

func NodeVersionDir(version string) (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, VersionsDir, NodeSubdir, NormalizeVersion(version)), nil
}

func ShimsPath() (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, ShimsDir), nil
}

func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if v[0] != 'v' && v[0] != 'V' {
		return "v" + v
	}
	return "v" + v[1:]
}

func EnsureRoot() (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}
	dirs := []string{
		r,
		filepath.Join(r, VersionsDir, NodeSubdir),
		filepath.Join(r, ShimsDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	return r, nil
}
