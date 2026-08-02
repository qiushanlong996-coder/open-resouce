package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// putAvatarFrame 发送头像框设置请求并返回响应记录器。
func putAvatarFrame(t *testing.T, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/auth/avatar-frame", strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	return response
}

func TestSetAvatarFramePreset(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	cookie, _ := registerTestUser(t, "frame-preset@example.com", "预设用户")
	response := putAvatarFrame(t, cookie, `{"frame":"zodiac-leo"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("set frame status = %d: %s", response.Code, response.Body)
	}
	var body authResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AvatarFrame != "zodiac-leo" {
		t.Fatalf("returned frame = %q, want zodiac-leo", body.Data.AvatarFrame)
	}

	// /auth/me 应回读持久化后的头像框。
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(cookie)
	meResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(meResponse, meRequest)
	var me authResponse
	if err := json.Unmarshal(meResponse.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Data.AvatarFrame != "zodiac-leo" {
		t.Fatalf("me frame = %q, want zodiac-leo", me.Data.AvatarFrame)
	}
}

func TestSetAvatarFrameInvalid(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	cookie, _ := registerTestUser(t, "frame-invalid@example.com", "非法用户")
	response := putAvatarFrame(t, cookie, `{"frame":"zodiac-nope"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid frame status = %d, want 422: %s", response.Code, response.Body)
	}
}

func TestSetAvatarFrameCustomNotOwned(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	cookie, _ := registerTestUser(t, "frame-custom@example.com", "自定义用户")
	// 对象键归属于其他用户，应被拒绝。
	response := putAvatarFrame(t, cookie, `{"frame":"custom:uploads/user-someone-else/2026/08/x.png"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("foreign custom key status = %d, want 422: %s", response.Code, response.Body)
	}
}

func TestSetAvatarFrameUnauthenticated(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	response := putAvatarFrame(t, nil, `{"frame":"zodiac-leo"}`)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", response.Code, response.Body)
	}
}

func TestCommentListCarriesAuthorFrame(t *testing.T) {
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

	cookie, _ := registerTestUser(t, "frame-comment@example.com", "评论头像框用户")
	if code := putAvatarFrame(t, cookie, `{"frame":"zodiac-aries"}`).Code; code != http.StatusOK {
		t.Fatalf("set frame status = %d", code)
	}

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"作者","body":"带头像框的评论"}`),
	)
	createRequest.AddCookie(cookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create comment status = %d: %s", createResponse.Code, createResponse.Body)
	}

	listRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments", nil)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list comments status = %d: %s", listResponse.Code, listResponse.Body)
	}
	var list commentListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, comment := range list.Data {
		if comment.AuthorFrame == "zodiac-aries" {
			found = true
		}
	}
	if !found {
		t.Fatalf("comment list did not carry author_frame: %s", listResponse.Body)
	}
}
