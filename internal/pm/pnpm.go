package pm

import (
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

	url := fmt.Sprintf("https://github.com/pnpm/pnpm/releases/download/v%s/pnpm-win-x64.exe", version)
	fmt.Printf("Downloading %s\n", url)
	if err := downloadFile(url, filepath.Join(nodeDir, "pnpm.exe"), 0o755); err != nil {
		return err
	}

	// pnpx wrapper — pnpm dropped the standalone pnpx binary; the
	// idiomatic replacement is `pnpm dlx <pkg>`.
	pnpxCmd := "@echo off\r\n" +
		"\"%~dp0pnpm.exe\" dlx %*\r\n"
	return os.WriteFile(filepath.Join(nodeDir, "pnpx.cmd"), []byte(pnpxCmd), 0o644)
}

func downloadFile(url, dst string, mode os.FileMode) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
