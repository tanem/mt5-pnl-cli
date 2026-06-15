package main

import (
	"runtime/debug"
	"strings"
)

// version is injected by GoReleaser via -ldflags "-X main.version=...".
// It stays "dev" for `go install` and local builds, where resolveVersion
// recovers the real version from the embedded build info instead.
var version = "dev"

// resolveVersion reports the version to display, preferring the GoReleaser
// ldflag and otherwise falling back to the module version Go embeds in the
// build info (so `go install ...@v1.0.0` reports 1.0.0 rather than dev).
func resolveVersion() string {
	return versionFrom(version, debug.ReadBuildInfo)
}

// versionFrom resolves the display version from the ldflag value and a build
// info reader. The ldflag wins when set; otherwise a concrete module version
// from the build info is used (stripped of its leading "v" for parity with
// GoReleaser output). Local builds report "(devel)" or "", which keep "dev".
func versionFrom(ldflagVersion string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ldflagVersion != "dev" {
		return ldflagVersion
	}
	if info, ok := readBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return ldflagVersion
}
