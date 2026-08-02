package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFollowLifecycle(t *testing.T) {
	originalAuth := authRepositoryStore
	originalFollows := followRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	followRepositoryStore = newMemoryFollowRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		followRepositoryStore = originalFollows
		authRateLimiter = originalLimiter
	})
	cookie, _ := registerTestUser(t, "follow@example.com", "关注用户")

	setRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/follow", nil)
	setRequest.AddCookie(cookie)
	setResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(setResponse, setRequest)
	if setResponse.Code != http.StatusNoContent {
		t.Fatalf("set follow status = %d: %s", setResponse.Code, setResponse.Body)
	}

	// Repeating the operation is intentionally idempotent.
	secondSetResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(secondSetResponse, setRequest.Clone(setRequest.Context()))
	if secondSetResponse.Code != http.StatusNoContent {
		t.Fatalf("repeat set follow status = %d", secondSetResponse.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/follows", nil)
	listRequest.AddCookie(cookie)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	var listed followListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listResponse.Code != http.StatusOK || len(listed.Data) != 1 || listed.Data[0] != "atlas" {
		t.Fatalf("follows response = %d %#v", listResponse.Code, listed.Data)
	}

	// The follow list is owner-scoped: another user sees nothing.
	otherCookie, _ := registerTestUser(t, "follow-other@example.com", "其他用户")
	otherList := httptest.NewRequest(http.MethodGet, "/api/v1/follows", nil)
	otherList.AddCookie(otherCookie)
	otherResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(otherResponse, otherList)
	var otherListed followListResponse
	if err := json.Unmarshal(otherResponse.Body.Bytes(), &otherListed); err != nil {
		t.Fatal(err)
	}
	if len(otherListed.Data) != 0 {
		t.Fatalf("other user follows = %#v", otherListed.Data)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/atlas-agent/follow", nil)
	deleteRequest.AddCookie(cookie)
	deleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete follow status = %d", deleteResponse.Code)
	}

	// Deleting again is idempotent.
	secondDelete := httptest.NewRecorder()
	newHandler().ServeHTTP(secondDelete, deleteRequest.Clone(deleteRequest.Context()))
	if secondDelete.Code != http.StatusNoContent {
		t.Fatalf("repeat delete follow status = %d", secondDelete.Code)
	}

	listAfterDelete := httptest.NewRecorder()
	newHandler().ServeHTTP(listAfterDelete, listRequest)
	if err := json.Unmarshal(listAfterDelete.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 0 {
		t.Fatalf("follows remained after delete: %#v", listed.Data)
	}
}

func TestFollowsRequireAuthenticationAndKnownProject(t *testing.T) {
	originalAuth := authRepositoryStore
	originalFollows := followRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	followRepositoryStore = newMemoryFollowRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		followRepositoryStore = originalFollows
		authRateLimiter = originalLimiter
	})

	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/follows", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list status = %d", response.Code)
	}

	cookie, _ := registerTestUser(t, "follow-errors@example.com", "关注校验用户")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/not-found/follow", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown project status = %d", response.Code)
	}
}

func TestApprovingFollowedProjectNotifiesFollowers(t *testing.T) {
	setupNotificationTest(t)
	originalProjects := managedProjectRepositoryStore
	originalFollows := followRepositoryStore
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	followRepositoryStore = newMemoryFollowRepository()
	t.Cleanup(func() {
		managedProjectRepositoryStore = originalProjects
		followRepositoryStore = originalFollows
	})

	ownerCookie, _ := registerTestUser(t, "follow-owner@example.com", "被关注作者")
	adminCookie, admin := registerTestUser(t, "follow-admin@example.com", "关注审核管理员")
	followerCookie, follower := registerTestUser(t, "follow-fan@example.com", "关注者")
	t.Setenv("ADMIN_EMAILS", "follow-admin@example.com")

	body := `{"slug":"follow-demo","name":"Follow Demo","summary":"用于验证关注更新通知的示例项目摘要",
		"description":"这是足够长的项目详细介绍，用来验证关注者在项目重新发布时收到站内通知。",
		"category":"Coding Agent","tags":["Agent"],"tech_stack":["Go"],
		"license":"MIT","repository_url":"","current_version":"0.1.0"}`
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

	// Register the follower directly against the project id. The acting admin
	// follows it too, to prove the actor is excluded from their own update.
	// (A published project cannot re-enter review through the public flow, so the
	// followers are seeded here and a single approve exercises the notify hook.)
	for _, id := range []string{follower.ID, admin.ID} {
		if err := followRepositoryStore.SetFollow(context.Background(), id, created.Data.ID, true); err != nil {
			t.Fatalf("seed follow: %v", err)
		}
	}

	submit := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", nil)
	submit.AddCookie(ownerCookie)
	submitResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusOK {
		t.Fatalf("submit status = %d: %s", submitResponse.Code, submitResponse.Body)
	}
	approve := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/approve", nil)
	approve.AddCookie(adminCookie)
	approveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(approveResponse, approve)
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveResponse.Code, approveResponse.Body)
	}

	followerNotifications := listNotifications(t, followerCookie)
	updates := 0
	for _, entry := range followerNotifications.Data {
		if entry.Type != "project.updated" {
			continue
		}
		updates++
		if entry.ProjectSlug != "follow-demo" || !strings.Contains(entry.Title, "Follow Demo") {
			t.Fatalf("follower update notification = %#v", entry)
		}
	}
	if updates != 1 {
		t.Fatalf("follower project.updated count = %d (%#v)", updates, followerNotifications.Data)
	}

	// The acting admin follows the project but must not be notified of their own action.
	adminNotifications := listNotifications(t, adminCookie)
	for _, entry := range adminNotifications.Data {
		if entry.Type == "project.updated" {
			t.Fatalf("acting admin received project.updated: %#v", entry)
		}
	}
}
