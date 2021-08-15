package version

import (
	"errors"
	"fmt"
)

var (
	commitHash string
	commitDate string
	semVer     string
)

// DumpVersion dumps the version information set at compile time
// by the linker.
func DumpVersion() error {
	if semVer == "" && commitHash == "" && commitDate == "" {
		return errors.New(
			"version information not linked at compile time")
	}
	fmt.Printf(`
Version: %s
Commit: %s
Date: %s
`, semVer, commitHash, commitDate)
	return nil
}
