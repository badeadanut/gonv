package node

import (
	"database/sql"
	"errors"
	"path/filepath"

	gonvdb "gonv/internal/db"
)

var ErrNoVersion = errors.New("no node version is configured for this directory (run `gonv use <version>`)")

// ResolveForCWD returns the Node version configured for the given dir
// (or any of its ancestors).
func ResolveForCWD(db *sql.DB, cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	v, err := gonvdb.ResolveDirectoryVersion(db, abs)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", ErrNoVersion
	}
	return v, nil
}
