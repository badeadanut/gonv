// Package semver provides minimal semver parsing and partial-version
// matching, enough for "complete v22 to the latest v22.x.x" lookups
// across Node, pnpm, and yarn.
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Pre                 string // text after '-' (prerelease)
}

// Parse decodes a full M.m.p version string (with or without leading 'v').
func Parse(s string) (Version, error) {
	s = stripV(strings.TrimSpace(s))
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("not a full version: %q", s)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid segment %q in %q", p, s)
		}
		n[i] = v
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2], Pre: pre}, nil
}

// Compare returns >0 if a > b, <0 if a < b, 0 if equal.
func Compare(a, b Version) int {
	if a.Major != b.Major {
		return a.Major - b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor - b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch - b.Patch
	}
	if a.Pre == "" && b.Pre != "" {
		return 1
	}
	if a.Pre != "" && b.Pre == "" {
		return -1
	}
	return strings.Compare(a.Pre, b.Pre)
}

// Query is a partial version specifier: 1, 2, or 3 numeric segments,
// optionally followed by a prerelease tag.
type Query struct {
	Major, Minor, Patch int
	Segments            int // 1, 2, or 3
	Pre                 string
}

func ParseQuery(s string) (Query, error) {
	s = stripV(strings.TrimSpace(s))
	if s == "" {
		return Query{}, fmt.Errorf("empty version query")
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Query{}, fmt.Errorf("invalid version query %q", s)
	}
	q := Query{Segments: len(parts), Pre: pre}
	nums := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return Query{}, fmt.Errorf("invalid segment %q in %q", p, s)
		}
		nums[i] = v
	}
	q.Major = nums[0]
	if len(nums) > 1 {
		q.Minor = nums[1]
	}
	if len(nums) > 2 {
		q.Patch = nums[2]
	}
	return q, nil
}

func (q Query) IsExact() bool { return q.Segments == 3 }

func (q Query) Matches(v Version) bool {
	if v.Major != q.Major {
		return false
	}
	if q.Segments >= 2 && v.Minor != q.Minor {
		return false
	}
	if q.Segments >= 3 && v.Patch != q.Patch {
		return false
	}
	if q.Pre != "" {
		return v.Pre == q.Pre
	}
	return true
}

// ResolveLatest returns the highest version from `available` that matches
// the query. Prereleases are excluded unless the query explicitly asks
// for one. The returned string is the value as it appeared in `available`
// (any leading 'v' is preserved).
func ResolveLatest(available []string, query string) (string, error) {
	q, err := ParseQuery(query)
	if err != nil {
		return "", err
	}
	var best Version
	bestStr := ""
	for _, raw := range available {
		v, err := Parse(raw)
		if err != nil {
			continue
		}
		if !q.Matches(v) {
			continue
		}
		if v.Pre != "" && q.Pre == "" {
			continue
		}
		if bestStr == "" || Compare(v, best) > 0 {
			best = v
			bestStr = raw
		}
	}
	if bestStr == "" {
		return "", fmt.Errorf("no version matches %q", query)
	}
	return bestStr, nil
}

func stripV(s string) string {
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		return s[1:]
	}
	return s
}
