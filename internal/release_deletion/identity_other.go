//go:build !linux && !darwin && !windows

package release_deletion

import "os"

// Unknown platforms fail closed rather than deleting unverifiable library files.
func fileIdentity(_ string, _ os.FileInfo) string { return "" }
