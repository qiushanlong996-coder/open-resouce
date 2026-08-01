package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 单文件下载：返回完整内容（不受预览截断限制），并强制以附件形式下载。

func TestProjectCodeFileDownload(t *testing.T) {
	// 构造一个超过预览上限的文件，用于验证下载不被截断。
	large := strings.Repeat("x", maxCodeFilePreviewBytes+2048)
	objectKey := "uploads/user-code/download.zip"
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{
		"README.md":     "# Demo\n说明。",
		"src/main.go":   "package main\n\nfunc main() {}\n",
		"big/large.txt": large,
	}))
	base := "/api/v1/projects/" + slug + "/code/file/download"

	t.Run("返回完整内容并强制下载", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, base+"?path=src/main.go", nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if body := response.Body.String(); body != "package main\n\nfunc main() {}\n" {
			t.Fatalf("body = %q", body)
		}
		header := response.Header()
		// 必须强制下载：仓库里的 HTML/SVG 不能在同源下被浏览器执行。
		if got := header.Get("Content-Type"); got != "application/octet-stream" {
			t.Fatalf("content type = %q", got)
		}
		if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("nosniff = %q", got)
		}
		disposition := header.Get("Content-Disposition")
		if !strings.HasPrefix(disposition, "attachment;") || !strings.Contains(disposition, "main.go") {
			t.Fatalf("content disposition = %q", disposition)
		}
	})

	t.Run("超过预览上限的文件也完整返回", func(t *testing.T) {
		// 预览接口会截断同一个文件，下载接口不应截断。
		preview := httptest.NewRequest(http.MethodGet,
			"/api/v1/projects/"+slug+"/code/file?path=big/large.txt", nil)
		previewResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(previewResponse, preview)
		if !strings.Contains(previewResponse.Body.String(), `"truncated":true`) {
			t.Fatalf("preview should be truncated: %s", previewResponse.Body.String()[:200])
		}

		request := httptest.NewRequest(http.MethodGet, base+"?path=big/large.txt", nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
		if response.Body.Len() != len(large) {
			t.Fatalf("download length = %d, want %d (下载不应截断)", response.Body.Len(), len(large))
		}
	})

	t.Run("路径穿越与不存在的文件被拒绝", func(t *testing.T) {
		for name, testCase := range map[string]struct {
			query  string
			status int
		}{
			"路径穿越":  {"?path=../../etc/passwd", http.StatusBadRequest},
			"绝对路径":  {"?path=/etc/passwd", http.StatusBadRequest},
			"空路径":   {"", http.StatusBadRequest},
			"文件不存在": {"?path=not/here.go", http.StatusNotFound},
			"目录":    {"?path=src", http.StatusNotFound},
		} {
			request := httptest.NewRequest(http.MethodGet, base+testCase.query, nil)
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("%s status = %d, want %d: %s", name, response.Code, testCase.status, response.Body)
			}
		}
	})

	t.Run("只接受 GET", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, base+"?path=src/main.go", nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("post status = %d, want 405", response.Code)
		}
	})
}

// TestCodeFileDownloadAllowsBinary 二进制文件不能预览但应当可以下载。
func TestCodeFileDownloadAllowsBinary(t *testing.T) {
	binary := string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe})
	objectKey := "uploads/user-code/binary.zip"
	slug := setupCodeBrowseTest(t, objectKey, buildTestZip(t, map[string]string{
		"assets/logo.bin": binary,
	}))

	preview := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file?path=assets/logo.bin", nil)
	previewResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(previewResponse, preview)
	if previewResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("binary preview status = %d, want 415", previewResponse.Code)
	}

	download := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+slug+"/code/file/download?path=assets/logo.bin", nil)
	downloadResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK {
		t.Fatalf("binary download status = %d, want 200", downloadResponse.Code)
	}
	if downloadResponse.Body.String() != binary {
		t.Fatalf("binary download body mismatch")
	}
}
