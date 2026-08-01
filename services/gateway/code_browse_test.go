package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// buildTestZip 生成用于代码浏览测试的 zip 归档。
func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	for name, body := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func buildTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	compressor := gzip.NewWriter(buffer)
	archive := tar.NewWriter(compressor)
	for name, body := range files {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// setupCodeBrowseTest 准备一个已发布项目及其代码包，返回项目 slug。
func setupCodeBrowseTest(t *testing.T, objectKey string, archive []byte) string {
	t.Helper()
	originalProjects, originalStorage := managedProjectRepositoryStore, objectStorageStore
	repository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = repository
	objectStorageStore = &fakeObjectStorage{objects: map[string][]byte{objectKey: archive}}
	codeArchives = newCodeArchiveCache()
	t.Cleanup(func() {
		managedProjectRepositoryStore, objectStorageStore = originalProjects, originalStorage
		codeArchives = newCodeArchiveCache()
	})

	ctx := context.Background()
	ownerID := "user-code-" + newRequestID()
	slug := "code-demo-" + newRequestID()
	project, err := repository.Create(ctx, ownerID, managedProjectInput{
		Slug: slug, Name: "Code Demo", Summary: "\u7528\u4e8e\u9a8c\u8bc1\u4ee3\u7801\u5728\u7ebf\u6d4f\u89c8\u7684\u793a\u4f8b\u9879\u76ee",
		Description: "\u8fd9\u662f\u8db3\u591f\u957f\u7684\u9879\u76ee\u4ecb\u7ecd\uff0c\u7528\u6765\u9a8c\u8bc1\u4ee3\u7801\u76ee\u5f55\u6811\u548c\u6587\u4ef6\u8bfb\u53d6\u3002",
		Category:    "Coding Agent", Tags: []string{"Agent"}, TechStack: []string{"Go"},
		License: "MIT", CurrentVersion: "0.1.0", CodeObjectKey: objectKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := repository.Submit(ctx, ownerID, project.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Review(ctx, project.ID, "admin", "approve", "", now); err != nil {
		t.Fatal(err)
	}
	return slug
}

func TestProjectCodeTreeAndFile(t *testing.T) {
	objectKey := "uploads/user-code/demo.zip"
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{
		"README.md":          "# Demo\n\u9879\u76ee\u8bf4\u660e\u3002",
		"src/main.go":        "package main\n\nfunc main() {}\n",
		"src/util/helper.py": "def helper():\n    return 1\n",
	}))

	treeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+slug+"/code", nil)
	treeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(treeResponse, treeRequest)
	if treeResponse.Code != http.StatusOK {
		t.Fatalf("code tree status = %d: %s", treeResponse.Code, treeResponse.Body)
	}
	var tree codeTreeResponse
	if err := json.Unmarshal(treeResponse.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	if tree.ReadmePath != "README.md" {
		t.Fatalf("readme path = %q", tree.ReadmePath)
	}
	if tree.Truncated {
		t.Fatal("small archive should not be truncated")
	}
	paths := map[string]codeEntry{}
	for _, entry := range tree.Data {
		paths[entry.Path] = entry
	}
	// 目录条目必须由文件路径推导补齐。
	for _, expected := range []string{"README.md", "src", "src/main.go", "src/util", "src/util/helper.py"} {
		if _, found := paths[expected]; !found {
			t.Fatalf("missing tree entry %q in %#v", expected, tree.Data)
		}
	}
	if !paths["src"].Dir || paths["src/main.go"].Dir {
		t.Fatalf("directory flags incorrect: %#v", tree.Data)
	}
	if paths["src/main.go"].Size == 0 {
		t.Fatal("file size should be reported")
	}

	fileRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file?path=src/main.go", nil)
	fileResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(fileResponse, fileRequest)
	if fileResponse.Code != http.StatusOK {
		t.Fatalf("code file status = %d: %s", fileResponse.Code, fileResponse.Body)
	}
	var file codeFileResponse
	if err := json.Unmarshal(fileResponse.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Data.Language != "go" || !strings.Contains(file.Data.Content, "func main()") ||
		file.Data.Truncated {
		t.Fatalf("unexpected code file: %#v", file.Data)
	}
}

func TestProjectCodeTarGzAndSearch(t *testing.T) {
	objectKey := "uploads/user-code/demo.tar.gz"
	slug := setupCodeBrowseTest(t, objectKey, buildTestTarGz(t, map[string]string{
		"app/readme.md":      "# Tar Demo\n",
		"app/server.go":      "package app\n",
		"app/static/app.css": "body { margin: 0; }\n",
	}))

	searchRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code?q=server", nil)
	searchResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(searchResponse, searchRequest)
	if searchResponse.Code != http.StatusOK {
		t.Fatalf("code search status = %d: %s", searchResponse.Code, searchResponse.Body)
	}
	var tree codeTreeResponse
	if err := json.Unmarshal(searchResponse.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Data) != 1 || tree.Data[0].Path != "app/server.go" {
		t.Fatalf("search result = %#v", tree.Data)
	}

	fileRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file?path=app/static/app.css", nil)
	fileResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(fileResponse, fileRequest)
	if fileResponse.Code != http.StatusOK {
		t.Fatalf("tar file status = %d: %s", fileResponse.Code, fileResponse.Body)
	}
	var file codeFileResponse
	if err := json.Unmarshal(fileResponse.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Data.Language != "css" || !strings.Contains(file.Data.Content, "margin") {
		t.Fatalf("unexpected tar file: %#v", file.Data)
	}
}

func TestProjectCodeRejectsPathTraversal(t *testing.T) {
	objectKey := "uploads/user-code/evil.zip"
	// 归档内的穿越路径必须在建树阶段被丢弃。
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{
		"safe.txt":               "safe\n",
		"../../etc/passwd":       "root:x:0:0\n",
		"/absolute/secret.txt":   "secret\n",
		"nested/../../escape.sh": "echo escaped\n",
	}))

	treeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+slug+"/code", nil)
	treeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(treeResponse, treeRequest)
	var tree codeTreeResponse
	if err := json.Unmarshal(treeResponse.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	for _, entry := range tree.Data {
		if strings.Contains(entry.Path, "..") || strings.HasPrefix(entry.Path, "/") ||
			strings.Contains(entry.Path, "passwd") || strings.Contains(entry.Path, "secret") {
			t.Fatalf("unsafe entry leaked into tree: %#v", entry)
		}
	}

	// 请求参数中的穿越路径必须被拒绝。
	for _, target := range []string{"../../etc/passwd", "/etc/passwd", "nested/../../escape.sh", ""} {
		request := httptest.NewRequest(http.MethodGet,
			"/api/v1/projects/"+slug+"/code/file?path="+target, nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
			t.Fatalf("traversal path %q status = %d: %s", target, response.Code, response.Body)
		}
	}
}

func TestProjectCodeRejectsBinaryFile(t *testing.T) {
	objectKey := "uploads/user-code/binary.zip"
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{
		"logo.bin": string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}),
	}))
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file?path=logo.bin", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("binary file status = %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "code_file_not_previewable") {
		t.Fatalf("unexpected binary error body: %s", response.Body)
	}
}

