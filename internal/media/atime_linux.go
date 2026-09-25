package media

import (
	"os"
	"syscall"
)

func atime(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Atim.Sec
	}
	return info.ModTime().Unix()
}
