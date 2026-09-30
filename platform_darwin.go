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

	output, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, err
	}

	return splitOutputLines(output), nil
}

func (app *DesktopApp) RevealFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	command := exec.Command("open", "-R", filePath)
	if err := command.Run(); err != nil {
		return false, err
	}

	return true, nil
}

func (app *DesktopApp) PreviewFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	command := exec.Command("qlmanage", "-p", filePath)
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
	absolutePath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return err
	}

	script := `tell application "Finder" to delete (POSIX file ` + strconv.Quote(absolutePath) + `)`
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
