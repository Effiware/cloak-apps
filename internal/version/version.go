// Package version provides build-time version information.
// These variables are populated via ldflags during the build process.
package version

// Version is the semantic version of the application.
// Set at build time via: -X github.com/effiware/cloak-apps/internal/version.Version=x.y.z
var Version = "0.0.0"

// BuildHash is the git commit hash of the build.
// Set at build time via: -X github.com/effiware/cloak-apps/internal/version.BuildHash=<commit-sha>
var BuildHash = "unknown"
