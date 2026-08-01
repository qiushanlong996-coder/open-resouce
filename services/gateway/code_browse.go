package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// 可在线浏览的代码包上限。超过此大小只提供下载，避免 Gateway 内存被单个请求占满。
	maxBrowsableArchiveBytes = 64 << 20
	// 单个文件预览上限，超出部分截断并标记 truncated。
	maxCodeFilePreviewBytes = 512 << 10
	// 目录树条目上限，防止超大仓库拖垮前端渲染。
	maxCodeTreeEntries = 2000
	// 解压后累计读取上限，防止 zip 炸弹。
	maxTotalUncompressedBytes = 256 << 20
	codeArchiveCacheTTL       = 10 * time.Minute
	codeArchiveCacheMaxBytes  = 192 << 20
)

var (
	errArchiveTooLarge      = errors.New("code archive exceeds browsable size")
	errArchiveUnsupported   = errors.New("unsupported code archive format")
	errArchiveMalformed     = errors.New("malformed code archive")
	errCodeFileNotFound     = errors.New("code file not found")
	errCodeFileNotPreviewed = errors.New("code file is not previewable")
)

type codeEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size"`
}

type codeTreeResponse struct {
	Data       []codeEntry `json:"data"`
	ReadmePath string      `json:"readme_path,omitempty"`
	Truncated  bool        `json:"truncated"`
	RequestID  string      `json:"request_id"`
}

