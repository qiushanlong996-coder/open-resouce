package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorManagedProjectLifecycle(t *testing.T) {
	originalAuth, originalProjects, originalLimiter :=
		authRepositoryStore, managedProjectRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, managedProjectRepositoryStore, authRateLimiter =
			originalAuth, originalProjects, originalLimiter
	})
	ownerCookie, _ := registerTestUser(t, "project-owner@example.com", "项目作者")
	otherCookie, _ := registerTestUser(t, "project-other@example.com", "其他作者")
	adminCookie, _ := registerTestUser(t, "project-admin@example.com", "审核管理员")
	t.Setenv("ADMIN_EMAILS", "project-admin@example.com")

	body := `{"slug":"codex-demo","name":"Codex Demo","summary":"一个用于验证项目发布流程的示例项目",
		"description":"这是足够长的项目详细介绍，用于验证作者创建、编辑和提交审核的完整过程。",
		"category":"Coding Agent","tags":["Agent","测试"],"tech_stack":["Go","React"],
		"license":"MIT","repository_url":"https://github.com/example/demo","current_version":"0.1.0"}`
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
	if created.Data.Status != "draft" || created.Data.OwnerID == "" {
		t.Fatalf("created project = %#v", created.Data)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/author/projects", nil)
	list.AddCookie(ownerCookie)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), created.Data.ID) {
		t.Fatalf("list projects status = %d: %s", listResponse.Code, listResponse.Body)
	}

	crossUpdate := httptest.NewRequest(http.MethodPut,
		"/api/v1/author/projects/"+created.Data.ID, strings.NewReader(body))
	crossUpdate.AddCookie(otherCookie)
	crossResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(crossResponse, crossUpdate)
	if crossResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user update status = %d", crossResponse.Code)
	}

	submit := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", nil)
	submit.AddCookie(ownerCookie)
	submitResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusOK ||
		!strings.Contains(submitResponse.Body.String(), `"status":"pending_review"`) {
		t.Fatalf("submit status = %d: %s", submitResponse.Code, submitResponse.Body)
	}

	deleteSubmitted := httptest.NewRequest(http.MethodDelete,
		"/api/v1/author/projects/"+created.Data.ID, nil)
	deleteSubmitted.AddCookie(ownerCookie)
	deleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(deleteResponse, deleteSubmitted)
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("delete submitted project status = %d: %s", deleteResponse.Code, deleteResponse.Body)
	}

	nonAdminReviews := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reviews", nil)
	nonAdminReviews.AddCookie(otherCookie)
	nonAdminResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(nonAdminResponse, nonAdminReviews)
	if nonAdminResponse.Code != http.StatusForbidden {
		t.Fatalf("non-admin review list status = %d", nonAdminResponse.Code)
	}

	reviews := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reviews", nil)
	reviews.AddCookie(adminCookie)
	reviewsResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(reviewsResponse, reviews)
	if reviewsResponse.Code != http.StatusOK ||
		!strings.Contains(reviewsResponse.Body.String(), created.Data.ID) {
		t.Fatalf("review list status = %d: %s", reviewsResponse.Code, reviewsResponse.Body)
	}

	approve := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/approve", strings.NewReader(`{"reason":"内容完整"}`))
	approve.AddCookie(adminCookie)
	approveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(approveResponse, approve)
	if approveResponse.Code != http.StatusOK ||
		!strings.Contains(approveResponse.Body.String(), `"status":"published"`) {
		t.Fatalf("approve status = %d: %s", approveResponse.Code, approveResponse.Body)
	}

	publicList := httptest.NewRequest(http.MethodGet, "/api/v1/projects?q=Codex+Demo", nil)
	publicListResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(publicListResponse, publicList)
	if publicListResponse.Code != http.StatusOK ||
		!strings.Contains(publicListResponse.Body.String(), `"slug":"codex-demo"`) {
		t.Fatalf("public project list status = %d: %s", publicListResponse.Code, publicListResponse.Body)
	}

	publicDetail := httptest.NewRequest(http.MethodGet, "/api/v1/projects/codex-demo", nil)
	publicDetailResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(publicDetailResponse, publicDetail)
	if publicDetailResponse.Code != http.StatusOK {
		t.Fatalf("public project detail status = %d: %s", publicDetailResponse.Code, publicDetailResponse.Body)
	}
}
