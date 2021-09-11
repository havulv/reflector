package version

import (
	"errors"
	"fmt"
)

var (
	// CommitHash is the commit hash of this version
	CommitHash string
	// CommitDate is the date at which the last commit was made
	CommitDate string
	// SemVer is the semantic version of the reflector
	SemVer string

	// OutputFunc is a variable for the printer that will dump
	// the version out (to avoid cluttering test logs).
	OutputFunc = fmt.Printf
)

// DumpVersion dumps the version information set at compile time
// by the linker.
func DumpVersion() error {
	if SemVer == "" && CommitHash == "" && CommitDate == "" {
		return errors.New(
			"version information not linked at compile time")
	}
	_, err := OutputFunc(`
Version: %s
Commit: %s
Date: %s
`, SemVer, CommitHash, CommitDate)
	if err != nil {
		// TODO: do something?
	}
	return nil
}
