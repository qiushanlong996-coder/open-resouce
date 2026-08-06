package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivityHandler(t *testing.T) {
	originalProjects := managedProjectRepositoryStore
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	t.Cleanup(func() { managedProjectRepositoryStore = originalProjects })

	now := time.Now().UTC()
	project, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
		Slug: "activity-demo", Name: "动态项目", Summary: "用于社区动态测试的项目",
		Description: "# 动态项目\n\n正文。", Category: "Coding Agent",
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
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"type":"project_published"`) ||
		!strings.Contains(body, `"type":"project_updated"`) ||
		!strings.Contains(body, `"project_slug":"activity-demo"`) {
		t.Fatalf("activity body = %s", body)
	}
}
