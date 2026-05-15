// Package version holds the build-time version string for binaries in
// this repo. Default is "dev" so unbuilt / go-run invocations report a
// stable sentinel; release builds override via -ldflags:
//
//	go build -ldflags "-X github.com/CarriedWorldUniverse/acp-claude-pty/internal/version.Version=v0.1.0" ./cmd/acp-claude-pty
//
// CI invokes `git describe --tags --always --dirty` for the value so
// dev builds report e.g. v0.1.0-3-gabc1234, release builds report the
// clean tag (v0.1.0).
package version

// Version is the build-time version string. Overridden via -ldflags at
// build time; "dev" when unset.
var Version = "dev"
