package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorProjectMetrics(t *testing.T) {
	ownerCookie, _, editorCookie, _, project := setupCollaborationTest(t)
	originalMetrics := projectMetricsRepositoryStore
	projectMetricsRepositoryStore = newMemoryProjectMetricsRepository()
	t.Cleanup(func() { projectMetricsRepositoryStore = originalMetrics })

	if err := projectMetricsRepositoryStore.IncrementView(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	if err := projectMetricsRepositoryStore.IncrementDownload(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/author/projects/" + project.ID + "/metrics"

	t.Run("匿名被拒", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("非作者被拒", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(editorCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", response.Code, response.Body)
		}
	})

	t.Run("作者可见指标", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(ownerCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if !strings.Contains(response.Body.String(), `"views":1`) ||
			!strings.Contains(response.Body.String(), `"downloads":1`) {
			t.Fatalf("body = %s", response.Body)
		}
	})
}
