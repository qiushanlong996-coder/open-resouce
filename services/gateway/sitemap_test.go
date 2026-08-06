package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSitemapHandler(t *testing.T) {
	originalProjects := managedProjectRepositoryStore
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	t.Cleanup(func() { managedProjectRepositoryStore = originalProjects })

	now := time.Now().UTC()
	project, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
		Slug: "sitemap-demo", Name: "站点地图项目", Summary: "测试", Description: "# 站点地图项目\n\n正文。",
		Category: "Coding Agent",
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
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "sitemaps.org/schemas/sitemap") ||
		!strings.Contains(body, "/projects/sitemap-demo") {
		t.Fatalf("sitemap body = %s", body)
	}
}