type codeFile struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Language  string `json:"language"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type codeFileResponse struct {
	Data      codeFile `json:"data"`
	RequestID string   `json:"request_id"`
}

// codeArchive 是解析后的代码包视图。文件内容按需从原始字节读取，
// 因此这里只保留归档字节和轻量元数据。
type codeArchive struct {
	format     string
	raw        []byte
	entries    []codeEntry
	readmePath string
	truncated  bool
	cachedAt   time.Time
}

type codeArchiveCache struct {
	sync.Mutex
	byKey      map[string]*codeArchive
	totalBytes int64
}

func newCodeArchiveCache() *codeArchiveCache {
	return &codeArchiveCache{byKey: make(map[string]*codeArchive)}
}

func (cache *codeArchiveCache) get(objectKey string) (*codeArchive, bool) {
	cache.Lock()
	defer cache.Unlock()
	archive, found := cache.byKey[objectKey]
	if !found {
		return nil, false
	}
	if time.Since(archive.cachedAt) > codeArchiveCacheTTL {
		delete(cache.byKey, objectKey)
		cache.totalBytes -= int64(len(archive.raw))
		return nil, false
	}
	return archive, true
}

func (cache *codeArchiveCache) put(objectKey string, archive *codeArchive) {
	cache.Lock()
	defer cache.Unlock()
	if existing, found := cache.byKey[objectKey]; found {
		cache.totalBytes -= int64(len(existing.raw))
	}
	// 容量不足时优先淘汰最旧的条目，保证缓存总量有上限。
	for cache.totalBytes+int64(len(archive.raw)) > codeArchiveCacheMaxBytes && len(cache.byKey) > 0 {
		oldestKey, oldest := "", time.Now()
		for key, candidate := range cache.byKey {
			if candidate.cachedAt.Before(oldest) {
				oldestKey, oldest = key, candidate.cachedAt
			}
		}
		if oldestKey == "" {
			break
		}
		cache.totalBytes -= int64(len(cache.byKey[oldestKey].raw))
		delete(cache.byKey, oldestKey)
	}
	cache.byKey[objectKey] = archive
	cache.totalBytes += int64(len(archive.raw))
}

var codeArchives = newCodeArchiveCache()

// safeArchivePath 归一化归档内路径并阻断路径穿越（zip slip）。
func safeArchivePath(raw string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" || strings.HasPrefix(normalized, "/") {
		return "", false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
		strings.Contains(cleaned, "/../") || strings.HasSuffix(cleaned, "/..") {
		return "", false
	}
	if strings.Contains(cleaned, "\x00") || len(cleaned) > 400 {
		return "", false
	}
	return cleaned, true
}

func detectArchiveFormat(objectKey string, raw []byte) string {
	lower := strings.ToLower(objectKey)
	switch {
	case len(raw) > 3 && raw[0] == 'P' && raw[1] == 'K':
		return "zip"
	case len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b:
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	}
	return ""
}

func parseCodeArchive(objectKey string, raw []byte) (*codeArchive, error) {
	format := detectArchiveFormat(objectKey, raw)
	archive := &codeArchive{format: format, raw: raw, cachedAt: time.Now()}
	var files []codeEntry
	var err error
	switch format {
	case "zip":
		files, archive.truncated, err = listZipEntries(raw)
	case "tar.gz", "tar":
		files, archive.truncated, err = listTarEntries(raw, format == "tar.gz")
	default:
		return nil, errArchiveUnsupported
	}
	if err != nil {
		return nil, err
	}
	archive.entries = buildCodeTree(files)
	archive.readmePath = pickReadme(files)
	return archive, nil
}

func listZipEntries(raw []byte) ([]codeEntry, bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, false, errArchiveMalformed
	}
	files := make([]codeEntry, 0, len(reader.File))
	truncated := false
	var totalUncompressed int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		cleaned, ok := safeArchivePath(entry.Name)
		if !ok {
			continue
		}
		size := int64(entry.UncompressedSize64)
		totalUncompressed += size
		if totalUncompressed > maxTotalUncompressedBytes {
			truncated = true
			break
		}
		if len(files) >= maxCodeTreeEntries {
			truncated = true
			break
		}
		files = append(files, codeEntry{Path: cleaned, Name: path.Base(cleaned), Size: size})
	}
	return files, truncated, nil
}

func listTarEntries(raw []byte, compressed bool) ([]codeEntry, bool, error) {
	stream, err := tarStream(raw, compressed)
	if err != nil {
		return nil, false, err
	}
	reader := tar.NewReader(stream)
	files := make([]codeEntry, 0, 64)
	truncated := false
	var totalUncompressed int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, errArchiveMalformed
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		cleaned, ok := safeArchivePath(header.Name)
		if !ok {
			continue
		}
		totalUncompressed += header.Size
		if totalUncompressed > maxTotalUncompressedBytes {
			truncated = true
			break
		}
		if len(files) >= maxCodeTreeEntries {
			truncated = true
			break
		}
		files = append(files, codeEntry{Path: cleaned, Name: path.Base(cleaned), Size: header.Size})
	}
	return files, truncated, nil
}

func tarStream(raw []byte, compressed bool) (io.Reader, error) {
	if !compressed {
		return bytes.NewReader(raw), nil
	}
	decompressed, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, errArchiveMalformed
	}
	// gzip 解压后仍需限制总读取量，避免高压缩比归档耗尽内存。
	return io.LimitReader(decompressed, maxTotalUncompressedBytes), nil
}

// buildCodeTree 在文件列表基础上补齐目录条目，并按目录优先、同级字典序排序。
func buildCodeTree(files []codeEntry) []codeEntry {
	directories := make(map[string]struct{})
	for _, file := range files {
		for parent := path.Dir(file.Path); parent != "." && parent != "/"; parent = path.Dir(parent) {
			directories[parent] = struct{}{}
		}
	}
	entries := make([]codeEntry, 0, len(files)+len(directories))
	for directory := range directories {
		entries = append(entries, codeEntry{Path: directory, Name: path.Base(directory), Dir: true})
	}
	entries = append(entries, files...)
	sort.SliceStable(entries, func(left, right int) bool {
		leftDepth, rightDepth := strings.Count(entries[left].Path, "/"), strings.Count(entries[right].Path, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		leftParent, rightParent := path.Dir(entries[left].Path), path.Dir(entries[right].Path)
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		if entries[left].Dir != entries[right].Dir {
			return entries[left].Dir
		}
		return entries[left].Path < entries[right].Path
	})
	return entries
}

// pickReadme 选择展示优先级最高的 README：根目录优先，其次路径最短。
func pickReadme(files []codeEntry) string {
	best := ""
	for _, file := range files {
		name := strings.ToLower(file.Name)
		if name != "readme.md" && name != "readme" && name != "readme.txt" && name != "readme.rst" {
			continue
		}
		if best == "" {
			best = file.Path
			continue
		}
		bestDepth, currentDepth := strings.Count(best, "/"), strings.Count(file.Path, "/")
		if currentDepth < bestDepth || (currentDepth == bestDepth && len(file.Path) < len(best)) {
			best = file.Path
		}
	}
	return best
}

func (archive *codeArchive) readFile(target string) (codeFile, error) {
	for _, entry := range archive.entries {
		if entry.Dir || entry.Path != target {
			continue
		}
		content, truncated, err := archive.extract(target)
		if err != nil {
			return codeFile{}, err
		}
		if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			return codeFile{}, errCodeFileNotPreviewed
		}
		return codeFile{
			Path: target, Size: entry.Size, Language: codeLanguage(target),
			Content: string(content), Truncated: truncated,
		}, nil
	}
	return codeFile{}, errCodeFileNotFound
}

func (archive *codeArchive) extract(target string) ([]byte, bool, error) {
	switch archive.format {
	case "zip":
		reader, err := zip.NewReader(bytes.NewReader(archive.raw), int64(len(archive.raw)))
		if err != nil {
			return nil, false, errArchiveMalformed
		}
		for _, entry := range reader.File {
			cleaned, ok := safeArchivePath(entry.Name)
			if !ok || entry.FileInfo().IsDir() || cleaned != target {
				continue
			}
			handle, err := entry.Open()
			if err != nil {
				return nil, false, errArchiveMalformed
			}
			defer handle.Close()
			return readLimited(handle)
		}
	case "tar.gz", "tar":
		stream, err := tarStream(archive.raw, archive.format == "tar.gz")
		if err != nil {
			return nil, false, err
		}
		reader := tar.NewReader(stream)
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, false, errArchiveMalformed
			}
			cleaned, ok := safeArchivePath(header.Name)
			if !ok || header.Typeflag != tar.TypeReg || cleaned != target {
				continue
			}
			return readLimited(reader)
		}
	}
	return nil, false, errCodeFileNotFound
}

func readLimited(source io.Reader) ([]byte, bool, error) {
	content, err := io.ReadAll(io.LimitReader(source, maxCodeFilePreviewBytes+1))
	if err != nil {
		return nil, false, errArchiveMalformed
	}
	if len(content) > maxCodeFilePreviewBytes {
		return content[:maxCodeFilePreviewBytes], true, nil
	}
	return content, false, nil
}

func codeLanguage(filePath string) string {
	byExtension := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".mjs": "javascript",
		".cjs": "javascript", ".ts": "typescript", ".tsx": "tsx", ".jsx": "jsx",
		".java": "java", ".kt": "kotlin", ".rs": "rust", ".rb": "ruby",
		".php": "php", ".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp",
		".cs": "csharp", ".swift": "swift", ".sh": "bash", ".bash": "bash",
		".zsh": "bash", ".sql": "sql", ".json": "json", ".yaml": "yaml",
		".yml": "yaml", ".toml": "toml", ".xml": "xml", ".html": "html",
		".css": "css", ".scss": "scss", ".md": "markdown", ".rst": "rest",
		".dockerfile": "dockerfile", ".ini": "ini", ".env": "ini",
	}
	lower := strings.ToLower(filePath)
	if language, found := byExtension[path.Ext(lower)]; found {
		return language
	}
	switch path.Base(lower) {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "go.mod", "go.sum":
		return "text"
	}
	return "text"
}

// loadProjectCodeArchive 读取已发布项目的代码包，命中缓存时直接复用。
func loadProjectCodeArchive(ctx context.Context, objectKey string) (*codeArchive, error) {
	if archive, found := codeArchives.get(objectKey); found {
		return archive, nil
	}
	if objectStorageStore == nil {
		return nil, errArchiveUnsupported
	}
	raw, err := objectStorageStore.GetObject(ctx, objectKey, maxBrowsableArchiveBytes)
	if errors.Is(err, errObjectTooLarge) {
		return nil, errArchiveTooLarge
	}
	if err != nil {
		return nil, err
	}
	archive, err := parseCodeArchive(objectKey, raw)
	if err != nil {
		return nil, err
	}
	codeArchives.put(objectKey, archive)
	return archive, nil
}

func projectCodeTreeHandler(writer http.ResponseWriter, request *http.Request) {
	archive, ok := resolveProjectCodeArchive(writer, request)
	if !ok {
		return
	}
	entries := archive.entries
	if query := strings.TrimSpace(request.URL.Query().Get("q")); query != "" {
		if len([]rune(query)) > 100 {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_query", "搜索关键词不能超过 100 个字符")
			return
		}
		entries = filterCodeEntries(entries, query)
	}
	writeJSON(writer, http.StatusOK, codeTreeResponse{
		Data: entries, ReadmePath: archive.readmePath, Truncated: archive.truncated,
		RequestID: requestIDFromContext(request.Context()),
	})
}

// filterCodeEntries 按文件名匹配，命中后只返回文件条目，便于前端直接展示搜索结果。
func filterCodeEntries(entries []codeEntry, query string) []codeEntry {
	normalized := strings.ToLower(query)
	filtered := make([]codeEntry, 0, 16)
	for _, entry := range entries {
		if entry.Dir || !strings.Contains(strings.ToLower(entry.Path), normalized) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func projectCodeFileHandler(writer http.ResponseWriter, request *http.Request) {
	archive, ok := resolveProjectCodeArchive(writer, request)
	if !ok {
		return
	}
	target, valid := safeArchivePath(request.URL.Query().Get("path"))
	if !valid {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_query", "文件路径无效")
		return
	}
	file, err := archive.readFile(target)
	switch {
	case errors.Is(err, errCodeFileNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "code_file_not_found", "代码文件不存在")
		return
	case errors.Is(err, errCodeFileNotPreviewed):
		writeAPIError(writer, request, http.StatusUnsupportedMediaType, "code_file_not_previewable",
			"该文件不是文本文件，请下载代码包查看")
		return
	case err != nil:
		writeCodeArchiveError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, codeFileResponse{
		Data: file, RequestID: requestIDFromContext(request.Context()),
	})
}

// resolveProjectCodeArchive 校验请求方法、项目发布状态和代码包存在性，并返回解析后的归档。
func resolveProjectCodeArchive(writer http.ResponseWriter, request *http.Request) (*codeArchive, bool) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return nil, false
	}
	slug := request.PathValue("slug")
	if slug == "" {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return nil, false
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), slug)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return nil, false
	}
	if !found || project.CodeObjectKey == "" {
		writeAPIError(writer, request, http.StatusNotFound, "code_archive_not_found", "该项目没有可浏览的代码包")
		return nil, false
	}
	archive, err := loadProjectCodeArchive(request.Context(), project.CodeObjectKey)
	if err != nil {
		writeCodeArchiveError(writer, request, err)
		return nil, false
	}
	return archive, true
}

func writeCodeArchiveError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, errArchiveTooLarge):
		writeAPIError(writer, request, http.StatusRequestEntityTooLarge, "code_archive_too_large",
			fmt.Sprintf("代码包超过 %d MB，暂不支持在线浏览，请下载后查看", maxBrowsableArchiveBytes>>20))
	case errors.Is(err, errArchiveUnsupported):
		writeAPIError(writer, request, http.StatusUnsupportedMediaType, "code_archive_unsupported",
			"代码包格式暂不支持在线浏览，请下载后查看")
	case errors.Is(err, errArchiveMalformed):
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "code_archive_malformed",
			"代码包无法解析，请重新上传")
	default:
		slog.ErrorContext(request.Context(), "code archive load failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusBadGateway, "storage_error", "文件存储暂时不可用")
	}
}
