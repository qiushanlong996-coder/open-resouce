package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorProjectComments(t *testing.T) {
	ownerCookie, _, editorCookie, editor, project := setupCollaborationTest(t)
	originalDocuments := projectDocumentRepositoryStore
	originalComments := commentRepositoryStore
	projectDocumentRepositoryStore = newMemoryProjectDocumentRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	t.Cleanup(func() {
		projectDocumentRepositoryStore = originalDocuments
		commentRepositoryStore = originalComments
	})

	document, err := projectDocumentRepositoryStore.Create(context.Background(), project.ID, editor.ID, projectDocumentInput{
		Slug: "guide", Title: "使用指南", Markdown: "# 使用指南",
	})
	if err != nil {
		t.Fatal(err)
	}
	comment := documentComment{
		ID: "author-comment-1", DocumentID: document.ID, BlockID: "block-1",
		AuthorID: editor.ID, Author: editor.DisplayName,
		Body: "这一段需要补充示例。", Status: "open", CreatedAt: "2026-08-07T00:00:00Z",
	}
	if _, err := commentRepositoryStore.Create(context.Background(), comment); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/author/projects/" + project.ID + "/comments"

	t.Run("所有者可见评论", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(ownerCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if !strings.Contains(response.Body.String(), `"id":"author-comment-1"`) ||
			!strings.Contains(response.Body.String(), `"document_title":"使用指南"`) {
			t.Fatalf("body = %s", response.Body)
		}
	})

	t.Run("编辑者可查看", func(t *testing.T) {
		add := httptest.NewRequest(http.MethodPost,
			"/api/v1/projects/"+project.Slug+"/collaborators",
			strings.NewReader(`{"email":"collaboration-editor@example.com","role":"editor"}`))
		add.Header.Set("Content-Type", "application/json")
		add.AddCookie(ownerCookie)
		addResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(addResponse, add)
		if addResponse.Code != http.StatusCreated {
			t.Fatalf("add collaborator = %d: %s", addResponse.Code, addResponse.Body)
		}

		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(editorCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
	})

	t.Run("匿名被拒", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})
}
