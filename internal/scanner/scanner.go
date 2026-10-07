package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Progress struct {
	Percent int    `json:"percent"`
	Current int    `json:"current,omitempty"`
	Total   int    `json:"total,omitempty"`
	Stage   string `json:"stage"`
	Detail  string `json:"detail"`
}

type Summary struct {
	TotalFiles      int   `json:"totalFiles"`
	CandidateFiles  int   `json:"candidateFiles"`
	DuplicateGroups int   `json:"duplicateGroups"`
	DuplicateFiles  int   `json:"duplicateFiles"`
	WastedSize      int64 `json:"wastedSize"`
}

type SkippedItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type FileInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Directory     string `json:"directory"`
	RootDirectory string `json:"rootDirectory"`
	RelativePath  string `json:"relativePath"`
	Size          int64  `json:"size"`
	Extension     string `json:"extension"`
	CreatedAt     string `json:"createdAt"`
	CreatedAtMs   int64  `json:"createdAtMs"`
	ModifiedAt    string `json:"modifiedAt"`
	ModifiedAtMs  int64  `json:"modifiedAtMs"`
	Hash          string `json:"hash,omitempty"`
}

type DuplicateGroup struct {
	ID            string     `json:"id"`
	Hash          string     `json:"hash"`
	Size          int64      `json:"size"`
	Extension     string     `json:"extension"`
	FileCount     int        `json:"fileCount"`
	DuplicateSize int64      `json:"duplicateSize"`
	Directories   []string   `json:"directories"`
	Files         []FileInfo `json:"files"`
}

type ScanResult struct {
	ScannedDirectories []string         `json:"scannedDirectories"`
	Summary            Summary          `json:"summary"`
	Groups             []DuplicateGroup `json:"groups"`
	Skipped            []SkippedItem    `json:"skipped"`
}

type ProgressFunc func(Progress)

func ScanDirectories(directories []string, onProgress ProgressFunc) (ScanResult, error) {
	return ScanDirectoriesWithContext(context.Background(), directories, onProgress)
}

