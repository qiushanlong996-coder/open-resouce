package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// 管理操作审计。
//
// 除既有的 auditAuth（slog）外，把关键管理动作（封禁/解封、下架、密钥签发/
// 撤销、项目审核）落到 admin_audit 表，供管理端查询近期操作。
// 写入是 best-effort：审计失败只记日志，绝不阻塞主动作。

type adminAuditEntry struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	ActorEmail string    `json:"actor_email"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

type adminAuditRepository interface {
	Record(ctx context.Context, entry adminAuditEntry) error
	List(ctx context.Context, limit int) ([]adminAuditEntry, error)
}

type memoryAdminAuditRepository struct {
	sync.RWMutex
	entries []adminAuditEntry
}

func newMemoryAdminAuditRepository() *memoryAdminAuditRepository {
	return &memoryAdminAuditRepository{entries: make([]adminAuditEntry, 0)}
}

func (repository *memoryAdminAuditRepository) Record(_ context.Context, entry adminAuditEntry) error {
	repository.Lock()
	defer repository.Unlock()
	// 头插，保持最近优先。
	repository.entries = append([]adminAuditEntry{entry}, repository.entries...)
	return nil
}

func (repository *memoryAdminAuditRepository) List(_ context.Context, limit int) ([]adminAuditEntry, error) {
	repository.RLock()
	defer repository.RUnlock()
	if limit <= 0 || limit > len(repository.entries) {
		limit = len(repository.entries)
	}
	result := make([]adminAuditEntry, limit)
	copy(result, repository.entries[:limit])
	return result, nil
}

type mysqlAdminAuditRepository struct{ db *sql.DB }

func newMySQLAdminAuditRepository(db *sql.DB) *mysqlAdminAuditRepository {
	return &mysqlAdminAuditRepository{db: db}
}

func (repository *mysqlAdminAuditRepository) Record(ctx context.Context, entry adminAuditEntry) error {
	var actor any
	if entry.ActorID != "" {
		actor = entry.ActorID
	}
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO admin_audit (id, actor_id, actor_email, action, target, detail)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, actor, entry.ActorEmail, entry.Action, entry.Target, entry.Detail)
	if err != nil {
		return fmt.Errorf("record admin audit: %w", err)
	}
	return nil
}

func (repository *mysqlAdminAuditRepository) List(ctx context.Context, limit int) ([]adminAuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, COALESCE(actor_id, ''), actor_email, action, target, detail, created_at
		 FROM admin_audit ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list admin audit: %w", err)
	}
	defer rows.Close()
	result := make([]adminAuditEntry, 0)
	for rows.Next() {
		var entry adminAuditEntry
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.ActorEmail, &entry.Action,
			&entry.Target, &entry.Detail, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan admin audit: %w", err)
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

var adminAuditRepositoryStore adminAuditRepository = newMemoryAdminAuditRepository()

var _ adminAuditRepository = (*memoryAdminAuditRepository)(nil)
var _ adminAuditRepository = (*mysqlAdminAuditRepository)(nil)

// recordAdminAudit 记录一次管理操作。best-effort：与 auditAuth（slog）并行，
// 失败只告警。detail/target 会被截断到列宽以内。
func recordAdminAudit(request *http.Request, admin authUser, action, target, detail string) {
	auditAuth(request, "admin_"+action, admin.Email, admin.ID)
	if adminAuditRepositoryStore == nil {
		return
	}
	entry := adminAuditEntry{
		ID: "audit-" + newRequestID(), ActorID: admin.ID, ActorEmail: admin.Email,
		Action: action, Target: truncateRunes(target, 191), Detail: truncateRunes(detail, 500),
		CreatedAt: time.Now().UTC(),
	}
	if err := adminAuditRepositoryStore.Record(request.Context(), entry); err != nil {
		slog.WarnContext(request.Context(), "record admin audit failed",
			"request_id", requestIDFromContext(request.Context()), "action", action, "error", err)
	}
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// adminAuditHandler 列出近期管理操作。
//
//	GET /api/v1/admin/audit?limit=100
func adminAuditHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	_ = admin
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	limit := 100
	if raw := request.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	entries, err := adminAuditRepositoryStore.List(request.Context(), limit)
	if err != nil {
		slog.ErrorContext(request.Context(), "list admin audit failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "审计服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": entries, "request_id": requestIDFromContext(request.Context())})
}
