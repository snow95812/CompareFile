//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	winfilepicker "github.com/zyoung11/GO-WinFilePicker"
)

func (app *DesktopApp) SelectDirectories() ([]string, error) {
	selectedPaths, err := winfilepicker.SelectFolders("选择要扫描的目录")
	if err != nil {
		lowerError := strings.ToLower(err.Error())
		if strings.Contains(lowerError, "canceled") || strings.Contains(lowerError, "cancelled") || strings.Contains(err.Error(), "取消") {
			return nil, nil
		}
		return nil, err
	}

	directories := make([]string, 0, len(selectedPaths))
	for _, selectedPath := range selectedPaths {
		selectedPath = strings.TrimSpace(selectedPath)
		if selectedPath == "" {
			continue
		}
		directories = append(directories, selectedPath)
	}

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

	targetPath := absolutePath
	if info, statErr := os.Stat(absolutePath); statErr == nil && !info.IsDir() {
		targetPath = filepath.Dir(absolutePath)
	}

	if err := shellOpen(targetPath); err != nil {
		return false, err
	}

	return true, nil
}

func (app *DesktopApp) PreviewFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	if err := shellOpen(filePath); err != nil {
		return false, err
	}

	return true, nil
}

func moveFileToTrash(filePath string) error {
	return moveFilesToTrash([]string{filePath})
}

func moveFilesToTrash(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	pathUTF16 := make([]uint16, 0, len(filePaths)*260)
	validPathCount := 0
	for _, filePath := range filePaths {
		if strings.TrimSpace(filePath) == "" {
			continue
		}

		absolutePath, err := filepath.Abs(filepath.Clean(filePath))
		if err != nil {
			return err
		}

		encodedPath, err := syscall.UTF16FromString(absolutePath)
		if err != nil {
			return err
		}

		pathUTF16 = append(pathUTF16, encodedPath...)
		validPathCount += 1
	}
	if validPathCount == 0 {
		return nil
	}
	pathUTF16 = append(pathUTF16, 0)

	fileOp := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &pathUTF16[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	result, _, callErr := shell32SHFileOperationW.Call(uintptr(unsafe.Pointer(&fileOp)))
	if result != 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("移动到回收站失败，错误代码：%d", result)
	}
	if fileOp.fAnyOperationsAborted != 0 {
		return errors.New("移动到回收站已取消。")
	}

	return nil
}

func shellOpen(filePath string) error {
	operation, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	target, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}

	result, _, callErr := shell32ShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(operation)),
		uintptr(unsafe.Pointer(target)),
		0,
		0,
		swShowDefault,
	)
	if result <= 32 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("打开文件失败，错误代码：%d", result)
	}

	return nil
}

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
	swShowDefault     = 10
)

type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var (
	shell32DLL              = syscall.NewLazyDLL("shell32.dll")
	shell32SHFileOperationW = shell32DLL.NewProc("SHFileOperationW")
	shell32ShellExecuteW    = shell32DLL.NewProc("ShellExecuteW")
)
