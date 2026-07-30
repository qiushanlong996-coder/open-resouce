package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type recordingPasswordResetDelivery struct {
	urls []string
}

func (delivery *recordingPasswordResetDelivery) SendPasswordReset(
	_ context.Context, _ authUser, resetURL string,
) error {
	delivery.urls = append(delivery.urls, resetURL)
	return nil
}

func TestAuthLifecycleAndCommentOwnership(t *testing.T) {
	originalAuth := authRepositoryStore
	originalComments := commentRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		commentRepositoryStore = originalComments
	})

	ownerCookie, owner := registerTestUser(t, "owner@example.com", "评论作者")
	otherCookie, _ := registerTestUser(t, "other@example.com", "其他用户")

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(ownerCookie)
	meResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", meResponse.Code, meResponse.Body)
	}

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"伪造作者","body":"受保护评论"}`),
	)
	createRequest.AddCookie(ownerCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", createResponse.Code, createResponse.Body)
	}
	var created commentResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.AuthorID != owner.ID || created.Data.Author != owner.DisplayName {
		t.Fatalf("server did not bind comment author: %#v", created.Data)
	}

	path := "/api/v1/projects/atlas-agent/documents/quick-start/comments/" + created.Data.ID
	forbiddenRequest := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"body":"越权修改"}`))
	forbiddenRequest.AddCookie(otherCookie)
	forbiddenResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(forbiddenResponse, forbiddenRequest)
	if forbiddenResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user edit status = %d, want 404: %s", forbiddenResponse.Code, forbiddenResponse.Body)
	}

	editRequest := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"body":"作者修改"}`))
	editRequest.AddCookie(ownerCookie)
	editResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(editResponse, editRequest)
	if editResponse.Code != http.StatusOK {
		t.Fatalf("owner edit status = %d: %s", editResponse.Code, editResponse.Body)
	}

	forbiddenDeleteRequest := httptest.NewRequest(http.MethodDelete, path, nil)
	forbiddenDeleteRequest.AddCookie(otherCookie)
	forbiddenDeleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(forbiddenDeleteResponse, forbiddenDeleteRequest)
	if forbiddenDeleteResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user delete status = %d, want 404: %s", forbiddenDeleteResponse.Code, forbiddenDeleteResponse.Body)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, path, nil)
	deleteRequest.AddCookie(ownerCookie)
	deleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("owner delete status = %d: %s", deleteResponse.Code, deleteResponse.Body)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.AddCookie(ownerCookie)
	logoutResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d: %s", logoutResponse.Code, logoutResponse.Body)
	}
}

func TestAuthenticationRequiredForCommentMutation(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"匿名","body":"不应创建"}`),
	)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", response.Code, response.Body)
	}
}

func TestLogoutAllRevokesEverySession(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	firstCookie, _ := registerTestUser(t, "sessions@example.com", "会话用户")
	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(`{"email":"sessions@example.com","password":"test-password-123"}`),
	)
	loginResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", loginResponse.Code, loginResponse.Body)
	}
	secondCookie := loginResponse.Result().Cookies()[0]

	sessionsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	sessionsRequest.AddCookie(firstCookie)
	sessionsResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(sessionsResponse, sessionsRequest)
	if sessionsResponse.Code != http.StatusOK {
		t.Fatalf("list sessions status = %d: %s", sessionsResponse.Code, sessionsResponse.Body)
	}
	var sessions sessionListResponse
	if err := json.Unmarshal(sessionsResponse.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	currentCount := 0
	for _, session := range sessions.Data {
		if session.Current {
			currentCount++
		}
	}
	if len(sessions.Data) != 2 || currentCount != 1 {
		t.Fatalf("sessions = %#v, want two with one current", sessions.Data)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout-all", nil)
	logoutRequest.AddCookie(firstCookie)
	logoutResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout all status = %d: %s", logoutResponse.Code, logoutResponse.Body)
	}

	for index, cookie := range []*http.Cookie{firstCookie, secondCookie} {
		meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		meRequest.AddCookie(cookie)
		meResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(meResponse, meRequest)
		if meResponse.Code != http.StatusUnauthorized {
			t.Fatalf("session %d remained active: %d", index+1, meResponse.Code)
		}
	}
}

