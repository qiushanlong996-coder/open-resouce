package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// issueTestAPIKey 为给定所有者签发一枚测试密钥，返回明文（用于 Bearer 头）。
func issueTestAPIKey(t *testing.T, ownerID string) string {
	t.Helper()
	plaintext, keyHash, prefix, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	key := apiKey{
		ID: "apikey-" + newRequestID(), OwnerID: ownerID, Name: "agent",
		Prefix: prefix, CreatedAt: time.Now().UTC(),
	}
	if err := apiKeyRepositoryStore.Create(context.Background(), key, keyHash); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return plaintext
}

const openProjectBody = `{"slug":"agent-demo","name":"Agent Demo",
	"summary":"一个由外部 Agent 通过开放接口发布的示例项目",
	"description":"这是足够长的项目详细介绍，用于验证密钥所有者经开放写接口创建草稿并提交审核的完整流程。",
	"category":"Coding Agent","tags":["Agent"],"tech_stack":["Go"],
	"license":"MIT","repository_url":"https://github.com/example/agent","current_version":"0.1.0"}`

func TestOpenWriteAPIPublishFlow(t *testing.T) {
	originalAuth, originalProjects, originalKeys, originalBans, originalLimiter :=
		authRepositoryStore, managedProjectRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	apiKeyRepositoryStore = newMemoryAPIKeyRepository()
	banRepositoryStore = newMemoryBanRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, managedProjectRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, authRateLimiter =
			originalAuth, originalProjects, originalKeys, originalBans, originalLimiter
	})

	_, owner := registerTestUser(t, "agent-owner@example.com", "密钥所有者")
	plaintext := issueTestAPIKey(t, owner.ID)

	// 1. 创建草稿：应归属于密钥所有者，状态为 draft。
	create := httptest.NewRequest(http.MethodPost, "/api/v1/open/projects", strings.NewReader(openProjectBody))
	create.Header.Set("Authorization", "Bearer "+plaintext)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("open create status = %d: %s", createResponse.Code, createResponse.Body)
	}
	var created struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Status != "draft" || created.Data.OwnerID != owner.ID {
		t.Fatalf("created project = %#v (want draft owned by %s)", created.Data, owner.ID)
	}

	// 2. 提交审核：状态应转为 pending_review，仍需管理员放行（不自动发布）。
	submit := httptest.NewRequest(http.MethodPost,
		"/api/v1/open/projects/"+created.Data.ID+"/submit", nil)
	submit.Header.Set("Authorization", "Bearer "+plaintext)
	submitResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusOK ||
		!strings.Contains(submitResponse.Body.String(), `"status":"pending_review"`) {
		t.Fatalf("open submit status = %d: %s", submitResponse.Code, submitResponse.Body)
	}
}

func TestOpenWriteAPIAuthAndValidation(t *testing.T) {
	originalAuth, originalProjects, originalKeys, originalBans, originalLimiter :=
		authRepositoryStore, managedProjectRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	apiKeyRepositoryStore = newMemoryAPIKeyRepository()
	banRepositoryStore = newMemoryBanRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, managedProjectRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, authRateLimiter =
			originalAuth, originalProjects, originalKeys, originalBans, originalLimiter
	})

	_, owner := registerTestUser(t, "agent-owner2@example.com", "密钥所有者")
	plaintext := issueTestAPIKey(t, owner.ID)

	// 缺少密钥 -> 401。
	missing := httptest.NewRequest(http.MethodPost, "/api/v1/open/projects", strings.NewReader(openProjectBody))
	missingResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d: %s", missingResponse.Code, missingResponse.Body)
	}

	// 无效密钥 -> 401。
	invalidKey := httptest.NewRequest(http.MethodPost, "/api/v1/open/projects", strings.NewReader(openProjectBody))
	invalidKey.Header.Set("Authorization", "Bearer ork_notarealkey")
	invalidKeyResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(invalidKeyResponse, invalidKey)
	if invalidKeyResponse.Code != http.StatusUnauthorized {
		t.Fatalf("invalid key status = %d: %s", invalidKeyResponse.Code, invalidKeyResponse.Body)
	}

	// 输入非法（slug/字段不合规）-> 422。
	badInput := httptest.NewRequest(http.MethodPost, "/api/v1/open/projects",
		strings.NewReader(`{"slug":"Bad Slug","name":"x"}`))
	badInput.Header.Set("Authorization", "Bearer "+plaintext)
	badInputResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(badInputResponse, badInput)
	if badInputResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad input status = %d: %s", badInputResponse.Code, badInputResponse.Body)
	}

	// 被封禁的所有者 -> 403。
	if err := banRepositoryStore.Ban(context.Background(), owner.ID, "admin", "spam", time.Now().UTC()); err != nil {
		t.Fatalf("ban owner: %v", err)
	}
	banned := httptest.NewRequest(http.MethodPost, "/api/v1/open/projects", strings.NewReader(openProjectBody))
	banned.Header.Set("Authorization", "Bearer "+plaintext)
	bannedResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(bannedResponse, banned)
	if bannedResponse.Code != http.StatusForbidden {
		t.Fatalf("banned owner status = %d: %s", bannedResponse.Code, bannedResponse.Body)
	}
}

func TestOpenWriteAPIPresign(t *testing.T) {
	originalAuth, originalKeys, originalBans, originalStorage, originalLimiter :=
		authRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, objectStorageStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	apiKeyRepositoryStore = newMemoryAPIKeyRepository()
	banRepositoryStore = newMemoryBanRepository()
	storage := &fakeObjectStorage{}
	objectStorageStore = storage
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, apiKeyRepositoryStore, banRepositoryStore, objectStorageStore, authRateLimiter =
			originalAuth, originalKeys, originalBans, originalStorage, originalLimiter
	})

	_, owner := registerTestUser(t, "agent-presign@example.com", "密钥所有者")
	plaintext := issueTestAPIKey(t, owner.ID)

	presign := httptest.NewRequest(http.MethodPost, "/api/v1/open/files/presign",
		strings.NewReader(`{"filename":"parsed.zip","content_type":"application/zip","size":4096,"kind":"code"}`))
	presign.Header.Set("Authorization", "Bearer "+plaintext)
	presignResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(presignResponse, presign)
	if presignResponse.Code != http.StatusOK {
		t.Fatalf("open presign status = %d: %s", presignResponse.Code, presignResponse.Body)
	}
	// 签发的对象键必须落在密钥所有者的命名空间下，才能通过项目文件归属校验。
	if !strings.HasPrefix(storage.key, "uploads/"+owner.ID+"/") || storage.size != 4096 {
		t.Fatalf("presign object = %#v (want owner %s namespace)", storage, owner.ID)
	}

	// 类型不合规 -> 422。
	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/open/files/presign",
		strings.NewReader(`{"filename":"malware.exe","content_type":"application/zip","size":10,"kind":"code"}`))
	invalid.Header.Set("Authorization", "Bearer "+plaintext)
	invalidResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid presign status = %d: %s", invalidResponse.Code, invalidResponse.Body)
	}
}
