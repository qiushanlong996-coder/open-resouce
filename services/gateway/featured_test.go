package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFeaturedProjects(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "featured-admin@example.com")
	originalAuth := authRepositoryStore
	originalProjects := managedProjectRepositoryStore
	originalFeatured := featuredProjectRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	featuredProjectRepositoryStore = newMemoryFeaturedProjectRepository()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		managedProjectRepositoryStore = originalProjects
		featuredProjectRepositoryStore = originalFeatured
		authRateLimiter = originalLimiter
	})

	now := time.Now().UTC()
	seedPublished := func(slug, name string) managedProject {
		t.Helper()
		project, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
			Slug: slug, Name: name, Summary: name + " 简介", Description: "# " + name + "\n\n正文。",
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
		return project
	}
	first := seedPublished("featured-one", "推荐项目一")
	second := seedPublished("featured-two", "推荐项目二")

	adminCookie, _ := registerTestUser(t, "featured-admin@example.com", "管理")

	t.Run("匿名不可写", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/featured",
			strings.NewReader(`{"project_ids":["`+first.ID+`"]}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("管理员设置推荐", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/featured",
			strings.NewReader(`{"project_ids":["`+first.ID+`","`+second.ID+`"]}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(adminCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
	})

	t.Run("公开推荐列表按序返回", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/featured", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
		body := response.Body.String()
		firstIndex := strings.Index(body, `"slug":"featured-one"`)
		secondIndex := strings.Index(body, `"slug":"featured-two"`)
		if firstIndex < 0 || secondIndex < 0 || firstIndex > secondIndex {
			t.Fatalf("featured order wrong: %s", body)
		}
	})

	t.Run("只能推荐已发布项目", func(t *testing.T) {
		draft, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
			Slug: "featured-draft", Name: "草稿项目", Summary: "草稿", Description: "# 草稿\n\n正文。",
			Category: "Coding Agent",
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/featured",
			strings.NewReader(`{"project_ids":["`+draft.ID+`"]}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(adminCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", response.Code)
		}
	})
}