func TestRevokeIndividualSession(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	firstCookie, _ := registerTestUser(t, "revoke-session@example.com", "会话管理用户")
	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(`{"email":"revoke-session@example.com","password":"test-password-123"}`),
	)
	loginResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", loginResponse.Code, loginResponse.Body)
	}
	secondCookie := loginResponse.Result().Cookies()[0]

	sessionsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	sessionsRequest.AddCookie(firstCookie)
	sessionsResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(sessionsResponse, sessionsRequest)
	var sessions sessionListResponse
	if err := json.Unmarshal(sessionsResponse.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	var otherSessionID string
	for _, session := range sessions.Data {
		if !session.Current {
			otherSessionID = session.ID
		}
	}
	if otherSessionID == "" {
		t.Fatalf("no non-current session in %#v", sessions.Data)
	}

	revokeRequest := httptest.NewRequest(
		http.MethodDelete, "/api/v1/auth/sessions/"+otherSessionID, nil,
	)
	revokeRequest.AddCookie(firstCookie)
	revokeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(revokeResponse, revokeRequest)
	if revokeResponse.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d: %s", revokeResponse.Code, revokeResponse.Body)
	}

	for index, expectation := range []struct {
		cookie *http.Cookie
		status int
	}{{firstCookie, http.StatusOK}, {secondCookie, http.StatusUnauthorized}} {
		meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		meRequest.AddCookie(expectation.cookie)
		meResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(meResponse, meRequest)
		if meResponse.Code != expectation.status {
			t.Fatalf("session %d status = %d, want %d", index+1, meResponse.Code, expectation.status)
		}
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	firstCookie, _ := registerTestUser(t, "password-change@example.com", "密码修改用户")
	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(`{"email":"password-change@example.com","password":"test-password-123"}`),
	)
	loginResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("second login status = %d: %s", loginResponse.Code, loginResponse.Body)
	}
	secondCookie := loginResponse.Result().Cookies()[0]

	wrongRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/auth/password",
		strings.NewReader(`{"current_password":"wrong-password","new_password":"new-password-456"}`),
	)
	wrongRequest.AddCookie(firstCookie)
	wrongResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(wrongResponse, wrongRequest)
	if wrongResponse.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d: %s", wrongResponse.Code, wrongResponse.Body)
	}

	changeRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/auth/password",
		strings.NewReader(`{"current_password":"test-password-123","new_password":"new-password-456"}`),
	)
	changeRequest.AddCookie(firstCookie)
	changeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(changeResponse, changeRequest)
	if changeResponse.Code != http.StatusNoContent {
		t.Fatalf("password change status = %d: %s", changeResponse.Code, changeResponse.Body)
	}

	for index, expectation := range []struct {
		cookie *http.Cookie
		status int
	}{{firstCookie, http.StatusOK}, {secondCookie, http.StatusUnauthorized}} {
		meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		meRequest.AddCookie(expectation.cookie)
		meResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(meResponse, meRequest)
		if meResponse.Code != expectation.status {
			t.Fatalf("session %d status = %d, want %d", index+1, meResponse.Code, expectation.status)
		}
	}

	for _, expectation := range []struct {
		password string
		status   int
	}{{"test-password-123", http.StatusUnauthorized}, {"new-password-456", http.StatusOK}} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/auth/login",
			strings.NewReader(`{"email":"password-change@example.com","password":"`+expectation.password+`"}`),
		)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != expectation.status {
			t.Fatalf("login with %q status = %d, want %d", expectation.password, response.Code, expectation.status)
		}
	}
}

