package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"gonv/internal/config"
)

type Install struct {
	Name        string
	NodeVersion string
}

const baseSchema = `
CREATE TABLE IF NOT EXISTS installs (
    name         TEXT PRIMARY KEY,
    node_version TEXT NOT NULL,
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS directory_versions (
    path         TEXT PRIMARY KEY,
    install_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS enabled_pm (
    install_name TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (install_name, name)
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
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrate brings the database forward from the pre-alias schema
// (installed_node + node_version columns) to the current one.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(baseSchema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	hasOld, err := tableExists(db, "installed_node")
	if err != nil {
		return err
	}
	if hasOld {
		if _, err := db.Exec(`
            INSERT OR IGNORE INTO installs (name, node_version, installed_at)
            SELECT version, version, installed_at FROM installed_node
        `); err != nil {
			return fmt.Errorf("migrate installed_node: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE installed_node`); err != nil {
			return err
		}
	}
	if err := renameColumnIfPresent(db, "directory_versions", "node_version", "install_name"); err != nil {
		return err
	}
	if err := renameColumnIfPresent(db, "enabled_pm", "node_version", "install_name"); err != nil {
		return err
	}
	return nil
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func renameColumnIfPresent(db *sql.DB, table, oldName, newName string) error {
	has, err := columnExists(db, table, oldName)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	stmt := fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, table, oldName, newName)
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("rename %s.%s: %w", table, oldName, err)
	}
	return nil
}

func columnExists(db *sql.DB, table, col string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, col) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// RegisterInstall records an install. Errors if name is already taken.
func RegisterInstall(db *sql.DB, name, nodeVersion string) error {
	_, err := db.Exec(`INSERT INTO installs (name, node_version) VALUES (?, ?)`, name, nodeVersion)
	return err
}

func IsInstallRegistered(db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM installs WHERE name = ?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetInstall returns (nodeVersion, found, error).
func GetInstall(db *sql.DB, name string) (string, bool, error) {
	var nv string
	err := db.QueryRow(`SELECT node_version FROM installs WHERE name = ?`, name).Scan(&nv)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return nv, true, nil
}

func ListInstalls(db *sql.DB) ([]Install, error) {
	rows, err := db.Query(`SELECT name, node_version FROM installs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Install
	for rows.Next() {
		var i Install
		if err := rows.Scan(&i.Name, &i.NodeVersion); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DeleteInstall removes the install record and any dependent rows.
func DeleteInstall(db *sql.DB, name string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM enabled_pm WHERE install_name = ?`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM directory_versions WHERE install_name = ?`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM installs WHERE name = ?`, name); err != nil {
		return err
	}
	return tx.Commit()
}

func SetDirectoryInstall(db *sql.DB, path, installName string) error {
	_, err := db.Exec(`
        INSERT INTO directory_versions (path, install_name) VALUES (?, ?)
        ON CONFLICT(path) DO UPDATE SET install_name = excluded.install_name
    `, path, installName)
	return err
}

// ResolveDirectoryInstall picks the longest-prefix directory entry that
// covers path. Empty string means no mapping was found.
func ResolveDirectoryInstall(db *sql.DB, path string) (string, error) {
	rows, err := db.Query(`SELECT path, install_name FROM directory_versions`)
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

func EnablePM(db *sql.DB, installName, name, version string) error {
	_, err := db.Exec(`
        INSERT INTO enabled_pm (install_name, name, version) VALUES (?, ?, ?)
        ON CONFLICT(install_name, name) DO UPDATE SET version = excluded.version
    `, installName, name, version)
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
