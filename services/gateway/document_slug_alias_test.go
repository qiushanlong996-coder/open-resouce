package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 文档标识变更后旧阅读链接应 301 到新地址，而不是直接 404。

func TestRenamedDocumentRedirectsOldSlug(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	base := "/api/v1/author/projects/" + project.ID + "/documents"
	document := createDocument(t, cookie, project.ID,
		`{"slug":"old-guide","title":"使用指南","markdown":"# 使用指南\n\n指南内容。"}`)
	public := "/api/v1/projects/" + project.Slug + "/documents/"

	// 改名前旧地址可直接访问。
	before := httptest.NewRecorder()
	newHandler().ServeHTTP(before, httptest.NewRequest(http.MethodGet, public+"old-guide", nil))
	if before.Code != http.StatusOK {
		t.Fatalf("original slug status = %d: %s", before.Code, before.Body)
	}

	rename := httptest.NewRequest(http.MethodPut, base+"/"+document.ID,
		strings.NewReader(`{"slug":"new-guide","title":"使用指南","markdown":"# 使用指南\n\n指南内容。"}`))
	rename.AddCookie(cookie)
	renameResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(renameResponse, rename)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status = %d: %s", renameResponse.Code, renameResponse.Body)
	}

	t.Run("旧地址 301 到新地址", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, public+"old-guide", nil))
		if response.Code != http.StatusMovedPermanently {
			t.Fatalf("old slug status = %d, want 301: %s", response.Code, response.Body)
		}
		location := response.Header().Get("Location")
		if !strings.HasSuffix(location, "/documents/new-guide") {
			t.Fatalf("Location = %q", location)
		}
		if got := response.Header().Get("X-Document-Slug"); got != "new-guide" {
			t.Fatalf("X-Document-Slug = %q", got)
		}
	})

	t.Run("新地址正常返回内容", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, public+"new-guide", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("new slug status = %d: %s", response.Code, response.Body)
		}
		var payload documentDetailResponse
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.Slug != "new-guide" || !strings.Contains(payload.Data.Markdown, "指南内容") {
			t.Fatalf("new slug payload = %#v", payload.Data)
		}
	})

	t.Run("从未存在过的 slug 仍是 404", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, public+"never-existed", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown slug status = %d, want 404", response.Code)
		}
	})

	t.Run("多次改名后每个历史地址都能重定向到最新", func(t *testing.T) {
		second := httptest.NewRequest(http.MethodPut, base+"/"+document.ID,
			strings.NewReader(`{"slug":"final-guide","title":"使用指南","markdown":"# 使用指南\n\n指南内容。"}`))
		second.AddCookie(cookie)
		secondResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(secondResponse, second)
		if secondResponse.Code != http.StatusOK {
			t.Fatalf("second rename status = %d: %s", secondResponse.Code, secondResponse.Body)
		}
		for _, old := range []string{"old-guide", "new-guide"} {
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, public+old, nil))
			if response.Code != http.StatusMovedPermanently {
				t.Fatalf("%s status = %d, want 301", old, response.Code)
			}
			if got := response.Header().Get("X-Document-Slug"); got != "final-guide" {
				t.Fatalf("%s redirects to %q, want final-guide", old, got)
			}
		}
	})
}

// TestRenamedDocumentAliasCannotBeHijacked 历史 slug 不能被其他文档占用，
// 否则旧链接会被导向错误的文档。
func TestRenamedDocumentAliasCannotBeHijacked(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	base := "/api/v1/author/projects/" + project.ID + "/documents"
	document := createDocument(t, cookie, project.ID,
		`{"slug":"taken-slug","title":"原文档","markdown":""}`)

	rename := httptest.NewRequest(http.MethodPut, base+"/"+document.ID,
		strings.NewReader(`{"slug":"renamed-slug","title":"原文档","markdown":""}`))
	rename.AddCookie(cookie)
	renameResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(renameResponse, rename)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status = %d: %s", renameResponse.Code, renameResponse.Body)
	}

	// 另一篇文档试图使用刚释放出来的历史 slug，必须被拒绝。
	create := httptest.NewRequest(http.MethodPost, base,
		strings.NewReader(`{"slug":"taken-slug","title":"冒名文档","markdown":""}`))
	create.AddCookie(cookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusConflict {
		t.Fatalf("hijack create status = %d, want 409: %s", createResponse.Code, createResponse.Body)
	}

	// 旧地址仍指向原文档。
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents/taken-slug", nil))
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("old slug status = %d, want 301", response.Code)
	}
	if got := response.Header().Get("X-Document-Slug"); got != "renamed-slug" {
		t.Fatalf("X-Document-Slug = %q, want renamed-slug", got)
	}
}

// TestDocumentAliasClearedOnDelete 文档删除后其历史别名不应再重定向。
func TestDocumentAliasClearedOnDelete(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	base := "/api/v1/author/projects/" + project.ID + "/documents"
	document := createDocument(t, cookie, project.ID,
		`{"slug":"doomed-old","title":"待删文档","markdown":""}`)

	rename := httptest.NewRequest(http.MethodPut, base+"/"+document.ID,
		strings.NewReader(`{"slug":"doomed-new","title":"待删文档","markdown":""}`))
	rename.AddCookie(cookie)
	newHandler().ServeHTTP(httptest.NewRecorder(), rename)

	remove := httptest.NewRequest(http.MethodDelete, base+"/"+document.ID, nil)
	remove.AddCookie(cookie)
	removeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(removeResponse, remove)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", removeResponse.Code, removeResponse.Body)
	}

	// 删除后项目已无文档，回退到项目正文；历史别名不应把请求导向已删文档。
	if _, found, err := projectDocumentRepositoryStore.FindByAliasSlug(
		context.Background(), project.ID, "doomed-old"); err != nil || found {
		t.Fatalf("alias should be cleared after delete: found=%v err=%v", found, err)
	}
}
