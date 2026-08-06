package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHotTagsHandler(t *testing.T) {
	originalProjects := managedProjectRepositoryStore
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	t.Cleanup(func() { managedProjectRepositoryStore = originalProjects })

	now := time.Now().UTC()
	seed := func(slug, name string, tags []string) {
		t.Helper()
		project, err := projectRepository.Create(context.Background(), "owner-1", managedProjectInput{
			Slug: slug, Name: name, Summary: name + " 简介", Description: "# " + name + "\n\n正文。",
			Category: "Coding Agent", Tags: tags,
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
	}
	seed("tags-one", "标签项目一", []string{"Agent", "检索", "开源"})
	seed("tags-two", "标签项目二", []string{"Agent", "开源"})

	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"name":"Agent","count":2`) {
		t.Fatalf("Agent should count 2: %s", body)
	}
	if !strings.Contains(body, `"name":"检索","count":1`) {
		t.Fatalf("检索 should count 1: %s", body)
	}
}
