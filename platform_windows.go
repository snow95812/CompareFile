//go:build windows

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

func (app *DesktopApp) SelectDirectories() ([]string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms | Out-Null
$selected = New-Object System.Collections.Generic.List[string]
while ($true) {
  $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
  $dialog.Description = "选择要扫描的目录"
  $dialog.ShowNewFolderButton = $false
  if ($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) {
    break
  }
  if (-not [string]::IsNullOrWhiteSpace($dialog.SelectedPath) -and -not $selected.Contains($dialog.SelectedPath)) {
    $selected.Add($dialog.SelectedPath) | Out-Null
  }
  $continue = [System.Windows.Forms.MessageBox]::Show(
    "是否继续选择其他目录？",
    "重复文件查找器",
    [System.Windows.Forms.MessageBoxButtons]::YesNo,
    [System.Windows.Forms.MessageBoxIcon]::Question
  )
  if ($continue -ne [System.Windows.Forms.DialogResult]::Yes) {
    break
  }
}
[string]::Join([Environment]::NewLine, $selected)
`

	output, err := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script).Output()
	if err != nil {
		return nil, err
	}

	return splitOutputLines(output), nil
}

func (app *DesktopApp) RevealFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	command := exec.Command("explorer.exe", "/select,"+filePath)
	if err := command.Run(); err != nil {
		return false, err
	}

	return true, nil
}

func (app *DesktopApp) PreviewFile(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("缺少文件路径。")
	}

	command := exec.Command("powershell", "-NoProfile", "-Command", "Start-Process -FilePath '"+escapePowerShellSingleQuoted(filePath)+"'")
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

	script := `
Add-Type -AssemblyName Microsoft.VisualBasic | Out-Null
[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile(
  '` + escapePowerShellSingleQuoted(absolutePath) + `',
  [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,
  [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin
)
`

	command := exec.Command("powershell", "-NoProfile", "-Command", script)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return runErr
		}

		return errors.New(message)
	}

	return nil
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
