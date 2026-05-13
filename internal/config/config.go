package config

import (
	"fmt"
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

// InstallDir returns the directory where the install identified by name
// lives (node.exe and any enabled package managers sit here).
func InstallDir(name string) (string, error) {
	if err := ValidateInstallName(name); err != nil {
		return "", err
	}
	r, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, VersionsDir, NodeSubdir, name), nil
}

func ShimsPath() (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, ShimsDir), nil
}

// NormalizeVersion ensures a Node version string carries the leading 'v'.
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

// ValidateInstallName rejects names that would escape the installs root
// when joined as a path component.
func ValidateInstallName(n string) error {
	if n == "" {
		return fmt.Errorf("install name cannot be empty")
	}
	if strings.ContainsAny(n, `\/`) {
		return fmt.Errorf("install name %q must not contain path separators", n)
	}
	if n == "." || n == ".." {
		return fmt.Errorf("install name %q is reserved", n)
	}
	return nil
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
