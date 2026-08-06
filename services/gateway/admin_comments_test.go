package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminCommentsModeration(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "comments-admin@example.com")
	originalAuth := authRepositoryStore
	originalComments := commentRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		commentRepositoryStore = originalComments
		authRateLimiter = originalLimiter
	})

	adminCookie, _ := registerTestUser(t, "comments-admin@example.com", "管理")
	userCookie, _ := registerTestUser(t, "comments-user@example.com", "普通用户")

	t.Run("匿名被拒", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/comments", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("普通用户被拒", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/comments", nil)
		request.AddCookie(userCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("管理员列表", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/comments", nil)
		request.AddCookie(adminCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if !strings.Contains(response.Body.String(), `"id":"comment-atlas-001"`) {
			t.Fatalf("body = %s", response.Body)
		}
	})

	t.Run("隐藏与恢复", func(t *testing.T) {
		hide := httptest.NewRequest(http.MethodPost, "/api/v1/admin/comments/comment-atlas-001/hide", nil)
		hide.AddCookie(adminCookie)
		hideResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(hideResponse, hide)
		if hideResponse.Code != http.StatusOK ||
			!strings.Contains(hideResponse.Body.String(), `"hidden":true`) {
			t.Fatalf("hide = %d: %s", hideResponse.Code, hideResponse.Body)
		}

		hidden := httptest.NewRequest(http.MethodGet, "/api/v1/admin/comments?status=hidden", nil)
		hidden.AddCookie(adminCookie)
		hiddenResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(hiddenResponse, hidden)
		if hiddenResponse.Code != http.StatusOK ||
			!strings.Contains(hiddenResponse.Body.String(), `"id":"comment-atlas-001"`) {
			t.Fatalf("hidden list = %d: %s", hiddenResponse.Code, hiddenResponse.Body)
		}

		all := httptest.NewRequest(http.MethodGet, "/api/v1/admin/comments", nil)
		all.AddCookie(adminCookie)
		allResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(allResponse, all)
		if strings.Contains(allResponse.Body.String(), `"id":"comment-atlas-001"`) {
			t.Fatalf("hidden comment should not appear in all list: %s", allResponse.Body)
		}

		restore := httptest.NewRequest(http.MethodPost, "/api/v1/admin/comments/comment-atlas-001/restore", nil)
		restore.AddCookie(adminCookie)
		restoreResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(restoreResponse, restore)
		if restoreResponse.Code != http.StatusOK {
			t.Fatalf("restore = %d: %s", restoreResponse.Code, restoreResponse.Body)
		}
	})

	t.Run("隐藏不存在的评论返回404", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/comments/missing/hide", nil)
		request.AddCookie(adminCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})
}
