package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Open API 密钥。
//
// 只存储明文密钥的 SHA-256 摘要，明文仅在签发时返回一次。prefix 保留前缀
// 供管理端识别。撤销即写入 revoked_at，历史记录保留以便审计。
// GET /api/v1/open/projects 用 Authorization: Bearer <key> 校验密钥，返回
// 已发布项目列表，证明密钥链路可用。

const apiKeyPlaintextPrefix = "ork_"

type apiKey struct {
	ID        string     `json:"id"`
	OwnerID   string     `json:"owner_id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type apiKeyRepository interface {
	Create(ctx context.Context, key apiKey, keyHash string) error
	ListByOwner(ctx context.Context, ownerID string) ([]apiKey, error)
	Revoke(ctx context.Context, id, ownerID string, now time.Time) (bool, error)
	// FindActiveByHash 返回未撤销的密钥，供 Bearer 鉴权使用。
	FindActiveByHash(ctx context.Context, keyHash string) (apiKey, bool, error)
}

type memoryAPIKeyRepository struct {
	sync.RWMutex
	keys      map[string]apiKey // id -> key
	hashIndex map[string]string // keyHash -> id
}

func newMemoryAPIKeyRepository() *memoryAPIKeyRepository {
	return &memoryAPIKeyRepository{keys: make(map[string]apiKey), hashIndex: make(map[string]string)}
}

func (repository *memoryAPIKeyRepository) Create(_ context.Context, key apiKey, keyHash string) error {
	repository.Lock()
	defer repository.Unlock()
	repository.keys[key.ID] = key
	repository.hashIndex[keyHash] = key.ID
	return nil
}

func (repository *memoryAPIKeyRepository) ListByOwner(_ context.Context, ownerID string) ([]apiKey, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]apiKey, 0)
	for _, key := range repository.keys {
		if key.OwnerID == ownerID {
			result = append(result, key)
		}
	}
	sortAPIKeysByCreatedDesc(result)
	return result, nil
}

func (repository *memoryAPIKeyRepository) Revoke(
	_ context.Context, id, ownerID string, now time.Time,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	key, ok := repository.keys[id]
	if !ok || key.OwnerID != ownerID || key.RevokedAt != nil {
		return false, nil
	}
	revoked := now
	key.RevokedAt = &revoked
	repository.keys[id] = key
	return true, nil
}

func (repository *memoryAPIKeyRepository) FindActiveByHash(
	_ context.Context, keyHash string,
) (apiKey, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	id, ok := repository.hashIndex[keyHash]
	if !ok {
		return apiKey{}, false, nil
	}
	key := repository.keys[id]
	if key.RevokedAt != nil {
		return apiKey{}, false, nil
	}
	return key, true, nil
}

type mysqlAPIKeyRepository struct{ db *sql.DB }

func newMySQLAPIKeyRepository(db *sql.DB) *mysqlAPIKeyRepository {
	return &mysqlAPIKeyRepository{db: db}
}

func (repository *mysqlAPIKeyRepository) Create(ctx context.Context, key apiKey, keyHash string) error {
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, owner_id, name, key_hash, prefix) VALUES (?, ?, ?, ?, ?)`,
		key.ID, key.OwnerID, key.Name, keyHash, key.Prefix)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (repository *mysqlAPIKeyRepository) ListByOwner(ctx context.Context, ownerID string) ([]apiKey, error) {
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, owner_id, name, prefix, created_at, revoked_at FROM api_keys
		 WHERE owner_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	result := make([]apiKey, 0)
	for rows.Next() {
		var key apiKey
		if err := rows.Scan(&key.ID, &key.OwnerID, &key.Name, &key.Prefix, &key.CreatedAt, &key.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

func (repository *mysqlAPIKeyRepository) Revoke(
	ctx context.Context, id, ownerID string, now time.Time,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND owner_id = ? AND revoked_at IS NULL`,
		now, id, ownerID)
	if err != nil {
		return false, fmt.Errorf("revoke api key: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlAPIKeyRepository) FindActiveByHash(
	ctx context.Context, keyHash string,
) (apiKey, bool, error) {
	var key apiKey
	err := repository.db.QueryRowContext(ctx,
		`SELECT id, owner_id, name, prefix, created_at, revoked_at FROM api_keys
		 WHERE key_hash = ? AND revoked_at IS NULL`, keyHash).
		Scan(&key.ID, &key.OwnerID, &key.Name, &key.Prefix, &key.CreatedAt, &key.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return apiKey{}, false, nil
	}
	if err != nil {
		return apiKey{}, false, fmt.Errorf("find api key by hash: %w", err)
	}
	return key, true, nil
}

var apiKeyRepositoryStore apiKeyRepository = newMemoryAPIKeyRepository()

var _ apiKeyRepository = (*memoryAPIKeyRepository)(nil)
var _ apiKeyRepository = (*mysqlAPIKeyRepository)(nil)

func sortAPIKeysByCreatedDesc(keys []apiKey) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j].CreatedAt.After(keys[j-1].CreatedAt); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

// generateAPIKey 生成明文密钥、其 SHA-256 摘要与展示前缀。
func generateAPIKey() (plaintext, keyHash, prefix string, err error) {
	buffer := make([]byte, 24)
	if _, err = rand.Read(buffer); err != nil {
		return "", "", "", err
	}
	plaintext = apiKeyPlaintextPrefix + hex.EncodeToString(buffer)
	sum := sha256.Sum256([]byte(plaintext))
	keyHash = hex.EncodeToString(sum[:])
	prefix = plaintext[:12]
	return plaintext, keyHash, prefix, nil
}

// adminAPIKeysHandler 列出与签发密钥。
//
//	GET  /api/v1/admin/api-keys   列出当前管理员签发的密钥（不含明文）
//	POST /api/v1/admin/api-keys   签发新密钥（返回明文，仅此一次）
func adminAPIKeysHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		keys, err := apiKeyRepositoryStore.ListByOwner(request.Context(), admin.ID)
		if err != nil {
			slog.ErrorContext(request.Context(), "list api keys failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": keys, "request_id": requestIDFromContext(request.Context())})
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if request.Body != nil && request.ContentLength != 0 && decodeJSONBody(request, &input) != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "密钥数据格式不正确")
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len([]rune(input.Name)) > 120 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_api_key_name", "密钥名称不能为空且不超过 120 字")
			return
		}
		plaintext, keyHash, prefix, err := generateAPIKey()
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "key_generation_failed", "密钥生成失败，请重试")
			return
		}
		key := apiKey{
			ID: "apikey-" + newRequestID(), OwnerID: admin.ID, Name: input.Name,
			Prefix: prefix, CreatedAt: time.Now().UTC(),
		}
		if err := apiKeyRepositoryStore.Create(request.Context(), key, keyHash); err != nil {
			slog.ErrorContext(request.Context(), "create api key failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
			return
		}
		recordAdminAudit(request, admin, "api_key_issued", key.ID, input.Name)
		writeJSON(writer, http.StatusCreated, map[string]any{
			"data": map[string]any{"key": key, "plaintext": plaintext}, "request_id": requestIDFromContext(request.Context()),
		})
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

// adminAPIKeyHandler 撤销密钥。
//
//	DELETE /api/v1/admin/api-keys/{id}
func adminAPIKeyHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	keyID := request.PathValue("keyID")
	revoked, err := apiKeyRepositoryStore.Revoke(request.Context(), keyID, admin.ID, time.Now().UTC())
	if err != nil {
		slog.ErrorContext(request.Context(), "revoke api key failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
		return
	}
	if !revoked {
		writeAPIError(writer, request, http.StatusNotFound, "api_key_not_found", "密钥不存在或已撤销")
		return
	}
	recordAdminAudit(request, admin, "api_key_revoked", keyID, "")
	writer.WriteHeader(http.StatusNoContent)
}

// maxUserAPIKeys 限制单个用户可持有的有效密钥数量，避免滥用。
const maxUserAPIKeys = 20

// userAPIKeysHandler 让任意登录用户自助管理自己的 Open API 密钥。
//
//	GET  /api/v1/auth/api-keys   列出当前用户的密钥（不含明文）
//	POST /api/v1/auth/api-keys   为当前用户签发新密钥（返回明文，仅此一次）
func userAPIKeysHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		keys, err := apiKeyRepositoryStore.ListByOwner(request.Context(), user.ID)
		if err != nil {
			slog.ErrorContext(request.Context(), "list user api keys failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": keys, "request_id": requestIDFromContext(request.Context())})
	case http.MethodPost:
		if !ensureNotBanned(writer, request, user.ID) {
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		if request.Body != nil && request.ContentLength != 0 && decodeJSONBody(request, &input) != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "密钥数据格式不正确")
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len([]rune(input.Name)) > 120 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_api_key_name", "密钥名称不能为空且不超过 120 字")
			return
		}
		existing, err := apiKeyRepositoryStore.ListByOwner(request.Context(), user.ID)
		if err != nil {
			slog.ErrorContext(request.Context(), "list user api keys failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
			return
		}
		active := 0
		for _, key := range existing {
			if key.RevokedAt == nil {
				active++
			}
		}
		if active >= maxUserAPIKeys {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "api_key_limit_reached",
				fmt.Sprintf("最多只能持有 %d 个有效密钥，请先撤销不用的密钥", maxUserAPIKeys))
			return
		}
		plaintext, keyHash, prefix, err := generateAPIKey()
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "key_generation_failed", "密钥生成失败，请重试")
			return
		}
		key := apiKey{
			ID: "apikey-" + newRequestID(), OwnerID: user.ID, Name: input.Name,
			Prefix: prefix, CreatedAt: time.Now().UTC(),
		}
		if err := apiKeyRepositoryStore.Create(request.Context(), key, keyHash); err != nil {
			slog.ErrorContext(request.Context(), "create user api key failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
			return
		}
		writeJSON(writer, http.StatusCreated, map[string]any{
			"data": map[string]any{"key": key, "plaintext": plaintext}, "request_id": requestIDFromContext(request.Context()),
		})
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

// userAPIKeyHandler 撤销当前用户自己的密钥；撤销他人密钥返回 404 且不产生副作用。
//
//	DELETE /api/v1/auth/api-keys/{keyID}
func userAPIKeyHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	keyID := request.PathValue("keyID")
	revoked, err := apiKeyRepositoryStore.Revoke(request.Context(), keyID, user.ID, time.Now().UTC())
	if err != nil {
		slog.ErrorContext(request.Context(), "revoke user api key failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
		return
	}
	if !revoked {
		writeAPIError(writer, request, http.StatusNotFound, "api_key_not_found", "密钥不存在或已撤销")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// openProjectsHandler 是 Bearer 鉴权的开放接口入口。
//
//	GET  /api/v1/open/projects   列出已发布项目（证明密钥链路可用）
//	POST /api/v1/open/projects   以密钥所有者身份创建草稿项目（供外部 AI Agent 发布）
func openProjectsHandler(writer http.ResponseWriter, request *http.Request) {
	key, ok := requireAPIKey(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		projects, err := managedProjectRepositoryStore.ListPublished(request.Context())
		if err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": projects, "request_id": requestIDFromContext(request.Context())})
	case http.MethodPost:
		openCreateProject(writer, request, key.OwnerID)
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

// requireAPIKey 从 Authorization: Bearer 头解析并校验 Open API 密钥。
func requireAPIKey(writer http.ResponseWriter, request *http.Request) (apiKey, bool) {
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	const scheme = "Bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		writer.Header().Set("WWW-Authenticate", "Bearer")
		writeAPIError(writer, request, http.StatusUnauthorized, "api_key_required", "缺少有效的 API 密钥")
		return apiKey{}, false
	}
	plaintext := strings.TrimSpace(header[len(scheme):])
	sum := sha256.Sum256([]byte(plaintext))
	key, found, err := apiKeyRepositoryStore.FindActiveByHash(request.Context(), hex.EncodeToString(sum[:]))
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "密钥服务暂时不可用")
		return apiKey{}, false
	}
	if !found {
		writer.Header().Set("WWW-Authenticate", "Bearer")
		writeAPIError(writer, request, http.StatusUnauthorized, "api_key_invalid", "API 密钥无效或已撤销")
		return apiKey{}, false
	}
	return key, true
}
