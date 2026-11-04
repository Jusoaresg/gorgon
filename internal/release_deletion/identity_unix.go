//go:build linux || darwin

package release_deletion

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(_ string, info os.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d:%s", st.Dev, st.Ino, identityMetadata(info))
}
