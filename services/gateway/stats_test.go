package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCollectSiteStats 验证公开统计的口径：
// 已发布项目数、今天更新的项目数、累计下载量、真实文档篇数。
func TestCollectSiteStats(t *testing.T) {
	originalProjects := managedProjectRepositoryStore
	originalDocuments := projectDocumentRepositoryStore
	originalMetrics := projectMetricsRepositoryStore
	t.Cleanup(func() {
		managedProjectRepositoryStore = originalProjects
		projectDocumentRepositoryStore = originalDocuments
		projectMetricsRepositoryStore = originalMetrics
	})

	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	projectDocumentRepositoryStore = newMemoryProjectDocumentRepository()
	projectMetricsRepositoryStore = newMemoryProjectMetricsRepository()

	now := time.Now().UTC()
	first, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
		Slug: "stats-one", Name: "统计项目一", Summary: "用于统计的项目一",
		Description: "# 统计项目一\n\n正文。", Category: "Coding Agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Submit(context.Background(), "owner-1", first.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Review(context.Background(), first.ID, "owner-1", "approve", "", now); err != nil {
		t.Fatal(err)
	}
	second, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
		Slug: "stats-two", Name: "统计项目二", Summary: "用于统计的项目二",
		Description: "# 统计项目二\n\n正文。", Category: "RAG Agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Submit(context.Background(), "owner-1", second.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := projectRepository.Review(context.Background(), second.ID, "owner-1", "approve", "", now); err != nil {
		t.Fatal(err)
	}

	if _, err := projectDocumentRepositoryStore.Create(context.Background(), first.ID, "owner-1", projectDocumentInput{
		Slug: "guide", Title: "使用指南", Markdown: "# 使用指南",
	}); err != nil {
		t.Fatal(err)
	}
	if err := projectMetricsRepositoryStore.IncrementDownload(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if err := projectMetricsRepositoryStore.IncrementDownload(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}

	stats, err := collectSiteStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Projects != 2 {
		t.Fatalf("projects = %d, want 2", stats.Projects)
	}
	if stats.Documents != 1 {
		t.Fatalf("documents = %d, want 1", stats.Documents)
	}
	if stats.Downloads != 2 {
		t.Fatalf("downloads = %d, want 2", stats.Downloads)
	}
	if stats.UpdatedToday < 1 {
		t.Fatalf("updated_today = %d, want >= 1", stats.UpdatedToday)
	}
}

func TestSiteStatsHandlerPublic(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}
	if !containsJSONKey(response.Body.String(), "projects") {
		t.Fatalf("body missing projects: %s", response.Body)
	}
}

func containsJSONKey(body, key string) bool {
	return strings.Contains(body, `"`+key+`"`)
}
