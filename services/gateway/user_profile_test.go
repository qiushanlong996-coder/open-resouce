package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserProfileReturnsPublishedProjectsAndStats(t *testing.T) {
	originalAuth, originalProjects, originalMetrics, originalLimiter :=
		authRepositoryStore, managedProjectRepositoryStore, projectMetricsRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	projectMetricsRepositoryStore = newMemoryProjectMetricsRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, managedProjectRepositoryStore, projectMetricsRepositoryStore, authRateLimiter =
			originalAuth, originalProjects, originalMetrics, originalLimiter
	})

	t.Setenv("ADMIN_EMAILS", "profile-admin@example.com")
	ownerCookie, owner := registerTestUser(t, "profile-owner@example.com", "资料作者")
	adminCookie, _ := registerTestUser(t, "profile-admin@example.com", "资料管理员")

	// 创建并发布一个项目（走完整审核流程）。
	body := `{"slug":"profile-demo","name":"Profile Demo","summary":"用于验证公开主页的示例项目",
		"description":"这是足够长的项目详细介绍，用于验证公开用户主页展示已发布项目的完整流程。",
		"category":"Coding Agent","tags":["Agent"],"tech_stack":["Go"],
		"license":"MIT","repository_url":"https://github.com/example/profile","current_version":"0.1.0"}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/author/projects", strings.NewReader(body))
	create.AddCookie(ownerCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create project status = %d: %s", createResponse.Code, createResponse.Body)
	}
	var created struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	submit := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", nil)
	submit.AddCookie(ownerCookie)
	newHandler().ServeHTTP(httptest.NewRecorder(), submit)

	approve := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/approve", nil)
	approve.AddCookie(adminCookie)
	approveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(approveResponse, approve)
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveResponse.Code, approveResponse.Body)
	}

	// 打一次浏览 beacon，profile 统计应反映出来。
	view := httptest.NewRequest(http.MethodPost, "/api/v1/projects/profile-demo/view", nil)
	newHandler().ServeHTTP(httptest.NewRecorder(), view)

	profile := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+owner.ID+"/profile", nil)
	profileResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(profileResponse, profile)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status = %d: %s", profileResponse.Code, profileResponse.Body)
	}
	var decoded publicUserProfileResponse
	if err := json.Unmarshal(profileResponse.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	data := decoded.Data
	if data.ID != owner.ID || data.DisplayName != "资料作者" {
		t.Fatalf("profile identity = %#v", data)
	}
	if data.Level < 1 {
		t.Fatalf("profile level = %d, want >= 1", data.Level)
	}
	if data.JoinedAt == "" {
		t.Fatal("profile joined_at must be set")
	}
	if data.Stats.ProjectsCount != 1 || len(data.Projects) != 1 {
		t.Fatalf("profile projects = %#v (stats %#v)", data.Projects, data.Stats)
	}
	if data.Projects[0].Slug != "profile-demo" {
		t.Fatalf("profile project slug = %q", data.Projects[0].Slug)
	}
	if data.Stats.TotalViews < 1 {
		t.Fatalf("profile total_views = %d, want >= 1", data.Stats.TotalViews)
	}

	// 绝不泄露邮箱或任何 PII。
	if strings.Contains(profileResponse.Body.String(), "profile-owner@example.com") {
		t.Fatal("profile response leaked email")
	}

	// 项目详情应带上 owner_id 与 author_name，供前端链接到作者主页。
	detail := httptest.NewRequest(http.MethodGet, "/api/v1/projects/profile-demo", nil)
	detailResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(detailResponse, detail)
	var detailDecoded projectDetailResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detailDecoded); err != nil {
		t.Fatal(err)
	}
	if detailDecoded.Data.OwnerID != owner.ID || detailDecoded.Data.AuthorName != "资料作者" {
		t.Fatalf("project detail author = owner_id %q author_name %q",
			detailDecoded.Data.OwnerID, detailDecoded.Data.AuthorName)
	}
}

func TestUserProfileUnknownUserReturns404(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-does-not-exist/profile", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown user profile status = %d, want 404", response.Code)
	}
}
