// Package version provides build-time version information and the single
// identity string used by every OTel tracer/meter in the module.
// Version and BuildHash are populated via ldflags during the build process.
package version

// ServiceName names the instrumentation scope of every tracer and meter in this
// module. The runtime service.name comes from OTEL_SERVICE_NAME and may differ.
const ServiceName = "cloak-apps"

// Version is the semantic version of the application.
// Set at build time via: -X github.com/effiware/cloak-apps/internal/version.Version=x.y.z
var Version = "0.0.0"

// BuildHash is the git commit hash of the build.
// Set at build time via: -X github.com/effiware/cloak-apps/internal/version.BuildHash=<commit-sha>
var BuildHash = "unknown"
