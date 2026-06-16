// Package version exposes build metadata for the CLI.
//
// Values are wired two ways so the binary reports something useful no matter
// how it was built:
//
//   - GoReleaser sets Version/Commit/Date via -ldflags "-X ...". See
//     .goreleaser.yaml and §14 of GOAL.md.
//   - `go install pkg@vX` and `go build` inside a repo leave those ldflags
//     empty; Info() then falls back to runtime/debug.ReadBuildInfo() so the
//     module version and VCS stamp still surface.
package version

import (
	"runtime"
	"runtime/debug"
)

// These are overwritten at link time by GoReleaser. main wires the package-main
// ldflags vars (see GOAL.md §14) into Set before any command runs. Defaults make
// a plain `go run` honest about being a dev build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Set lets package main inject ldflags-provided values. main owns the
// `-X main.version=...` vars because -X only reaches package main; it forwards
// them here so the rest of the program reads version state from one place.
// Empty arguments are ignored so a partial ldflags set never blanks a field.
func Set(v, c, d string) {
	if v != "" {
		version = v
	}
	if c != "" {
		commit = c
	}
	if d != "" {
		date = d
	}
}

// Build holds the resolved build metadata. JSON tags match the `version --json`
// contract in GOAL.md §9.8: {"version","commit","date","go"}.
type Build struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
}

// Info resolves build metadata, preferring ldflags values and falling back to
// the module/VCS stamp embedded by the Go toolchain.
func Info() Build {
	b := Build{
		Version: version,
		Commit:  commit,
		Date:    date,
		Go:      runtime.Version(),
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}

	// `go install module@v1.2.3` records the version here; ldflags builds leave
	// it as "(devel)", which is no better than our "dev" default.
	if b.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		b.Version = bi.Main.Version
	}

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if b.Commit == "none" && s.Value != "" {
				b.Commit = s.Value
			}
		case "vcs.time":
			if b.Date == "unknown" && s.Value != "" {
				b.Date = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				b.Commit += "-dirty"
			}
		}
	}
	return b
}

// String is the short, human one-liner used by `--version`.
func (b Build) String() string {
	return b.Version + " (commit " + b.Commit + ", built " + b.Date + ", " + b.Go + ")"
}
