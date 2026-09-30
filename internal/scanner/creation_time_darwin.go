//go:build darwin

package scanner

import (
	"os"
	"syscall"
	"time"
)

func creationTime(info os.FileInfo) (time.Time, int64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok {
		createdAt := time.Unix(int64(stat.Birthtimespec.Sec), int64(stat.Birthtimespec.Nsec))
		return createdAt, createdAt.UnixMilli()
	}

	modifiedAt := info.ModTime()
	return modifiedAt, modifiedAt.UnixMilli()
}
