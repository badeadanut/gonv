package node

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const remoteIndexURL = "https://nodejs.org/dist/index.json"

type RemoteRelease struct {
	Version string
	Date    string
	LTS     string // empty if not an LTS release; otherwise the codename ("Iron", "Hydrogen", ...)
}

// rawRelease mirrors the upstream JSON. `lts` is `false` for non-LTS
// releases and a string codename otherwise, so we decode it loosely.
type rawRelease struct {
	Version string          `json:"version"`
	Date    string          `json:"date"`
	LTS     json.RawMessage `json:"lts"`
}

// FetchRemoteReleases returns every published Node.js release, newest first
// (the order nodejs.org serves them in).
func FetchRemoteReleases() ([]RemoteRelease, error) {
	resp, err := http.Get(remoteIndexURL)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", remoteIndexURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", remoteIndexURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw []rawRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	out := make([]RemoteRelease, 0, len(raw))
	for _, r := range raw {
		rr := RemoteRelease{Version: r.Version, Date: r.Date}
		if len(r.LTS) > 0 && r.LTS[0] == '"' {
			var s string
			if err := json.Unmarshal(r.LTS, &s); err == nil {
				rr.LTS = s
			}
		}
		out = append(out, rr)
	}
	return out, nil
}
