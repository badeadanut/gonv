package node

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gonv/internal/config"
)

func downloadURL(version string) string {
	return fmt.Sprintf("https://nodejs.org/dist/%s/node-%s-win-x64.zip", version, version)
}

// Install downloads and extracts the requested Node version into the gonv
// root. It is idempotent: if `node.exe` already exists at the target path,
// it returns immediately.
func Install(version string) (string, error) {
	version = config.NormalizeVersion(version)
	target, err := config.NodeVersionDir(version)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(target, "node.exe")); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}

	url := downloadURL(version)
	fmt.Printf("Downloading %s\n", url)

	tmp, err := os.CreateTemp("", "gonv-node-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(url)
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return "", fmt.Errorf("download: HTTP %d (is %s a real release?)", resp.StatusCode, version)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	if err := extractZip(tmpPath, target); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	return target, nil
}

// extractZip flattens the single top-level directory the official Node
// archive ships with (e.g. "node-v20.10.0-win-x64/...").
func extractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		outPath := filepath.Join(dest, filepath.FromSlash(parts[1]))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(f, outPath); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, outPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
