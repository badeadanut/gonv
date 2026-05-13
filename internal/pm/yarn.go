package pm

import (
	"archive/tar"
	"compress/gzip"
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
	yarnLatestURL  = "https://registry.npmjs.org/yarn/latest"
	yarnTarballFmt = "https://registry.npmjs.org/yarn/-/yarn-%s.tgz"
)

type npmDistTag struct {
	Version string `json:"version"`
}

func resolveLatestYarn() (string, error) {
	resp, err := http.Get(yarnLatestURL)
	if err != nil {
		return "", fmt.Errorf("resolve latest yarn: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve latest yarn: HTTP %d", resp.StatusCode)
	}
	var e npmDistTag
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return "", err
	}
	return e.Version, nil
}

// resolveYarnVersion turns a partial query like "1" or "1.22" into an
// exact yarn version. Empty input means "use latest".
func resolveYarnVersion(query string) (string, error) {
	if query == "" {
		return resolveLatestYarn()
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
	versions, err := fetchNpmVersions("yarn")
	if err != nil {
		return "", err
	}
	return semver.ResolveLatest(versions, query)
}

// installYarn pulls the yarn tarball from the npm registry, extracts it
// alongside node.exe, and writes a yarn.cmd wrapper that invokes the
// bundled yarn.js with the Node binary from the same directory.
func installYarn(nodeDir, version string) error {
	resolved, err := resolveYarnVersion(version)
	if err != nil {
		return err
	}
	if resolved != version {
		fmt.Printf("Resolved yarn %s → %s\n", version, resolved)
	}
	version = strings.TrimPrefix(resolved, "v")

	url := fmt.Sprintf(yarnTarballFmt, version)
	fmt.Printf("Downloading %s\n", url)

	bundleDir := filepath.Join(nodeDir, "yarn-pkg")
	if err := os.RemoveAll(bundleDir); err != nil {
		return err
	}
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return err
	}
	if err := downloadAndExtractTgz(url, bundleDir); err != nil {
		return err
	}

	cmdContent := "@echo off\r\n" +
		"\"%~dp0node.exe\" \"%~dp0yarn-pkg\\bin\\yarn.js\" %*\r\n"
	return os.WriteFile(filepath.Join(nodeDir, "yarn.cmd"), []byte(cmdContent), 0o644)
}

// downloadAndExtractTgz pulls a gzipped tarball from URL and extracts it
// into dest, stripping the leading "package/" path component used by npm
// tarballs.
func downloadAndExtractTgz(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := hdr.Name
		i := strings.Index(name, "/")
		if i < 0 {
			continue
		}
		name = name[i+1:]
		if name == "" {
			continue
		}
		out := filepath.Join(dest, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			if err := writeTarEntry(tr, out, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		}
	}
}

func writeTarEntry(tr *tar.Reader, out string, mode os.FileMode) error {
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, tr)
	return err
}
