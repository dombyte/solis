// Package buildinfo holds the build information of the binary (set by cmd from its
// -ldflags variables). The binary is the single source of the version: the frontend and
// the API docs read it from GET /api/version.
package buildinfo

import "fmt"

// Info is the build information of one binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// String renders the build information for the startup log and `solis version`.
func (i Info) String() string {
	return fmt.Sprintf("solis %s (commit %s, built %s, %s)", i.Version, i.Commit, i.BuildDate,
		i.GoVersion)
}
