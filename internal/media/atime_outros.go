//go:build !darwin && !linux

package media

import "os"

func atime(info os.FileInfo) int64 {
	return info.ModTime().Unix()
}
