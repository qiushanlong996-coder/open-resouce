package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFeedHandler(t *testing.T) {
	originalProjects := managedProjectRepositoryStore
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	t.Cleanup(func() { managedProjectRepositoryStore = originalProjects })

	now := time.Now().UTC()
	project, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
		Slug: "feed-demo", Name: "订阅源项目", Summary: "用于订阅源测试的项目",
		Description: "# 订阅源项目\n\n正文。", Category: "Coding Agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Submit(context.Background(), "owner-1", project.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Review(context.Background(), project.ID, "owner-1", "approve", "", now); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/feed.xml", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/atom+xml") {
		t.Fatalf("content-type = %q", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "<feed") || !strings.Contains(body, "订阅源项目") ||
		!strings.Contains(body, "/projects/feed-demo") {
		t.Fatalf("feed body = %s", body)
	}
}
