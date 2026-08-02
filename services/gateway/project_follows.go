package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

type followRepository interface {
	ListProjectIDs(context.Context, string) ([]string, error)
	SetFollow(context.Context, string, string, bool) error
	FollowerIDs(context.Context, string) ([]string, error)
}

type memoryFollowRepository struct {
	sync.RWMutex
	byUser map[string]map[string]struct{}
}

func newMemoryFollowRepository() *memoryFollowRepository {
	return &memoryFollowRepository{byUser: make(map[string]map[string]struct{})}
}

func (repository *memoryFollowRepository) ListProjectIDs(
	_ context.Context, userID string,
) ([]string, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]string, 0, len(repository.byUser[userID]))
	for projectID := range repository.byUser[userID] {
		result = append(result, projectID)
	}
	sort.Strings(result)
	return result, nil
}

func (repository *memoryFollowRepository) SetFollow(
	_ context.Context, userID string, projectID string, follow bool,
) error {
	repository.Lock()
	defer repository.Unlock()
	if !follow {
		if projects := repository.byUser[userID]; projects != nil {
			delete(projects, projectID)
		}
		return nil
	}
	if repository.byUser[userID] == nil {
		repository.byUser[userID] = make(map[string]struct{})
	}
	repository.byUser[userID][projectID] = struct{}{}
	return nil
}

func (repository *memoryFollowRepository) FollowerIDs(
	_ context.Context, projectID string,
) ([]string, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]string, 0)
	for userID, projects := range repository.byUser {
		if _, ok := projects[projectID]; ok {
			result = append(result, userID)
		}
	}
	sort.Strings(result)
	return result, nil
}

type mysqlFollowRepository struct {
	db *sql.DB
}

func newMySQLFollowRepository(db *sql.DB) *mysqlFollowRepository {
	return &mysqlFollowRepository{db: db}
}

func (repository *mysqlFollowRepository) ListProjectIDs(
	ctx context.Context, userID string,
) ([]string, error) {
	rows, err := repository.db.QueryContext(
		ctx, `SELECT project_id FROM project_follows WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list project follows: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return nil, fmt.Errorf("scan project follow: %w", err)
		}
		result = append(result, projectID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project follows: %w", err)
	}
	return result, nil
}

func (repository *mysqlFollowRepository) SetFollow(
	ctx context.Context, userID string, projectID string, follow bool,
) error {
	if follow {
		if _, err := repository.db.ExecContext(
			ctx, `INSERT IGNORE INTO project_follows (user_id, project_id) VALUES (?, ?)`,
			userID, projectID,
		); err != nil {
			return fmt.Errorf("create project follow: %w", err)
		}
		return nil
	}
	if _, err := repository.db.ExecContext(
		ctx, `DELETE FROM project_follows WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	); err != nil {
		return fmt.Errorf("delete project follow: %w", err)
	}
	return nil
}

func (repository *mysqlFollowRepository) FollowerIDs(
	ctx context.Context, projectID string,
) ([]string, error) {
	rows, err := repository.db.QueryContext(
		ctx, `SELECT user_id FROM project_follows WHERE project_id = ? ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list project followers: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan project follower: %w", err)
		}
		result = append(result, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project followers: %w", err)
	}
	return result, nil
}

var followRepositoryStore followRepository = newMemoryFollowRepository()

var _ followRepository = (*memoryFollowRepository)(nil)
var _ followRepository = (*mysqlFollowRepository)(nil)

type followListResponse struct {
	Data      []string `json:"data"`
	RequestID string   `json:"request_id"`
}

func followsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	projectIDs, err := followRepositoryStore.ListProjectIDs(request.Context(), user.ID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "关注服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, followListResponse{
		Data: projectIDs, RequestID: requestIDFromContext(request.Context()),
	})
}

func projectFollowHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost && request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if request.Method == http.MethodPost && !ensureNotBanned(writer, request, user.ID) {
		return
	}
	projectID, found, err := resolveProjectID(request.Context(), request.PathValue("slug"))
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "关注服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if err := followRepositoryStore.SetFollow(
		request.Context(), user.ID, projectID, request.Method == http.MethodPost,
	); err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "关注服务暂时不可用")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