func ScanDirectoriesWithContext(ctx context.Context, directories []string, onProgress ProgressFunc) (ScanResult, error) {
	normalizedDirectories := uniqueDirectories(directories)
	files := make([]FileInfo, 0)
	skipped := make([]SkippedItem, 0)
	emitProgress := createProgressEmitter(onProgress)

	emitProgress(Progress{
		Percent: 0,
		Stage:   "准备扫描目录",
		Detail:  "共 " + itoa(len(normalizedDirectories)) + " 个目录",
	})

	if err := checkCancelled(ctx); err != nil {
		return ScanResult{}, err
	}

	for index, rootDirectory := range normalizedDirectories {
		if err := checkCancelled(ctx); err != nil {
			return ScanResult{}, err
		}

		percent := 0
		if len(normalizedDirectories) > 0 {
			percent = int(float64(index) / float64(len(normalizedDirectories)) * 45)
		}

		emitProgress(Progress{
			Percent: percent,
			Stage:   "正在扫描目录",
			Detail:  rootDirectory,
		})

		if err := walkDirectory(ctx, rootDirectory, rootDirectory, &files, &skipped); err != nil {
			return ScanResult{}, err
		}
	}

	emitProgress(Progress{
		Percent: 45,
		Stage:   "正在筛选待比对文件",
		Detail:  "已扫描 " + itoa(len(files)) + " 个文件",
	})

	if err := checkCancelled(ctx); err != nil {
		return ScanResult{}, err
	}

	candidateMap := make(map[string][]FileInfo)
	candidateFiles := make([]FileInfo, 0)
	for _, file := range files {
		if err := checkCancelled(ctx); err != nil {
			return ScanResult{}, err
		}

		key := fileKeyBySize(file)
		candidateMap[key] = append(candidateMap[key], file)
	}

	for _, group := range candidateMap {
		if len(group) > 1 {
			candidateFiles = append(candidateFiles, group...)
		}
	}

	emitProgress(Progress{
		Percent: 55,
		Stage:   "正在快速比对文件特征",
		Detail:  "待比对 " + itoa(len(candidateFiles)) + " 个文件",
	})

	sampledCandidates, sampleSkipped, err := filterCandidatesBySampleWithContext(ctx, candidateFiles, emitProgress)
	if err != nil {
		return ScanResult{}, err
	}
	skipped = append(skipped, sampleSkipped...)

	emitProgress(Progress{
		Percent: 72,
		Stage:   "正在校验文件内容",
		Detail:  "待校验 " + itoa(len(sampledCandidates)) + " 个文件",
	})

	hashedFiles, hashSkipped, err := hashCandidatesWithContext(ctx, sampledCandidates, emitProgress)
	if err != nil {
		return ScanResult{}, err
	}
	skipped = append(skipped, hashSkipped...)

	if err := checkCancelled(ctx); err != nil {
		return ScanResult{}, err
	}

	emitProgress(Progress{
		Percent: 92,
		Stage:   "正在整理重复结果",
		Detail:  "已完成内容校验",
	})

	exactMap := make(map[string][]FileInfo)
	for _, file := range hashedFiles {
		key := fileKeyBySizeAndHash(file)
		exactMap[key] = append(exactMap[key], file)
	}

	groups := make([]DuplicateGroup, 0)
	for _, group := range exactMap {
		if err := checkCancelled(ctx); err != nil {
			return ScanResult{}, err
		}

		if len(group) < 2 {
			continue
		}

		sort.Slice(group, func(i, j int) bool {
			return compareFilesByModifiedAtDesc(group[i], group[j])
		})

		sample := group[0]
		groups = append(groups, DuplicateGroup{
			ID:            sample.Hash + "-" + itoa64(sample.Size),
			Hash:          sample.Hash,
			Size:          sample.Size,
			Extension:     sample.Extension,
			FileCount:     len(group),
			DuplicateSize: sample.Size * int64(len(group)-1),
			Directories:   uniqueRootDirectories(group),
			Files:         group,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		left := groups[i].Files[0]
		right := groups[j].Files[0]
		if left.ModifiedAtMs != right.ModifiedAtMs {
			return left.ModifiedAtMs > right.ModifiedAtMs
		}

		return left.Path < right.Path
	})

	emitProgress(Progress{
		Percent: 100,
		Stage:   "扫描完成",
		Detail:  "找到 " + itoa(len(groups)) + " 组重复文件",
	})

	duplicateFiles := 0
	var wastedSize int64
	for _, group := range groups {
		duplicateFiles += group.FileCount
		wastedSize += group.DuplicateSize
	}

	return ScanResult{
		ScannedDirectories: normalizedDirectories,
		Summary: Summary{
			TotalFiles:      len(files),
			CandidateFiles:  len(candidateFiles),
			DuplicateGroups: len(groups),
			DuplicateFiles:  duplicateFiles,
			WastedSize:      wastedSize,
		},
		Groups:  groups,
		Skipped: skipped,
	}, nil
}

func walkDirectory(ctx context.Context, directory string, rootDirectory string, files *[]FileInfo, skipped *[]SkippedItem) error {
	if err := checkCancelled(ctx); err != nil {
		return err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		*skipped = append(*skipped, SkippedItem{
			Path:   directory,
			Reason: "无法读取目录：" + err.Error(),
		})
		return nil
	}

	for _, entry := range entries {
		if err := checkCancelled(ctx); err != nil {
			return err
		}

		fullPath := filepath.Join(directory, entry.Name())

		if entry.Type()&os.ModeSymlink != 0 {
			*skipped = append(*skipped, SkippedItem{
				Path:   fullPath,
				Reason: "已跳过符号链接",
			})
			continue
		}

		if entry.IsDir() {
			if err := walkDirectory(ctx, fullPath, rootDirectory, files, skipped); err != nil {
				return err
			}
			continue
		}

		if !entry.Type().IsRegular() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			*skipped = append(*skipped, SkippedItem{
				Path:   fullPath,
				Reason: "无法读取文件信息：" + err.Error(),
			})
			continue
		}

		createdAt, createdAtMs := creationTime(info)
		relativePath, relErr := filepath.Rel(rootDirectory, fullPath)
		if relErr != nil {
			relativePath = entry.Name()
		}

		*files = append(*files, FileInfo{
			Name:          entry.Name(),
			Path:          fullPath,
			Directory:     filepath.Dir(fullPath),
			RootDirectory: rootDirectory,
			RelativePath:  relativePath,
			Size:          info.Size(),
			Extension:     normalizeExtension(entry.Name()),
			CreatedAt:     createdAt.Format(time.RFC3339),
			CreatedAtMs:   createdAtMs,
			ModifiedAt:    info.ModTime().Format(time.RFC3339),
			ModifiedAtMs:  info.ModTime().UnixMilli(),
		})
	}

	return nil
}

