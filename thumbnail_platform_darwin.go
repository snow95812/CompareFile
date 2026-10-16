//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func generatePlatformThumbnail(filePath string, size int, cacheDir string) ([]byte, string, error) {
	workingDir, err := os.MkdirTemp(cacheDir, "ql-thumb-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(workingDir)

	command := exec.Command("qlmanage", "-t", "-s", strconv.Itoa(size*2), "-o", workingDir, filePath)
	if err := command.Run(); err != nil {
		return nil, "", err
	}

	thumbnailPath, contentType, err := locateGeneratedThumbnail(workingDir)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return nil, "", err
	}

	return data, contentType, nil
}

func locateGeneratedThumbnail(rootDir string) (string, string, error) {
	var locatedPath string
	var contentType string

	walkErr := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(path))
		switch extension {
		case ".png":
			locatedPath = path
			contentType = "image/png"
			return filepath.SkipAll
		case ".jpg", ".jpeg":
			locatedPath = path
			contentType = "image/jpeg"
			return filepath.SkipAll
		default:
			return nil
		}
	})
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		return "", "", walkErr
	}

	if locatedPath == "" {
		return "", "", errors.New("thumbnail not generated")
	}

	return locatedPath, contentType, nil
}
