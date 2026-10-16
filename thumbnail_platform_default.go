//go:build !darwin && !windows

package main

import "errors"

func generatePlatformThumbnail(filePath string, size int, cacheDir string) ([]byte, string, error) {
	return nil, "", errors.New("platform thumbnail generation is unavailable")
}
