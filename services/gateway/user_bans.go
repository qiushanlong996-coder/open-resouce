package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// 用户封禁（黑名单）。
//
// 被封禁的用户仍可浏览/阅读，但所有需要登录的写操作（评论、回复、点赞、
// 收藏、分享、创建项目）都会被 ensureNotBanned 拦截并返回 403。
// 以 user_id 为主键，封禁/解封天然幂等。

type userBan struct {
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"`
	BannedBy  string    `json:"banned_by"`
	CreatedAt time.Time `json:"created_at"`
}

type banRepository interface {
	Ban(ctx context.Context, userID, bannedBy, reason string, now time.Time) error
	Unban(ctx context.Context, userID string) (bool, error)
	IsBanned(ctx context.Context, userID string) (bool, error)
	// BannedSet 批量返回给定用户中处于封禁状态的集合，用于列表叠加展示。
	BannedSet(ctx context.Context, userIDs []string) (map[string]bool, error)
	// CountBanned 返回当前处于封禁状态的用户总数，供管理概览统计。
	CountBanned(ctx context.Context) (int, error)
}

type memoryBanRepository struct {
	sync.RWMutex
	bans map[string]userBan
}

func newMemoryBanRepository() *memoryBanRepository {
	return &memoryBanRepository{bans: make(map[string]userBan)}
}

func (repository *memoryBanRepository) Ban(
	_ context.Context, userID, bannedBy, reason string, now time.Time,
) error {
	repository.Lock()
	defer repository.Unlock()
	repository.bans[userID] = userBan{UserID: userID, Reason: reason, BannedBy: bannedBy, CreatedAt: now}
	return nil
}

func (repository *memoryBanRepository) Unban(_ context.Context, userID string) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	if _, ok := repository.bans[userID]; !ok {
		return false, nil
	}
	delete(repository.bans, userID)
	return true, nil
}

func (repository *memoryBanRepository) IsBanned(_ context.Context, userID string) (bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	_, ok := repository.bans[userID]
	return ok, nil
}

func (repository *memoryBanRepository) BannedSet(
	_ context.Context, userIDs []string,
) (map[string]bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make(map[string]bool)
	for _, id := range userIDs {
		if _, ok := repository.bans[id]; ok {
			result[id] = true
		}
	}
	return result, nil
}

func (repository *memoryBanRepository) CountBanned(_ context.Context) (int, error) {
	repository.RLock()
	defer repository.RUnlock()
	return len(repository.bans), nil
}

type mysqlBanRepository struct{ db *sql.DB }

func newMySQLBanRepository(db *sql.DB) *mysqlBanRepository {
	return &mysqlBanRepository{db: db}
}

func (repository *mysqlBanRepository) Ban(
	ctx context.Context, userID, bannedBy, reason string, now time.Time,
) error {
	var admin any
	if bannedBy != "" {
		admin = bannedBy
	}
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO user_bans (user_id, reason, banned_by, created_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE reason = VALUES(reason), banned_by = VALUES(banned_by), created_at = VALUES(created_at)`,
		userID, reason, admin, now)
	if err != nil {
		return fmt.Errorf("ban user: %w", err)
	}
	return nil
}

func (repository *mysqlBanRepository) Unban(ctx context.Context, userID string) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `DELETE FROM user_bans WHERE user_id = ?`, userID)
	if err != nil {
		return false, fmt.Errorf("unban user: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlBanRepository) IsBanned(ctx context.Context, userID string) (bool, error) {
	var exists int
	err := repository.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_bans WHERE user_id = ?`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check user ban: %w", err)
	}
	return true, nil
}

func (repository *mysqlBanRepository) BannedSet(
	ctx context.Context, userIDs []string,
) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(userIDs) == 0 {
		return result, nil
	}
	placeholders, arguments := sqlInPlaceholders(userIDs)
	rows, err := repository.db.QueryContext(ctx,
		`SELECT user_id FROM user_bans WHERE user_id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query banned users: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan banned user: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}

func (repository *mysqlBanRepository) CountBanned(ctx context.Context) (int, error) {
	var count int
	if err := repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_bans`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count banned users: %w", err)
	}
	return count, nil
}

var banRepositoryStore banRepository = newMemoryBanRepository()

var _ banRepository = (*memoryBanRepository)(nil)
var _ banRepository = (*mysqlBanRepository)(nil)

// ensureNotBanned 是写操作的共享闸门：被封禁用户返回 403 并终止请求。
// 检查失败（仓库错误）时放行——安全策略不应因基础设施抖动而拒绝正常用户。
// 匿名或未接仓库时直接放行（登录校验由 requireCurrentUser 负责）。
func ensureNotBanned(writer http.ResponseWriter, request *http.Request, userID string) bool {
	if userID == "" || banRepositoryStore == nil {
		return true
	}
	banned, err := banRepositoryStore.IsBanned(request.Context(), userID)
	if err != nil {
		slog.WarnContext(request.Context(), "check user ban failed",
			"request_id", requestIDFromContext(request.Context()), "user_id", userID, "error", err)
		return true
	}
	if banned {
		auditAuth(request, "banned_write_rejected", "", userID)
		writeAPIError(writer, request, http.StatusForbidden, "user_banned", "账号已被封禁，无法执行该操作")
		return false
	}
	return true
}

// sqlInPlaceholders 生成 "?,?,?" 占位符及其参数切片，供 IN (...) 查询复用。
func sqlInPlaceholders(values []string) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}
	buffer := make([]byte, 0, len(values)*2)
	arguments := make([]any, len(values))
	for index, value := range values {
		if index > 0 {
			buffer = append(buffer, ',')
		}
		buffer = append(buffer, '?')
		arguments[index] = value
	}
	return string(buffer), arguments
}
