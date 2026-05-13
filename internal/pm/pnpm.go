package pm

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const pnpmLatestAPI = "https://api.github.com/repos/pnpm/pnpm/releases/latest"

type ghLatestRelease struct {
	TagName string `json:"tag_name"`
}

func resolveLatestPnpm() (string, error) {
	req, _ := http.NewRequest("GET", pnpmLatestAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve latest pnpm: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve latest pnpm: HTTP %d", resp.StatusCode)
	}
	var r ghLatestRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return strings.TrimPrefix(r.TagName, "v"), nil
}

func installPnpm(nodeDir, version string) error {
	if version == "" {
		v, err := resolveLatestPnpm()
		if err != nil {
			return err
		}
		version = v
	}
	version = strings.TrimPrefix(version, "v")

	url := fmt.Sprintf("https://github.com/pnpm/pnpm/releases/download/v%s/pnpm-win32-x64.zip", version)
	fmt.Printf("Downloading %s\n", url)

	tmp, err := os.CreateTemp("", "gonv-pnpm-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := downloadInto(url, tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := extractPnpmExe(tmpPath, filepath.Join(nodeDir, "pnpm.exe")); err != nil {
		return fmt.Errorf("extract pnpm: %w", err)
	}

	// pnpx wrapper — pnpm dropped the standalone pnpx binary; the
	// idiomatic replacement is `pnpm dlx <pkg>`.
	pnpxCmd := "@echo off\r\n" +
		"\"%~dp0pnpm.exe\" dlx %*\r\n"
	return os.WriteFile(filepath.Join(nodeDir, "pnpx.cmd"), []byte(pnpxCmd), 0o644)
}

func downloadInto(url string, w io.Writer) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// extractPnpmExe pulls pnpm.exe out of the release zip regardless of
// where it sits in the archive layout.
func extractPnpmExe(zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Base(f.Name), "pnpm.exe") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("pnpm.exe not found inside %s", zipPath)
}
