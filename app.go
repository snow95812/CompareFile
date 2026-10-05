package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"comparefile/internal/scanner"

	webview "github.com/samcharles93/webview_go"
)

const (
	appName    = "重复文件查找器"
	appVersion = "1.0.0"
)

//go:embed frontend/*
var frontendFiles embed.FS

type DeleteFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type DeleteResult struct {
	Deleted []string        `json:"deleted"`
	Failed  []DeleteFailure `json:"failed"`
}

type DesktopApp struct {
	webView    webview.WebView
	serverURL  string
	server     *http.Server
	scanMu     sync.Mutex
	scans      map[string]*scanTask
	nextScan   uint64
	deleteMu   sync.Mutex
	deletes    map[string]*deleteTask
	nextDelete uint64
}

type scanTask struct {
	progress scanner.Progress
	done     bool
	result   *scanner.ScanResult
	err      string
}

type ScanTaskStatus struct {
	Progress scanner.Progress    `json:"progress"`
	Done     bool                `json:"done"`
	Result   *scanner.ScanResult `json:"result,omitempty"`
	Error    string              `json:"error,omitempty"`
}

type deleteTask struct {
	progress scanner.Progress
	done     bool
	result   *DeleteResult
	err      string
}

type DeleteTaskStatus struct {
	Progress scanner.Progress `json:"progress"`
	Done     bool             `json:"done"`
	Result   *DeleteResult    `json:"result,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func NewDesktopApp() *DesktopApp {
	return &DesktopApp{
		scans:   make(map[string]*scanTask),
		deletes: make(map[string]*deleteTask),
	}
}

func (app *DesktopApp) SetWebView(w webview.WebView) {
	app.webView = w
}

func (app *DesktopApp) GetAppVersion() string {
	return appVersion
}

func (app *DesktopApp) BridgeScript() string {
	return `
		window.__duplicateFinderProgressListeners = [];
                window.__duplicateFinderDeleteProgressListeners = [];
		window.__duplicateFinderEmitProgress = function (progress) {
			window.__duplicateFinderProgressListeners.forEach(function (listener) {
				try {
					listener(progress);
				} catch (error) {
					console.error(error);
				}
			});
		};
                window.__duplicateFinderEmitDeleteProgress = function (progress) {
                        window.__duplicateFinderDeleteProgressListeners.forEach(function (listener) {
                                try {
                                        listener(progress);
                                } catch (error) {
                                        console.error(error);
                                }
                        });
                };

		window.duplicateFinderAPI = {
			getAppVersion: function () {
				return getAppVersion();
			},
			selectDirectories: function () {
				return selectDirectories();
			},
			scanDirectories: function (directories) {
                                return startScanDirectories(directories).then(function (scanId) {
                                        return new Promise(function (resolve, reject) {
                                                function handleStatus(status) {
                                                        if (status && status.progress) {
                                                                window.__duplicateFinderEmitProgress(status.progress);
                                                        }

                                                        if (!status || !status.done) {
                                                                return false;
                                                        }

                                                        if (status.error) {
                                                                reject(new Error(status.error));
                                                                return true;
                                                        }

                                                        resolve(status.result || null);
                                                        return true;
                                                }

                                                function poll() {
                                                        getScanStatus(scanId).then(function (status) {
                                                                if (handleStatus(status)) {
                                                                        return;
                                                                }

								window.setTimeout(poll, 250);
                                                        }).catch(function (error) {
                                                                reject(error);
                                                        });
                                                }

                                                poll();
                                        });
                                });
			},
			deleteFiles: function (filePaths) {
                                return startDeleteFiles(filePaths).then(function (deleteId) {
                                        return new Promise(function (resolve, reject) {
                                                function handleStatus(status) {
                                                        if (status && status.progress) {
                                                                window.__duplicateFinderEmitDeleteProgress(status.progress);
                                                        }

                                                        if (!status || !status.done) {
                                                                return false;
                                                        }

                                                        if (status.error) {
                                                                reject(new Error(status.error));
                                                                return true;
                                                        }

                                                        resolve(status.result || null);
                                                        return true;
                                                }

                                                function poll() {
                                                        getDeleteStatus(deleteId).then(function (status) {
                                                                if (handleStatus(status)) {
                                                                        return;
                                                                }

								window.setTimeout(poll, 150);
                                                        }).catch(function (error) {
                                                                reject(error);
                                                        });
                                                }

                                                poll();
                                        });
                                });
			},
			revealFile: function (filePath) {
				return revealFile(filePath);
			},
			previewFile: function (filePath) {
				return previewFile(filePath);
			},
			onScanProgress: function (callback) {
				window.__duplicateFinderProgressListeners.push(callback);
				return function () {
					window.__duplicateFinderProgressListeners =
						window.__duplicateFinderProgressListeners.filter(function (item) {
							return item !== callback;
						});
				};
                        },
                        onDeleteProgress: function (callback) {
                                window.__duplicateFinderDeleteProgressListeners.push(callback);
                                return function () {
                                        window.__duplicateFinderDeleteProgressListeners =
                                                window.__duplicateFinderDeleteProgressListeners.filter(function (item) {
                                                        return item !== callback;
                                                });
                                };
			}
		};
	`
}

func (app *DesktopApp) StartServer() (string, error) {
	subFS, err := fs.Sub(frontendFiles, "frontend")
	if err != nil {
		return "", err
	}

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.FS(subFS))

	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			content, readErr := fs.ReadFile(subFS, "index.html")
			if readErr != nil {
				http.Error(writer, readErr.Error(), http.StatusInternalServerError)
				return
			}

			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(content)
			return
		}
		fileServer.ServeHTTP(writer, request)
	})

	mux.HandleFunc("/file", func(writer http.ResponseWriter, request *http.Request) {
		filePath := strings.TrimSpace(request.URL.Query().Get("path"))
		if filePath == "" {
			http.Error(writer, "missing file path", http.StatusBadRequest)
			return
		}

		cleanPath, err := filepath.Abs(filepath.Clean(filePath))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		info, err := os.Stat(cleanPath)
		if err != nil || info.IsDir() {
			http.Error(writer, "file not found", http.StatusNotFound)
			return
		}

		http.ServeFile(writer, request, cleanPath)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	app.server = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = app.server.Serve(listener)
	}()

	app.serverURL = "http://" + listener.Addr().String()
	return app.serverURL, nil
}

func (app *DesktopApp) Shutdown() error {
	if app.server == nil {
		return nil
	}

	return app.server.Shutdown(context.Background())
}

func (app *DesktopApp) ScanDirectories(directories []string) (scanner.ScanResult, error) {
	if len(directories) == 0 {
		return scanner.ScanResult{}, errors.New("请先选择至少一个目录。")
	}

	return scanner.ScanDirectories(directories, func(progress scanner.Progress) {
		app.emitProgress(progress)
	})
}

func (app *DesktopApp) StartScanDirectories(directories []string) (string, error) {
	if len(directories) == 0 {
		return "", errors.New("请先选择至少一个目录。")
	}

	scanID := strconv.FormatUint(atomic.AddUint64(&app.nextScan, 1), 10)
	initialProgress := scanner.Progress{
		Percent: 0,
		Stage:   "准备扫描目录",
		Detail:  "正在创建扫描任务",
	}

	app.scanMu.Lock()
	app.scans[scanID] = &scanTask{
		progress: initialProgress,
	}
	app.scanMu.Unlock()

	go func() {
		result, err := scanner.ScanDirectories(directories, func(progress scanner.Progress) {
			app.updateScanProgress(scanID, progress)
		})
		app.finishScan(scanID, result, err)
	}()

	return scanID, nil
}

func (app *DesktopApp) GetScanStatus(scanID string) (ScanTaskStatus, error) {
	app.scanMu.Lock()
	task, exists := app.scans[scanID]
	if !exists {
		app.scanMu.Unlock()
		return ScanTaskStatus{}, errors.New("扫描任务不存在。")
	}

	status := ScanTaskStatus{
		Progress: task.progress,
		Done:     task.done,
		Error:    task.err,
	}
	if task.result != nil {
		result := *task.result
		status.Result = &result
	}
	app.scanMu.Unlock()

	return status, nil
}

func (app *DesktopApp) DeleteFiles(filePaths []string) (DeleteResult, error) {
	if len(filePaths) == 0 {
		return DeleteResult{}, errors.New("请先选择要删除的文件。")
	}

	result := DeleteResult{
		Deleted: make([]string, 0, len(filePaths)),
		Failed:  make([]DeleteFailure, 0),
	}

	for _, filePath := range filePaths {
		if err := moveFileToTrash(filePath); err != nil {
			result.Failed = append(result.Failed, DeleteFailure{
				Path:   filePath,
				Reason: "删除失败：" + err.Error(),
			})
			continue
		}

		result.Deleted = append(result.Deleted, filePath)
	}

	return result, nil
}

func (app *DesktopApp) StartDeleteFiles(filePaths []string) (string, error) {
	if len(filePaths) == 0 {
		return "", errors.New("请先选择要删除的文件。")
	}

	deleteID := strconv.FormatUint(atomic.AddUint64(&app.nextDelete, 1), 10)
	initialProgress := scanner.Progress{
		Percent: 0,
		Stage:   "准备删除文件",
		Detail:  "正在创建删除任务",
	}

	app.deleteMu.Lock()
	app.deletes[deleteID] = &deleteTask{
		progress: initialProgress,
	}
	app.deleteMu.Unlock()

	go func() {
		result, err := app.runDeleteFiles(filePaths, func(progress scanner.Progress) {
			app.updateDeleteProgress(deleteID, progress)
		})
		app.finishDelete(deleteID, result, err)
	}()

	return deleteID, nil
}

func (app *DesktopApp) GetDeleteStatus(deleteID string) (DeleteTaskStatus, error) {
	app.deleteMu.Lock()
	task, exists := app.deletes[deleteID]
	if !exists {
		app.deleteMu.Unlock()
		return DeleteTaskStatus{}, errors.New("删除任务不存在。")
	}

	status := DeleteTaskStatus{
		Progress: task.progress,
		Done:     task.done,
		Error:    task.err,
	}
	if task.result != nil {
		result := *task.result
		status.Result = &result
	}
	app.deleteMu.Unlock()

	return status, nil
}

func (app *DesktopApp) emitProgress(progress scanner.Progress) {
	if app.webView == nil {
		return
	}

	payload, err := json.Marshal(progress)
	if err != nil {
		return
	}

	script := "window.__duplicateFinderEmitProgress && window.__duplicateFinderEmitProgress(" + string(payload) + ");"
	app.webView.Dispatch(func() {
		app.webView.Eval(script)
	})
}

func (app *DesktopApp) updateScanProgress(scanID string, progress scanner.Progress) {
	app.scanMu.Lock()
	defer app.scanMu.Unlock()

	task, exists := app.scans[scanID]
	if !exists {
		return
	}

	task.progress = progress
}

func (app *DesktopApp) finishScan(scanID string, result scanner.ScanResult, err error) {
	app.scanMu.Lock()
	defer app.scanMu.Unlock()

	task, exists := app.scans[scanID]
	if !exists {
		return
	}

	task.done = true
	if err != nil {
		task.progress = scanner.Progress{
			Percent: 100,
			Stage:   "扫描失败",
			Detail:  err.Error(),
		}
		task.err = err.Error()
		return
	}

	task.progress = scanner.Progress{
		Percent: 100,
		Stage:   "扫描完成",
		Detail:  "结果已生成",
	}
	resultCopy := result
	task.result = &resultCopy
}

func (app *DesktopApp) runDeleteFiles(filePaths []string, onProgress func(scanner.Progress)) (DeleteResult, error) {
	result := DeleteResult{
		Deleted: make([]string, 0, len(filePaths)),
		Failed:  make([]DeleteFailure, 0),
	}

	emit := scanner.ProgressFunc(onProgress)
	if emit != nil {
		emit(scanner.Progress{
			Percent: 0,
			Stage:   "准备删除文件",
			Detail:  "共 " + strconv.Itoa(len(filePaths)) + " 个文件",
		})
	}

	for index, filePath := range filePaths {
		if emit != nil {
			emit(scanner.Progress{
				Percent: calculateTaskPercent(index, len(filePaths)),
				Stage:   "正在删除文件",
				Detail:  filepath.Base(filePath),
			})
		}

		if err := moveFileToTrash(filePath); err != nil {
			result.Failed = append(result.Failed, DeleteFailure{
				Path:   filePath,
				Reason: "删除失败：" + err.Error(),
			})
		} else {
			result.Deleted = append(result.Deleted, filePath)
		}

		if emit != nil {
			emit(scanner.Progress{
				Percent: calculateTaskPercent(index+1, len(filePaths)),
				Stage:   "正在删除文件",
				Detail:  filepath.Base(filePath),
			})
		}
	}

	return result, nil
}

func (app *DesktopApp) updateDeleteProgress(deleteID string, progress scanner.Progress) {
	app.deleteMu.Lock()
	defer app.deleteMu.Unlock()

	task, exists := app.deletes[deleteID]
	if !exists {
		return
	}

	task.progress = progress
}

func (app *DesktopApp) finishDelete(deleteID string, result DeleteResult, err error) {
	app.deleteMu.Lock()
	defer app.deleteMu.Unlock()

	task, exists := app.deletes[deleteID]
	if !exists {
		return
	}

	task.done = true
	if err != nil {
		task.progress = scanner.Progress{
			Percent: 100,
			Stage:   "删除失败",
			Detail:  err.Error(),
		}
		task.err = err.Error()
		return
	}

	task.progress = scanner.Progress{
		Percent: 100,
		Stage:   "删除完成",
		Detail:  "已处理 " + strconv.Itoa(len(result.Deleted)+len(result.Failed)) + " 个文件",
	}
	resultCopy := result
	task.result = &resultCopy
}

func calculateTaskPercent(current int, total int) int {
	if total <= 0 {
		return 0
	}

	percent := int(float64(current) / float64(total) * 100)
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func splitOutputLines(output []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	seen := make(map[string]struct{})

	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
