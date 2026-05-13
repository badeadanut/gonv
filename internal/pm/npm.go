package pm

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// fetchNpmVersions returns every published version of an npm package.
// It requests the abbreviated registry document so the response stays
// small even for packages with hundreds of releases.
func fetchNpmVersions(pkg string) ([]string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", pkg)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch npm %s: %w", pkg, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch npm %s: HTTP %d", pkg, resp.StatusCode)
	}
	var doc struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode npm %s: %w", pkg, err)
	}
	out := make([]string, 0, len(doc.Versions))
	for v := range doc.Versions {
		out = append(out, v)
	}
	return out, nil
}