func TestUpdateCurrentUserDisplayName(t *testing.T) {
	originalAuth := authRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		authRateLimiter = originalLimiter
	})

	cookie, _ := registerTestUser(t, "profile@example.com", "原昵称")
	request := httptest.NewRequest(
		http.MethodPatch, "/api/v1/auth/me", strings.NewReader(`{"display_name":"  新昵称  "}`),
	)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("profile update status = %d: %s", response.Code, response.Body)
	}
	var updated authResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.DisplayName != "新昵称" {
		t.Fatalf("updated display name = %q", updated.Data.DisplayName)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(cookie)
	meResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(meResponse, meRequest)
	if err := json.Unmarshal(meResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.DisplayName != "新昵称" {
		t.Fatalf("persisted display name = %q", updated.Data.DisplayName)
	}
}

func TestPasswordResetLifecycle(t *testing.T) {
	originalAuth := authRepositoryStore
	originalLimiter := authRateLimiter
	originalDelivery := passwordResetDeliveryStore
	delivery := &recordingPasswordResetDelivery{}
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	passwordResetDeliveryStore = delivery
	t.Setenv("PUBLIC_BASE_URL", "https://example.com")
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		authRateLimiter = originalLimiter
		passwordResetDeliveryStore = originalDelivery
	})

	firstCookie, _ := registerTestUser(t, "reset@example.com", "重置密码用户")
	loginRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"reset@example.com","password":"test-password-123"}`),
	)
	loginResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("second login status = %d", loginResponse.Code)
	}
	secondCookie := loginResponse.Result().Cookies()[0]

	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/auth/password-reset/request",
		strings.NewReader(`{"email":"reset@example.com"}`),
	)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(delivery.urls) != 1 {
		t.Fatalf("reset request = %d urls=%#v", response.Code, delivery.urls)
	}
	resetURL, err := url.Parse(delivery.urls[0])
	if err != nil {
		t.Fatal(err)
	}
	token := resetURL.Query().Get("reset_token")
	if token == "" {
		t.Fatal("reset token missing from delivery URL")
	}

	confirmRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/auth/password-reset/confirm",
		strings.NewReader(`{"token":"`+token+`","new_password":"reset-password-456"}`),
	)
	confirmResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusNoContent {
		t.Fatalf("reset confirmation = %d: %s", confirmResponse.Code, confirmResponse.Body)
	}

	reuseRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/auth/password-reset/confirm",
		strings.NewReader(`{"token":"`+token+`","new_password":"reset-password-456"}`),
	)
	reuseResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(reuseResponse, reuseRequest)
	if reuseResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reused token status = %d", reuseResponse.Code)
	}
	for index, cookie := range []*http.Cookie{firstCookie, secondCookie} {
		meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		meRequest.AddCookie(cookie)
		meResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(meResponse, meRequest)
		if meResponse.Code != http.StatusUnauthorized {
			t.Fatalf("session %d remained after reset", index+1)
		}
	}

	unknownRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/auth/password-reset/request",
		strings.NewReader(`{"email":"unknown@example.com"}`),
	)
	unknownResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(unknownResponse, unknownRequest)
	if unknownResponse.Code != http.StatusAccepted || len(delivery.urls) != 1 {
		t.Fatalf("unknown reset leaked account state: %d urls=%d", unknownResponse.Code, len(delivery.urls))
	}
}

func TestPasswordHash(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if verifyPassword(encoded, "wrong password") {
		t.Fatal("invalid password accepted")
	}
}

func TestFixedWindowLimiter(t *testing.T) {
	limiter := newFixedWindowLimiter()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	for attempt := 1; attempt <= 2; attempt++ {
		if allowed, _, _ := limiter.Allow(context.Background(), "login:test", 2, time.Minute, now); !allowed {
			t.Fatalf("attempt %d was unexpectedly limited", attempt)
		}
	}
	if allowed, retry, _ := limiter.Allow(context.Background(), "login:test", 2, time.Minute, now); allowed || retry != time.Minute {
		t.Fatalf("third attempt = allowed %v, retry %s", allowed, retry)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "login:test", 2, time.Minute, now.Add(time.Minute)); !allowed {
		t.Fatal("new window did not reset")
	}
}

func TestRequestClientIP(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.4, 127.0.0.1")
	if got := requestClientIP(request); got != "203.0.113.4" {
		t.Fatalf("client IP = %q", got)
	}
}

func TestOAuthStartBuildsSecureProviderRedirect(t *testing.T) {
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET", "client-secret")
	t.Setenv("PUBLIC_BASE_URL", "https://103.236.98.166:8443")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/start", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, "https://github.com/login/oauth/authorize?") ||
		!strings.Contains(location, "client_id=client-id") ||
		!strings.Contains(location, "redirect_uri=https%3A%2F%2F103.236.98.166%3A8443") {
		t.Fatalf("unexpected OAuth redirect: %s", location)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("unexpected OAuth state cookie: %#v", cookies)
	}
}

func registerTestUser(t *testing.T, email, displayName string) (*http.Cookie, authUser) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/register",
		strings.NewReader(`{"email":"`+email+`","display_name":"`+displayName+`","password":"test-password-123"}`),
	)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d: %s", response.Code, response.Body)
	}
	var body authResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	result := response.Result()
	cookies := result.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	return cookies[0], body.Data
}
