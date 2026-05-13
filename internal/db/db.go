package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"gonv/internal/config"
)

const schema = `
CREATE TABLE IF NOT EXISTS installed_node (
    version      TEXT PRIMARY KEY,
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS directory_versions (
    path         TEXT PRIMARY KEY,
    node_version TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS enabled_pm (
    node_version TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (node_version, name)
);
`

func Open() (*sql.DB, error) {
	if _, err := config.EnsureRoot(); err != nil {
		return nil, err
	}
	p, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return db, nil
}

func MarkNodeInstalled(db *sql.DB, version string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO installed_node (version) VALUES (?)`, version)
	return err
}

func IsNodeInstalled(db *sql.DB, version string) (bool, error) {
	var v string
	err := db.QueryRow(`SELECT version FROM installed_node WHERE version = ?`, version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func ListInstalledNode(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT version FROM installed_node ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func SetDirectoryVersion(db *sql.DB, path, version string) error {
	_, err := db.Exec(`
        INSERT INTO directory_versions (path, node_version) VALUES (?, ?)
        ON CONFLICT(path) DO UPDATE SET node_version = excluded.node_version
    `, path, version)
	return err
}

// ResolveDirectoryVersion picks the longest-prefix directory entry that
// covers `path`. Empty string means no mapping was found.
func ResolveDirectoryVersion(db *sql.DB, path string) (string, error) {
	rows, err := db.Query(`SELECT path, node_version FROM directory_versions`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	best := ""
	bestLen := -1
	for rows.Next() {
		var p, v string
		if err := rows.Scan(&p, &v); err != nil {
			return "", err
		}
		if isPrefixDir(path, p) && len(p) > bestLen {
			best = v
			bestLen = len(p)
		}
	}
	return best, rows.Err()
}

func EnablePM(db *sql.DB, nodeVersion, name, version string) error {
	_, err := db.Exec(`
        INSERT INTO enabled_pm (node_version, name, version) VALUES (?, ?, ?)
        ON CONFLICT(node_version, name) DO UPDATE SET version = excluded.version
    `, nodeVersion, name, version)
	return err
}

func isPrefixDir(child, parent string) bool {
	if len(child) < len(parent) {
		return false
	}
	if !strings.EqualFold(child[:len(parent)], parent) {
		return false
	}
	if len(child) == len(parent) {
		return true
	}
	c := child[len(parent)]
	return c == '\\' || c == '/'
}
