package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func putAvatar(t *testing.T, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/auth/avatar", strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	return response
}

func TestSetAvatarAndRestoreDefault(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	cookie, user := registerTestUser(t, "avatar@example.com", "头像用户")
	key := "uploads/" + user.ID + "/2026/08/avatar.webp"
	response := putAvatar(t, cookie, `{"avatar":"`+key+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("set avatar status = %d: %s", response.Code, response.Body)
	}
	var body authResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Avatar != key {
		t.Fatalf("returned avatar = %q, want %q", body.Data.Avatar, key)
	}

	response = putAvatar(t, cookie, `{"avatar":""}`)
	if response.Code != http.StatusOK {
		t.Fatalf("restore avatar status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Avatar != "" {
		t.Fatalf("restored avatar = %q, want empty", body.Data.Avatar)
	}
}

func TestSetAvatarRejectsForeignAndNonImageObjects(t *testing.T) {
	originalAuth, originalLimiter := authRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() { authRepositoryStore, authRateLimiter = originalAuth, originalLimiter })

	cookie, user := registerTestUser(t, "avatar-invalid@example.com", "头像校验用户")
	tests := []string{
		`{"avatar":"uploads/user-someone-else/2026/08/avatar.png"}`,
		`{"avatar":"uploads/` + user.ID + `/2026/08/avatar.pdf"}`,
		`{"avatar":"uploads/` + user.ID + `/../avatar.png"}`,
	}
	for _, body := range tests {
		if response := putAvatar(t, cookie, body); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid avatar status = %d, want 422: %s", response.Code, response.Body)
		}
	}
}

func TestCommentListCarriesAuthorAvatar(t *testing.T) {
	originalAuth, originalComments, originalLimiter := authRepositoryStore, commentRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, commentRepositoryStore, authRateLimiter = originalAuth, originalComments, originalLimiter
	})

	cookie, user := registerTestUser(t, "avatar-comment@example.com", "评论头像用户")
	key := "uploads/" + user.ID + "/2026/08/avatar.png"
	if response := putAvatar(t, cookie, `{"avatar":"`+key+`"}`); response.Code != http.StatusOK {
		t.Fatalf("set avatar status = %d: %s", response.Code, response.Body)
	}

	createRequest := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"作者","body":"带头像的评论"}`))
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
	var list commentListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, comment := range list.Data {
		if comment.AuthorAvatar == key {
			return
		}
	}
	t.Fatalf("comment list did not carry author_avatar: %s", listResponse.Body)
}