func uniqueDirectories(directories []string) []string {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(directories))

	for _, directory := range directories {
		if strings.TrimSpace(directory) == "" {
			continue
		}

		absolutePath, err := filepath.Abs(directory)
		if err != nil {
			continue
		}

		cleanPath := filepath.Clean(absolutePath)
		if _, exists := seen[cleanPath]; exists {
			continue
		}

		seen[cleanPath] = struct{}{}
		normalized = append(normalized, cleanPath)
	}

	sort.Strings(normalized)
	return normalized
}

func uniqueRootDirectories(files []FileInfo) []string {
	seen := make(map[string]struct{})
	directories := make([]string, 0)

	for _, file := range files {
		if _, exists := seen[file.RootDirectory]; exists {
			continue
		}

		seen[file.RootDirectory] = struct{}{}
		directories = append(directories, file.RootDirectory)
	}

	sort.Strings(directories)
	return directories
}

func normalizeExtension(fileName string) string {
	extension := strings.ToLower(filepath.Ext(fileName))
	if extension == "" {
		return "(无扩展名)"
	}

	return extension
}

const fileSampleChunkSize int64 = 256 * 1024

type sampleResult struct {
	file   FileInfo
	sample string
	err    error
}

type hashResult struct {
	file FileInfo
	hash string
	err  error
}

func filterCandidatesBySampleWithContext(ctx context.Context, candidateFiles []FileInfo, emitProgress ProgressFunc) ([]FileInfo, []SkippedItem, error) {
	if len(candidateFiles) == 0 {
		return nil, nil, nil
	}

	results := make(chan sampleResult, len(candidateFiles))
	go func() {
		runFileWorkers(ctx, candidateFiles, func(file FileInfo) {
			sample, err := sampleFileWithContext(ctx, file.Path, file.Size)
			results <- sampleResult{
				file:   file,
				sample: sample,
				err:    err,
			}
		})
		close(results)
	}()

	skipped := make([]SkippedItem, 0)
	sampleGroups := make(map[string][]FileInfo)
	processed := 0
	total := len(candidateFiles)

	for result := range results {
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) {
				return nil, skipped, result.err
			}

			skipped = append(skipped, SkippedItem{
				Path:   result.file.Path,
				Reason: "无法读取文件特征：" + result.err.Error(),
			})
		} else {
			key := fileKeyBySizeAndSample(result.file, result.sample)
			sampleGroups[key] = append(sampleGroups[key], result.file)
		}

		processed += 1
		percent := 72
		if total > 0 {
			percent = 55 + int(float64(processed)/float64(total)*17)
		}

		emitProgress(Progress{
			Percent: percent,
			Current: processed,
			Total:   total,
			Stage:   "正在快速比对文件特征",
			Detail:  result.file.Directory,
		})
	}

	if err := checkCancelled(ctx); err != nil {
		return nil, skipped, err
	}

	filtered := make([]FileInfo, 0)
	for _, group := range sampleGroups {
		if len(group) > 1 {
			filtered = append(filtered, group...)
		}
	}

	return filtered, skipped, nil
}

func hashCandidatesWithContext(ctx context.Context, candidateFiles []FileInfo, emitProgress ProgressFunc) ([]FileInfo, []SkippedItem, error) {
	if len(candidateFiles) == 0 {
		return nil, nil, nil
	}

	results := make(chan hashResult, len(candidateFiles))
	go func() {
		runFileWorkers(ctx, candidateFiles, func(file FileInfo) {
			hash, err := hashFileWithContext(ctx, file.Path)
			results <- hashResult{
				file: file,
				hash: hash,
				err:  err,
			}
		})
		close(results)
	}()

	skipped := make([]SkippedItem, 0)
	hashedFiles := make([]FileInfo, 0, len(candidateFiles))
	processed := 0
	total := len(candidateFiles)

	for result := range results {
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) {
				return nil, skipped, result.err
			}

			skipped = append(skipped, SkippedItem{
				Path:   result.file.Path,
				Reason: "无法读取文件内容：" + result.err.Error(),
			})
		} else {
			file := result.file
			file.Hash = result.hash
			hashedFiles = append(hashedFiles, file)
		}

		processed += 1
		percent := 90
		if total > 0 {
			percent = 72 + int(float64(processed)/float64(total)*18)
		}

		emitProgress(Progress{
			Percent: percent,
			Current: processed,
			Total:   total,
			Stage:   "正在校验文件内容",
			Detail:  result.file.Directory,
		})
	}

	if err := checkCancelled(ctx); err != nil {
		return nil, skipped, err
	}

	return hashedFiles, skipped, nil
}

