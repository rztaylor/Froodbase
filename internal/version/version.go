package version

import "fmt"

// Set at link time with -ldflags "-X github.com/rztaylor/froodbase/internal/version.Version=...".
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns the human-readable build identity.
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}

// MinClientVersion is the oldest froodbase-mcp release froodbase-api accepts.
// A client below it is told to update (TR-102). Development builds, whose
// Version is not a release tag, are always accepted.
const MinClientVersion = "v0.0.0"

// ClientHeader carries the client's Version on every request to the API.
const ClientHeader = "Froodbase-Client-Version"

// Compatible reports whether a client version satisfies MinClientVersion.
// Both are compared as vMAJOR.MINOR.PATCH; a client version that is not a
// release tag (for example "dev" or a commit hash) is treated as compatible.
func Compatible(client string) bool {
	c, ok := parseSemver(client)
	if !ok {
		return true
	}
	m, _ := parseSemver(MinClientVersion)
	for i := range 3 {
		if c[i] != m[i] {
			return c[i] > m[i]
		}
	}
	return true
}

// parseSemver reads vMAJOR.MINOR.PATCH, ignoring any pre-release or build
// suffix after '-' or '+'.
func parseSemver(s string) ([3]int, bool) {
	var out [3]int
	if len(s) < 2 || s[0] != 'v' {
		return out, false
	}
	s = s[1:]
	if i := indexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	part := 0
	n := -1
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if n < 0 {
				n = 0
			}
			n = n*10 + int(r-'0')
		case r == '.':
			if n < 0 || part == 2 {
				return out, false
			}
			out[part] = n
			part++
			n = -1
		default:
			return out, false
		}
	}
	if part != 2 || n < 0 {
		return out, false
	}
	out[2] = n
	return out, true
}

func indexAny(s, chars string) int {
	for i, r := range s {
		for _, c := range chars {
			if r == c {
				return i
			}
		}
	}
	return -1
}
