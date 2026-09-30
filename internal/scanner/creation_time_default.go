//go:build !darwin

package scanner

import (
	"os"
	"time"
)

func creationTime(info os.FileInfo) (time.Time, int64) {
	modifiedAt := info.ModTime()
	return modifiedAt, modifiedAt.UnixMilli()
}
