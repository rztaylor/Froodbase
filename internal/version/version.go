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
