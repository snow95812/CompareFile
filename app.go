package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
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
	Deleted   []string        `json:"deleted"`
	Failed    []DeleteFailure `json:"failed"`
	Cancelled bool            `json:"cancelled"`
}

type DesktopApp struct {
	webView       webview.WebView
	serverURL     string
	server        *http.Server
	thumbMu       sync.Mutex
	thumbDir      string
	thumbCache    map[string]thumbnailCacheEntry
	thumbCacheSeq uint64
	scanMu        sync.Mutex
	scans         map[string]*scanTask
	nextScan      uint64
	deleteMu      sync.Mutex
	deletes       map[string]*deleteTask
	nextDelete    uint64
}

type thumbnailCacheEntry struct {
	Data        []byte
	ContentType string
	Sequence    uint64
}

type scanTask struct {
	progress scanner.Progress
	done     bool
	result   *scanner.ScanResult
	err      string
	cancel   context.CancelFunc
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
	cancel   context.CancelFunc
}

type DeleteTaskStatus struct {
	Progress scanner.Progress `json:"progress"`
	Done     bool             `json:"done"`
	Result   *DeleteResult    `json:"result,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func NewDesktopApp() *DesktopApp {
	return &DesktopApp{
		thumbDir:   resolveThumbnailCacheDir(),
		thumbCache: make(map[string]thumbnailCacheEntry),
		scans:      make(map[string]*scanTask),
		deletes:    make(map[string]*deleteTask),
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
                window.__duplicateFinderActiveScanId = null;
                window.__duplicateFinderActiveDeleteId = null;
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
                                        window.__duplicateFinderActiveScanId = scanId;
					return new Promise(function (resolve, reject) {
						function handleStatus(status) {
							if (status && status.progress) {
								window.__duplicateFinderEmitProgress(status.progress);
							}

							if (!status || !status.done) {
								return false;
							}

							if (status.error) {
                                                                window.__duplicateFinderActiveScanId = null;
								reject(new Error(status.error));
								return true;
							}

                                                        window.__duplicateFinderActiveScanId = null;
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
                        cancelScan: function () {
                                if (!window.__duplicateFinderActiveScanId) {
                                        return Promise.resolve(false);
                                }

                                return cancelScan(window.__duplicateFinderActiveScanId);
                        },
			deleteFiles: function (filePaths) {
				return startDeleteFiles(filePaths).then(function (deleteId) {
                                        window.__duplicateFinderActiveDeleteId = deleteId;
					return new Promise(function (resolve, reject) {
						function handleStatus(status) {
							if (status && status.progress) {
								window.__duplicateFinderEmitDeleteProgress(status.progress);
							}

							if (!status || !status.done) {
								return false;
							}

							if (status.error) {
                                                                window.__duplicateFinderActiveDeleteId = null;
								reject(new Error(status.error));
								return true;
							}

                                                        window.__duplicateFinderActiveDeleteId = null;
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
                        cancelDelete: function () {
                                if (!window.__duplicateFinderActiveDeleteId) {
                                        return Promise.resolve(false);
                                }

                                return cancelDelete(window.__duplicateFinderActiveDeleteId);
                        },
			revealFile: function (filePath) {
				return revealFile(filePath);
			},
			previewFile: function (filePath) {
				return previewFile(filePath);
			},
			clearThumbnailCache: function () {
				return clearThumbnailCache();
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
		cleanPath, err := resolveLocalFilePath(strings.TrimSpace(request.URL.Query().Get("path")))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		if _, err := os.Stat(cleanPath); err != nil {
			http.Error(writer, "file not found", http.StatusNotFound)
			return
		}

		http.ServeFile(writer, request, cleanPath)
	})

	mux.HandleFunc("/thumbnail", func(writer http.ResponseWriter, request *http.Request) {
		cleanPath, err := resolveLocalFilePath(strings.TrimSpace(request.URL.Query().Get("path")))
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		size := 100
		if rawSize := strings.TrimSpace(request.URL.Query().Get("size")); rawSize != "" {
			if parsedSize, parseErr := strconv.Atoi(rawSize); parseErr == nil && parsedSize > 0 && parsedSize <= 512 {
				size = parsedSize
			}
		}

		thumbnailData, contentType, err := app.ensureThumbnail(cleanPath, size)
		if err != nil {
			// Fallback to the original file when thumbnail generation is unavailable for
			// a format, so the result list still renders instead of showing a broken image.
			http.ServeFile(writer, request, cleanPath)
			return
		}

		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(thumbnailData)
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

	return scanner.ScanDirectoriesWithContext(context.Background(), directories, func(progress scanner.Progress) {
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

	ctx, cancel := context.WithCancel(context.Background())

	app.scanMu.Lock()
	app.scans[scanID] = &scanTask{
		progress: initialProgress,
		cancel:   cancel,
	}
	app.scanMu.Unlock()

	go func() {
		result, err := scanner.ScanDirectoriesWithContext(ctx, directories, func(progress scanner.Progress) {
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

func (app *DesktopApp) CancelScan(scanID string) (bool, error) {
	app.scanMu.Lock()
	defer app.scanMu.Unlock()

	task, exists := app.scans[scanID]
	if !exists {
		return false, errors.New("扫描任务不存在。")
	}
	if task.done || task.cancel == nil {
		return false, nil
	}

	task.progress = scanner.Progress{
		Percent: task.progress.Percent,
		Stage:   "正在中止扫描",
		Detail:  "等待当前文件处理结束",
	}
	task.cancel()
	task.cancel = nil
	return true, nil
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
		Current: 0,
		Total:   len(filePaths),
		Stage:   "准备删除文件",
		Detail:  "正在创建删除任务",
	}

	ctx, cancel := context.WithCancel(context.Background())

	app.deleteMu.Lock()
	app.deletes[deleteID] = &deleteTask{
		progress: initialProgress,
		cancel:   cancel,
	}
	app.deleteMu.Unlock()

	go func() {
		result, err := app.runDeleteFiles(ctx, filePaths, func(progress scanner.Progress) {
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

func (app *DesktopApp) CancelDelete(deleteID string) (bool, error) {
	app.deleteMu.Lock()
	defer app.deleteMu.Unlock()

	task, exists := app.deletes[deleteID]
	if !exists {
		return false, errors.New("删除任务不存在。")
	}
	if task.done || task.cancel == nil {
		return false, nil
	}

	task.progress = scanner.Progress{
		Percent: task.progress.Percent,
		Current: task.progress.Current,
		Total:   task.progress.Total,
		Stage:   "正在中止删除",
		Detail:  "等待当前批次处理结束",
	}
	task.cancel()
	task.cancel = nil
	return true, nil
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
	task.cancel = nil
	if errors.Is(err, context.Canceled) {
		task.progress = scanner.Progress{
			Percent: task.progress.Percent,
			Stage:   "扫描已中止",
			Detail:  "用户已中止扫描",
		}
		task.err = "扫描已中止。"
		return
	}
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

func (app *DesktopApp) runDeleteFiles(ctx context.Context, filePaths []string, onProgress func(scanner.Progress)) (DeleteResult, error) {
	result := DeleteResult{
		Deleted: make([]string, 0, len(filePaths)),
		Failed:  make([]DeleteFailure, 0),
	}

	emit := scanner.ProgressFunc(onProgress)
	totalFiles := len(filePaths)
	if emit != nil {
		emit(scanner.Progress{
			Percent: 0,
			Current: 0,
			Total:   totalFiles,
			Stage:   "准备删除文件",
			Detail:  "共 " + strconv.Itoa(totalFiles) + " 个文件",
		})
	}

	processedCount := 0
	for _, batch := range chunkFilePaths(filePaths, 32) {
		if err := ctx.Err(); err != nil {
			result.Cancelled = true
			return result, err
		}

		if emit != nil {
			emit(scanner.Progress{
				Percent: calculateTaskPercent(processedCount, totalFiles),
				Current: processedCount,
				Total:   totalFiles,
				Stage:   "正在删除文件",
				Detail:  "当前批次 " + strconv.Itoa(len(batch)) + " 个文件",
			})
		}

		if err := moveFilesToTrash(batch); err == nil {
			result.Deleted = append(result.Deleted, batch...)
			processedCount += len(batch)
			if emit != nil {
				emit(scanner.Progress{
					Percent: calculateTaskPercent(processedCount, totalFiles),
					Current: processedCount,
					Total:   totalFiles,
					Stage:   "正在删除文件",
					Detail:  "已完成 " + strconv.Itoa(processedCount) + " 个文件",
				})
			}
			continue
		}

		for _, filePath := range batch {
			if err := ctx.Err(); err != nil {
				result.Cancelled = true
				return result, err
			}

			if emit != nil {
				emit(scanner.Progress{
					Percent: calculateTaskPercent(processedCount, totalFiles),
					Current: processedCount,
					Total:   totalFiles,
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
			processedCount += 1

			if emit != nil {
				emit(scanner.Progress{
					Percent: calculateTaskPercent(processedCount, totalFiles),
					Current: processedCount,
					Total:   totalFiles,
					Stage:   "正在删除文件",
					Detail:  filepath.Base(filePath),
				})
			}
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
	task.cancel = nil
	if errors.Is(err, context.Canceled) {
		result.Cancelled = true
		task.progress = scanner.Progress{
			Percent: task.progress.Percent,
			Current: task.progress.Current,
			Total:   task.progress.Total,
			Stage:   "删除已中止",
			Detail:  "已处理中止请求",
		}
		resultCopy := result
		task.result = &resultCopy
		return
	}
	if err != nil {
		task.progress = scanner.Progress{
			Percent: 100,
			Current: task.progress.Current,
			Total:   task.progress.Total,
			Stage:   "删除失败",
			Detail:  err.Error(),
		}
		task.err = err.Error()
		return
	}

	task.progress = scanner.Progress{
		Percent: 100,
		Current: len(result.Deleted) + len(result.Failed),
		Total:   len(result.Deleted) + len(result.Failed),
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

func chunkFilePaths(filePaths []string, batchSize int) [][]string {
	if batchSize <= 0 {
		batchSize = len(filePaths)
	}

	chunks := make([][]string, 0, (len(filePaths)+batchSize-1)/batchSize)
	for start := 0; start < len(filePaths); start += batchSize {
		end := start + batchSize
		if end > len(filePaths) {
			end = len(filePaths)
		}
		chunks = append(chunks, filePaths[start:end])
	}

	return chunks
}

func resolveThumbnailCacheDir() string {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = os.TempDir()
	}

	cacheDir := filepath.Join(cacheRoot, "DuplicateFinder", "thumbnail-cache")
	if mkdirErr := os.MkdirAll(cacheDir, 0o755); mkdirErr != nil {
		fallbackDir := filepath.Join(os.TempDir(), "DuplicateFinder", "thumbnail-cache")
		_ = os.MkdirAll(fallbackDir, 0o755)
		return fallbackDir
	}

	return cacheDir
}

func resolveLocalFilePath(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.New("missing file path")
	}

	cleanPath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}

	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", errors.New("file not found")
	}

	return cleanPath, nil
}

func (app *DesktopApp) ensureThumbnail(filePath string, size int) ([]byte, string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, "", err
	}

	cacheKey := strings.Join([]string{
		filePath,
		strconv.FormatInt(info.Size(), 10),
		strconv.FormatInt(info.ModTime().UnixNano(), 10),
		strconv.Itoa(size),
	}, "|")

	app.thumbMu.Lock()
	if entry, exists := app.thumbCache[cacheKey]; exists {
		entry.Sequence = app.thumbCacheSeq + 1
		app.thumbCacheSeq = entry.Sequence
		app.thumbCache[cacheKey] = entry
		app.thumbMu.Unlock()
		return entry.Data, entry.ContentType, nil
	}
	app.thumbMu.Unlock()

	sourceFile, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer sourceFile.Close()

	sourceImage, _, err := image.Decode(sourceFile)
	if err != nil {
		return nil, "", err
	}

	thumbnailImage := createSquareThumbnail(sourceImage, size)
	buffer := bytes.NewBuffer(nil)
	if err := jpeg.Encode(buffer, thumbnailImage, &jpeg.Options{Quality: 72}); err != nil {
		return nil, "", err
	}

	thumbnailData := buffer.Bytes()

	app.thumbMu.Lock()
	defer app.thumbMu.Unlock()

	if entry, exists := app.thumbCache[cacheKey]; exists {
		entry.Sequence = app.thumbCacheSeq + 1
		app.thumbCacheSeq = entry.Sequence
		app.thumbCache[cacheKey] = entry
		return entry.Data, entry.ContentType, nil
	}

	app.thumbCacheSeq += 1
	app.thumbCache[cacheKey] = thumbnailCacheEntry{
		Data:        append([]byte(nil), thumbnailData...),
		ContentType: "image/jpeg",
		Sequence:    app.thumbCacheSeq,
	}
	pruneThumbnailCache(app.thumbCache, 240)

	return thumbnailData, "image/jpeg", nil
}

func (app *DesktopApp) ClearThumbnailCache() (string, error) {
	app.thumbMu.Lock()
	cachedCount := len(app.thumbCache)
	app.thumbCache = make(map[string]thumbnailCacheEntry)
	app.thumbCacheSeq = 0
	app.thumbMu.Unlock()

	removedFiles, err := clearThumbnailCacheDir(app.thumbDir)
	if err != nil {
		return "", err
	}

	if cachedCount == 0 && removedFiles == 0 {
		return "缩略图缓存已经是空的。", nil
	}

	return "已清除缩略图缓存。", nil
}

func createSquareThumbnail(source image.Image, size int) image.Image {
	if size <= 0 {
		size = 100
	}

	cropBounds := cropToSquare(source.Bounds())
	destination := image.NewRGBA(image.Rect(0, 0, size, size))
	scaleX := float64(cropBounds.Dx()) / float64(size)
	scaleY := float64(cropBounds.Dy()) / float64(size)

	for y := 0; y < size; y += 1 {
		sourceY := float64(cropBounds.Min.Y) + ((float64(y) + 0.5) * scaleY) - 0.5
		y0, y1, weightY := sampleAxis(sourceY, cropBounds.Min.Y, cropBounds.Max.Y-1)

		for x := 0; x < size; x += 1 {
			sourceX := float64(cropBounds.Min.X) + ((float64(x) + 0.5) * scaleX) - 0.5
			x0, x1, weightX := sampleAxis(sourceX, cropBounds.Min.X, cropBounds.Max.X-1)
			destination.Set(x, y, blendSample(
				source.At(x0, y0),
				source.At(x1, y0),
				source.At(x0, y1),
				source.At(x1, y1),
				weightX,
				weightY,
			))
		}
	}

	return destination
}

func cropToSquare(bounds image.Rectangle) image.Rectangle {
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= height {
		offsetY := (height - width) / 2
		return image.Rect(bounds.Min.X, bounds.Min.Y+offsetY, bounds.Max.X, bounds.Min.Y+offsetY+width)
	}

	offsetX := (width - height) / 2
	return image.Rect(bounds.Min.X+offsetX, bounds.Min.Y, bounds.Min.X+offsetX+height, bounds.Max.Y)
}

func sampleAxis(value float64, min int, max int) (int, int, float64) {
	if max <= min {
		return min, min, 0
	}

	if value <= float64(min) {
		return min, min, 0
	}
	if value >= float64(max) {
		return max, max, 0
	}

	lower := int(value)
	upper := lower + 1
	if upper > max {
		upper = max
	}

	weight := value - float64(lower)
	return lower, upper, weight
}

func blendSample(topLeft color.Color, topRight color.Color, bottomLeft color.Color, bottomRight color.Color, weightX float64, weightY float64) color.Color {
	r00, g00, b00, a00 := topLeft.RGBA()
	r10, g10, b10, a10 := topRight.RGBA()
	r01, g01, b01, a01 := bottomLeft.RGBA()
	r11, g11, b11, a11 := bottomRight.RGBA()

	return color.RGBA{
		R: blendChannel(r00, r10, r01, r11, weightX, weightY),
		G: blendChannel(g00, g10, g01, g11, weightX, weightY),
		B: blendChannel(b00, b10, b01, b11, weightX, weightY),
		A: blendChannel(a00, a10, a01, a11, weightX, weightY),
	}
}

func blendChannel(topLeft uint32, topRight uint32, bottomLeft uint32, bottomRight uint32, weightX float64, weightY float64) uint8 {
	top := (float64(topLeft)*(1-weightX) + float64(topRight)*weightX) / 257
	bottom := (float64(bottomLeft)*(1-weightX) + float64(bottomRight)*weightX) / 257
	value := top*(1-weightY) + bottom*weightY
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}

	return uint8(value + 0.5)
}

func pruneThumbnailCache(cache map[string]thumbnailCacheEntry, maxEntries int) {
	if len(cache) <= maxEntries {
		return
	}

	var oldestKey string
	var oldestSequence uint64
	first := true
	for key, entry := range cache {
		if first || entry.Sequence < oldestSequence {
			oldestKey = key
			oldestSequence = entry.Sequence
			first = false
		}
	}

	if oldestKey != "" {
		delete(cache, oldestKey)
	}
}

func clearThumbnailCacheDir(cacheDir string) (int, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	removedCount := 0
	for _, entry := range entries {
		targetPath := filepath.Join(cacheDir, entry.Name())
		if removeErr := os.RemoveAll(targetPath); removeErr != nil {
			return removedCount, removeErr
		}
		removedCount += 1
	}

	return removedCount, nil
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