func runFileWorkers(ctx context.Context, files []FileInfo, processor func(FileInfo)) {
	if len(files) == 0 {
		return
	}

	workerCount := calculateFileWorkerCount(len(files))
	jobs := make(chan FileInfo, len(files))
	var workers sync.WaitGroup

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workers.Add(1)
		go func() {
			defer workers.Done()

			for file := range jobs {
				if err := checkCancelled(ctx); err != nil {
					return
				}

				processor(file)
			}
		}()
	}

	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	workers.Wait()
}

func calculateFileWorkerCount(fileCount int) int {
	if fileCount <= 1 {
		return 1
	}

	workerCount := runtime.NumCPU()
	if workerCount < 2 {
		workerCount = 2
	}
	if workerCount > 6 {
		workerCount = 6
	}
	if fileCount < workerCount {
		workerCount = fileCount
	}

	return workerCount
}

func sampleFileWithContext(ctx context.Context, filePath string, fileSize int64) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if fileSize <= fileSampleChunkSize*2 {
		if err := writeReaderBytesWithContext(ctx, file, hash, -1); err != nil {
			return "", err
		}
		return hex.EncodeToString(hash.Sum(nil)), nil
	}

	if err := writeReaderBytesWithContext(ctx, file, hash, fileSampleChunkSize); err != nil {
		return "", err
	}

	if _, err := file.Seek(fileSize-fileSampleChunkSize, io.SeekStart); err != nil {
		return "", err
	}

	if err := writeReaderBytesWithContext(ctx, file, hash, fileSampleChunkSize); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashFileWithContext(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if err := writeReaderBytesWithContext(ctx, file, hash, -1); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeReaderBytesWithContext(ctx context.Context, file *os.File, writer io.Writer, limit int64) error {
	buffer := make([]byte, 1024*1024)
	remaining := limit

	for remaining != 0 {
		if err := checkCancelled(ctx); err != nil {
			return err
		}

		readBuffer := buffer
		if remaining > 0 && int64(len(readBuffer)) > remaining {
			readBuffer = buffer[:int(remaining)]
		}

		bytesRead, readErr := file.Read(readBuffer)
		if bytesRead > 0 {
			if _, writeErr := writer.Write(readBuffer[:bytesRead]); writeErr != nil {
				return writeErr
			}

			if remaining > 0 {
				remaining -= int64(bytesRead)
			}
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	return nil
}

func fileKeyBySize(file FileInfo) string {
	return itoa64(file.Size)
}

func fileKeyBySizeAndSample(file FileInfo, sample string) string {
	return itoa64(file.Size) + "::" + sample
}

func fileKeyBySizeAndHash(file FileInfo) string {
	return itoa64(file.Size) + "::" + file.Hash
}

func compareFilesByModifiedAtDesc(left FileInfo, right FileInfo) bool {
	if left.ModifiedAtMs != right.ModifiedAtMs {
		return left.ModifiedAtMs > right.ModifiedAtMs
	}

	return left.Path < right.Path
}

func createProgressEmitter(onProgress ProgressFunc) ProgressFunc {
	lastPercent := -1
	lastStage := ""
	lastDetail := ""
	lastEmittedAt := time.Time{}

	return func(progress Progress) {
		if onProgress == nil {
			return
		}

		if progress.Percent < 0 {
			progress.Percent = 0
		}
		if progress.Percent > 100 {
			progress.Percent = 100
		}

		now := time.Now()
		shouldEmit := false

		if progress.Percent == 100 || lastPercent == -1 {
			shouldEmit = true
		} else if progress.Stage != lastStage {
			shouldEmit = true
		} else if progress.Percent != lastPercent && now.Sub(lastEmittedAt) >= 250*time.Millisecond {
			shouldEmit = true
		} else if progress.Detail != lastDetail && now.Sub(lastEmittedAt) >= 200*time.Millisecond {
			shouldEmit = true
		}

		if !shouldEmit {
			return
		}

		lastPercent = progress.Percent
		lastStage = progress.Stage
		lastDetail = progress.Detail
		lastEmittedAt = now
		onProgress(progress)
	}
}

func checkCancelled(ctx context.Context) error {
	if ctx == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func itoa(value int) string {
	return strconvFormatInt(int64(value))
}

func itoa64(value int64) string {
	return strconvFormatInt(value)
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
