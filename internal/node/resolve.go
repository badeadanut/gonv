package node

import (
	"database/sql"
	"errors"
	"path/filepath"

	gonvdb "gonv/internal/db"
)

var ErrNoInstall = errors.New("no install is configured for this directory (run `gonv use <name>`)")

// ResolveForCWD returns the install name configured for the given dir or
// any of its ancestors.
func ResolveForCWD(db *sql.DB, cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	name, err := gonvdb.ResolveDirectoryInstall(db, abs)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", ErrNoInstall
	}
	return name, nil
}
