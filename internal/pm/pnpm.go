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

	"gonv/internal/semver"
)

const (
	pnpmLatestAPI  = "https://api.github.com/repos/pnpm/pnpm/releases/latest"
	pnpmReleaseAPI = "https://api.github.com/repos/pnpm/pnpm/releases/tags/v%s"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// fetchPnpmRelease retrieves the GitHub release metadata. An empty version
// requests the latest release.
func fetchPnpmRelease(version string) (*ghRelease, error) {
	url := pnpmLatestAPI
	if version != "" {
		url = fmt.Sprintf(pnpmReleaseAPI, strings.TrimPrefix(version, "v"))
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query pnpm release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query pnpm release %s: HTTP %d", url, resp.StatusCode)
	}
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// resolvePnpmVersion turns a partial query like "8" or "8.15" into an
// exact pnpm version by consulting the npm registry. Empty input means
// "use latest" and is passed through unchanged (the GitHub releases API
// handles that case).
func resolvePnpmVersion(query string) (string, error) {
	if query == "" {
		return "", nil
	}
	q, err := semver.ParseQuery(query)
	if err != nil {
		return "", err
	}
	if q.IsExact() {
		base := fmt.Sprintf("%d.%d.%d", q.Major, q.Minor, q.Patch)
		if q.Pre != "" {
			base += "-" + q.Pre
		}
		return base, nil
	}
	versions, err := fetchNpmVersions("pnpm")
	if err != nil {
		return "", err
	}
	return semver.ResolveLatest(versions, query)
}

func installPnpm(nodeDir, version string) error {
	resolved, err := resolvePnpmVersion(version)
	if err != nil {
		return err
	}
	if resolved != "" && resolved != version {
		fmt.Printf("Resolved pnpm %s → %s\n", version, resolved)
	}
	rel, err := fetchPnpmRelease(resolved)
	if err != nil {
		return err
	}

	fmt.Printf("pnpm %s release assets:\n", rel.TagName)
	for _, a := range rel.Assets {
		fmt.Printf("  %s\n", a.Name)
	}

	asset, err := pickWinX64Asset(rel.Assets)
	if err != nil {
		return err
	}
	fmt.Printf("Selected: %s\n", asset.Name)
	fmt.Printf("Downloading %s\n", asset.BrowserDownloadURL)

	lower := strings.ToLower(asset.Name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		tmp, err := os.CreateTemp("", "gonv-pnpm-*.zip")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if err := downloadInto(asset.BrowserDownloadURL, tmp); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := extractPnpmArchive(tmpPath, nodeDir); err != nil {
			return fmt.Errorf("extract pnpm: %w", err)
		}
	default:
		// .exe or extensionless bare executable
		out, err := os.OpenFile(filepath.Join(nodeDir, "pnpm.exe"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if err := downloadInto(asset.BrowserDownloadURL, out); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}

	if _, err := os.Stat(filepath.Join(nodeDir, "pnpm.exe")); err != nil {
		return fmt.Errorf("pnpm.exe missing under %s after install — release layout may have changed", nodeDir)
	}

	// pnpx wrapper — pnpm dropped the standalone pnpx binary; the
	// idiomatic replacement is `pnpm dlx <pkg>`.
	pnpxCmd := "@echo off\r\n" +
		"\"%~dp0pnpm.exe\" dlx %*\r\n"
	return os.WriteFile(filepath.Join(nodeDir, "pnpx.cmd"), []byte(pnpxCmd), 0o644)
}

// pickWinX64Asset picks the Windows x64 asset from a release. It prefers
// .zip (newer releases ship a `dist/` directory with pnpm.exe and
// supporting files) over standalone .exe binaries.
func pickWinX64Asset(assets []ghAsset) (*ghAsset, error) {
	var zipAsset, exeAsset *ghAsset
	for i := range assets {
		a := &assets[i]
		lower := strings.ToLower(a.Name)
		if !strings.Contains(lower, "x64") || strings.Contains(lower, "arm") {
			continue
		}
		// Match "win-x64" or "win32-x64" but not e.g. "win-arm64"
		if !strings.Contains(lower, "win32") && !strings.Contains(lower, "win-") {
			continue
		}
		switch {
		case strings.HasSuffix(lower, ".zip"):
			zipAsset = a
		case strings.HasSuffix(lower, ".exe"):
			exeAsset = a
		default:
			if !strings.ContainsRune(filepath.Base(lower), '.') {
				exeAsset = a
			}
		}
	}
	if zipAsset != nil {
		return zipAsset, nil
	}
	if exeAsset != nil {
		return exeAsset, nil
	}
	return nil, fmt.Errorf("no Windows x64 pnpm asset found in release")
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

// extractPnpmArchive extracts a pnpm release zip into dst. It strips a
// single common top-level directory (e.g. "dist/" or "pnpm-win32-x64/")
// when every entry sits underneath it, so pnpm.exe lands directly at
// dst/pnpm.exe.
func extractPnpmArchive(zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, filepath.ToSlash(f.Name))
	}
	prefix := commonTopLevel(names)

	for _, f := range r.File {
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), prefix)
		if name == "" {
			continue
		}
		out := filepath.Join(dst, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := writeZipFile(f, out); err != nil {
			return err
		}
	}
	return nil
}

// commonTopLevel returns the shared first-segment path (with trailing /)
// when every entry sits under the same root directory; otherwise "".
func commonTopLevel(names []string) string {
	if len(names) == 0 {
		return ""
	}
	i := strings.Index(names[0], "/")
	if i < 0 {
		return ""
	}
	prefix := names[0][:i+1]
	for _, n := range names[1:] {
		if !strings.HasPrefix(n, prefix) {
			return ""
		}
	}
	return prefix
}

func writeZipFile(f *zip.File, out string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = io.Copy(w, rc)
	return err
}
