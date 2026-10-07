//go:build darwin

package main

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func (app *DesktopApp) SelectDirectories() ([]string, error) {
	script := `
		set chosenFolders to choose folder with prompt "选择要扫描的目录" with multiple selections allowed
		set outputText to ""
		repeat with folderAlias in chosenFolders
			set outputText to outputText & POSIX path of folderAlias & linefeed
		end repeat
		return outputText
	`

	output, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		outputText := strings.TrimSpace(string(output))
		lowerError := strings.ToLower(err.Error() + "\n" + outputText)
		if strings.Contains(lowerError, "canceled") || strings.Contains(lowerError, "cancelled") || strings.Contains(outputText, "取消") {
			return nil, nil
		}
		return nil, err
	}

	directories := splitOutputLines(output)
	if len(directories) == 0 {
		return nil, nil
	}

	return directories, nil
}

func (app *DesktopApp) RevealFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	absolutePath, err := filepath.Abs(filepath.Clean(filepath.FromSlash(filePath)))
	if err != nil {
		return false, err
	}

	command := exec.Command("open", "-R", absolutePath)
	if err := command.Run(); err != nil {
		return false, err
	}

	return true, nil
}

func (app *DesktopApp) PreviewFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	absolutePath, err := filepath.Abs(filepath.Clean(filepath.FromSlash(filePath)))
	if err != nil {
		return false, err
	}

	command := exec.Command("qlmanage", "-p", absolutePath)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := command.Start(); err != nil {
		return false, err
	}

	_ = command.Process.Release()
	return true, nil
}

func moveFileToTrash(filePath string) error {
	return moveFilesToTrash([]string{filePath})
}

func moveFilesToTrash(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	normalizedPaths := make([]string, 0, len(filePaths))
	for _, filePath := range filePaths {
		if strings.TrimSpace(filePath) == "" {
			continue
		}

		absolutePath, err := filepath.Abs(filepath.Clean(filepath.FromSlash(filePath)))
		if err != nil {
			return err
		}
		normalizedPaths = append(normalizedPaths, absolutePath)
	}

	if len(normalizedPaths) == 0 {
		return nil
	}

	appleScriptItems := make([]string, 0, len(normalizedPaths))
	for _, absolutePath := range normalizedPaths {
		appleScriptItems = append(appleScriptItems, `POSIX file `+strconv.Quote(absolutePath))
	}

	script := `
		set fileItems to {` + strings.Join(appleScriptItems, ", ") + `}
		tell application "Finder"
			delete fileItems
		end tell
	`
	command := exec.Command("osascript", "-e", script)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return runErr
		}

		return errors.New(message)
	}

	return nil
}
