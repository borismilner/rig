package capture

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByUs reports whether the file belongs to the user running rigd.
func ownedByUs(st fs.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Getuid()
}