func TestProjectCodeTruncatesLargeFile(t *testing.T) {
	objectKey := "uploads/user-code/large.zip"
	large := strings.Repeat("a", maxCodeFilePreviewBytes+1024)
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{"big.txt": large}))
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file?path=big.txt", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("large file status = %d: %s", response.Code, response.Body)
	}
	var file codeFileResponse
	if err := json.Unmarshal(response.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if !file.Data.Truncated || len(file.Data.Content) != maxCodeFilePreviewBytes {
		t.Fatalf("large file not truncated: truncated=%v length=%d",
			file.Data.Truncated, len(file.Data.Content))
	}
}

func TestProjectCodeMissingArchive(t *testing.T) {
	originalProjects, originalStorage := managedProjectRepositoryStore, objectStorageStore
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	objectStorageStore = &fakeObjectStorage{objects: map[string][]byte{}}
	t.Cleanup(func() {
		managedProjectRepositoryStore, objectStorageStore = originalProjects, originalStorage
	})
	// 未发布或不存在的项目不应暴露代码浏览能力。
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-found/code", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing project code status = %d: %s", response.Code, response.Body)
	}
}

func TestProjectCodeRejectsWrongMethod(t *testing.T) {
	objectKey := "uploads/user-code/method.zip"
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{"a.txt": "a\n"}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+slug+"/code", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("wrong method status = %d allow=%q", response.Code, response.Header().Get("Allow"))
	}
}

func TestSafeArchivePathRejectsEscapes(t *testing.T) {
	for _, raw := range []string{
		"", "/", "..", "../x", "a/../../b", "/etc/passwd", "./..", "a/..",
		"x\x00y", strings.Repeat("d/", 250) + "deep.txt",
	} {
		if cleaned, ok := safeArchivePath(raw); ok {
			t.Fatalf("path %q should be rejected, got %q", raw, cleaned)
		}
	}
	for raw, expected := range map[string]string{
		"a.txt": "a.txt", "./a.txt": "a.txt", "src/main.go": "src/main.go",
		"src\\win.go": "src/win.go", "a/b/../c.txt": "a/c.txt",
	} {
		cleaned, ok := safeArchivePath(raw)
		if !ok || cleaned != expected {
			t.Fatalf("path %q => %q (ok=%v), want %q", raw, cleaned, ok, expected)
		}
	}
}

func TestCodeArchiveCacheEvictsExpiredEntry(t *testing.T) {
	cache := newCodeArchiveCache()
	cache.put("key-a", &codeArchive{raw: []byte("abc"), cachedAt: time.Now()})
	if _, found := cache.get("key-a"); !found {
		t.Fatal("fresh cache entry should be found")
	}
	// 过期条目必须在读取时淘汰，并归还占用的字节数。
	cache.put("key-b", &codeArchive{raw: []byte("defgh"), cachedAt: time.Now().Add(-2 * codeArchiveCacheTTL)})
	if _, found := cache.get("key-b"); found {
		t.Fatal("expired cache entry should be evicted")
	}
	if cache.totalBytes != int64(len("abc")) {
		t.Fatalf("cache totalBytes = %d, want 3", cache.totalBytes)
	}
}

func TestCodeLanguageDetection(t *testing.T) {
	for filePath, expected := range map[string]string{
		"main.go": "go", "app.py": "python", "index.tsx": "tsx",
		"Dockerfile": "dockerfile", "Makefile": "makefile",
		"config.yml": "yaml", "notes.md": "markdown", "unknown.xyz": "text",
		"go.mod": "text",
	} {
		if language := codeLanguage(filePath); language != expected {
			t.Fatalf("codeLanguage(%q) = %q, want %q", filePath, language, expected)
		}
	}
}
