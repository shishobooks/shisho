package version

// Version is the application version, set at build time via ldflags.
// Example: go build -ldflags "-X github.com/shishobooks/shisho/pkg/version.Version=1.0.0".
var Version = "dev"
