package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Progress struct {
	Percent int    `json:"percent"`
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
	normalizedDirectories := uniqueDirectories(directories)
	files := make([]FileInfo, 0)
	skipped := make([]SkippedItem, 0)
	emitProgress := createProgressEmitter(onProgress)

	emitProgress(Progress{
		Percent: 0,
		Stage:   "准备扫描目录",
		Detail:  "共 " + itoa(len(normalizedDirectories)) + " 个目录",
	})

	for index, rootDirectory := range normalizedDirectories {
		percent := 0
		if len(normalizedDirectories) > 0 {
			percent = int(float64(index) / float64(len(normalizedDirectories)) * 45)
		}

		emitProgress(Progress{
			Percent: percent,
			Stage:   "正在扫描目录",
			Detail:  rootDirectory,
		})

		walkDirectory(rootDirectory, rootDirectory, &files, &skipped)
	}

	emitProgress(Progress{
		Percent: 45,
		Stage:   "正在筛选待比对文件",
		Detail:  "已扫描 " + itoa(len(files)) + " 个文件",
	})

	candidateMap := make(map[string][]FileInfo)
	candidateFiles := make([]FileInfo, 0)
	for _, file := range files {
		key := fileKeyBySizeAndExtension(file)
		candidateMap[key] = append(candidateMap[key], file)
	}

	for _, group := range candidateMap {
		if len(group) > 1 {
			candidateFiles = append(candidateFiles, group...)
		}
	}

	emitProgress(Progress{
		Percent: 55,
		Stage:   "正在校验文件内容",
		Detail:  "待比对 " + itoa(len(candidateFiles)) + " 个文件",
	})

	hashedFiles := make([]FileInfo, 0, len(candidateFiles))
	for index, file := range candidateFiles {
		hash, err := hashFile(file.Path)
		if err != nil {
			skipped = append(skipped, SkippedItem{
				Path:   file.Path,
				Reason: "无法读取文件内容：" + err.Error(),
			})
			continue
		}

		file.Hash = hash
		hashedFiles = append(hashedFiles, file)

		percent := 90
		if len(candidateFiles) > 0 {
			percent = 55 + int(float64(index+1)/float64(len(candidateFiles))*35)
		}

		emitProgress(Progress{
			Percent: percent,
			Stage:   "正在校验文件内容",
			Detail:  file.Directory,
		})
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

func walkDirectory(directory string, rootDirectory string, files *[]FileInfo, skipped *[]SkippedItem) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		*skipped = append(*skipped, SkippedItem{
			Path:   directory,
			Reason: "无法读取目录：" + err.Error(),
		})
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(directory, entry.Name())

		if entry.Type()&os.ModeSymlink != 0 {
			*skipped = append(*skipped, SkippedItem{
				Path:   fullPath,
				Reason: "已跳过符号链接",
			})
			continue
		}

		if entry.IsDir() {
			walkDirectory(fullPath, rootDirectory, files, skipped)
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

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileKeyBySizeAndExtension(file FileInfo) string {
	return itoa64(file.Size) + "::" + file.Extension
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

func itoa(value int) string {
	return strconvFormatInt(int64(value))
}

func itoa64(value int64) string {
	return strconvFormatInt(value)
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
