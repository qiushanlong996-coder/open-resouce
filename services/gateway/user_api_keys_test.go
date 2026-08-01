package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupUserAPIKeysTest 重置用户自助密钥所依赖的内存仓库。
func setupUserAPIKeysTest(t *testing.T) {
	t.Helper()
	originalAuth := authRepositoryStore
	originalKeys := apiKeyRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	apiKeyRepositoryStore = newMemoryAPIKeyRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		apiKeyRepositoryStore = originalKeys
		authRateLimiter = originalLimiter
	})
}

func TestUserAPIKeysLifecycle(t *testing.T) {
	setupUserAPIKeysTest(t)
	userCookie, _ := registerTestUser(t, "keys-user@example.com", "密钥用户")

	// 未登录不可访问（401）。
	anon := httptest.NewRequest(http.MethodGet, "/api/v1/auth/api-keys", nil)
	anonResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(anonResponse, anon)
	if anonResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list status = %d, want 401", anonResponse.Code)
	}

	// 创建密钥，返回明文一次。
	issue := doAdminRequest(t, http.MethodPost, "/api/v1/auth/api-keys", `{"name":"我的密钥"}`, userCookie)
	if issue.Code != http.StatusCreated {
		t.Fatalf("issue key status = %d: %s", issue.Code, issue.Body)
	}
	var issued struct {
		Data struct {
			Key       apiKey `json:"key"`
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Data.Plaintext == "" || issued.Data.Key.ID == "" {
		t.Fatalf("issued key missing plaintext/id: %s", issue.Body)
	}

	// 该明文密钥可用于 Open API（Bearer），以本人身份行事。
	openReq := httptest.NewRequest(http.MethodGet, "/api/v1/open/projects", nil)
	openReq.Header.Set("Authorization", "Bearer "+issued.Data.Plaintext)
	openResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(openResponse, openReq)
	if openResponse.Code != http.StatusOK {
		t.Fatalf("open with user key status = %d: %s", openResponse.Code, openResponse.Body)
	}

	// 列表只含本人密钥，且不回传明文。
	list := doAdminRequest(t, http.MethodGet, "/api/v1/auth/api-keys", "", userCookie)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body)
	}
	if strings.Contains(list.Body.String(), issued.Data.Plaintext) {
		t.Fatalf("api key list leaked plaintext: %s", list.Body)
	}
	var listBody struct {
		Data []apiKey `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Data) != 1 || listBody.Data[0].ID != issued.Data.Key.ID {
		t.Fatalf("list should contain only the user's own key: %s", list.Body)
	}

	// 撤销后 Open API 401。
	revoke := doAdminRequest(t, http.MethodDelete, "/api/v1/auth/api-keys/"+issued.Data.Key.ID, "", userCookie)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d: %s", revoke.Code, revoke.Body)
	}
	revoked := httptest.NewRequest(http.MethodGet, "/api/v1/open/projects", nil)
	revoked.Header.Set("Authorization", "Bearer "+issued.Data.Plaintext)
	revokedResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(revokedResponse, revoked)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("open with revoked key status = %d, want 401", revokedResponse.Code)
	}
}

func TestUserAPIKeysAreOwnerScoped(t *testing.T) {
	setupUserAPIKeysTest(t)
	userA, _ := registerTestUser(t, "keys-a@example.com", "用户A")
	userB, _ := registerTestUser(t, "keys-b@example.com", "用户B")

	issue := doAdminRequest(t, http.MethodPost, "/api/v1/auth/api-keys", `{"name":"A 的密钥"}`, userA)
	if issue.Code != http.StatusCreated {
		t.Fatalf("issue key status = %d: %s", issue.Code, issue.Body)
	}
	var issued struct {
		Data struct {
			Key apiKey `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}

	// 用户 B 看不到用户 A 的密钥。
	listB := doAdminRequest(t, http.MethodGet, "/api/v1/auth/api-keys", "", userB)
	if listB.Code != http.StatusOK {
		t.Fatalf("list B status = %d: %s", listB.Code, listB.Body)
	}
	if strings.Contains(listB.Body.String(), issued.Data.Key.ID) {
		t.Fatalf("user B can see user A's key: %s", listB.Body)
	}

	// 用户 B 撤销用户 A 的密钥应 404 且不产生副作用。
	revokeB := doAdminRequest(t, http.MethodDelete, "/api/v1/auth/api-keys/"+issued.Data.Key.ID, "", userB)
	if revokeB.Code != http.StatusNotFound {
		t.Fatalf("user B revoke A's key status = %d, want 404", revokeB.Code)
	}

	// 用户 A 的密钥仍然有效（未被 B 撤销）。
	listA := doAdminRequest(t, http.MethodGet, "/api/v1/auth/api-keys", "", userA)
	var listABody struct {
		Data []apiKey `json:"data"`
	}
	if err := json.Unmarshal(listA.Body.Bytes(), &listABody); err != nil {
		t.Fatal(err)
	}
	if len(listABody.Data) != 1 || listABody.Data[0].RevokedAt != nil {
		t.Fatalf("user A's key should remain active: %s", listA.Body)
	}
}
