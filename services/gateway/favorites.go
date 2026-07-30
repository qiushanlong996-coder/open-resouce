package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

type favoriteRepository interface {
	ListProjectIDs(context.Context, string) ([]string, error)
	SetFavorite(context.Context, string, string, bool) error
}

type memoryFavoriteRepository struct {
	sync.RWMutex
	byUser map[string]map[string]struct{}
}

func newMemoryFavoriteRepository() *memoryFavoriteRepository {
	return &memoryFavoriteRepository{byUser: make(map[string]map[string]struct{})}
}

func (repository *memoryFavoriteRepository) ListProjectIDs(
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

func (repository *memoryFavoriteRepository) SetFavorite(
	_ context.Context, userID string, projectID string, favorite bool,
) error {
	repository.Lock()
	defer repository.Unlock()
	if !favorite {
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

type mysqlFavoriteRepository struct {
	db *sql.DB
}

func newMySQLFavoriteRepository(db *sql.DB) *mysqlFavoriteRepository {
	return &mysqlFavoriteRepository{db: db}
}

func (repository *mysqlFavoriteRepository) ListProjectIDs(
	ctx context.Context, userID string,
) ([]string, error) {
	rows, err := repository.db.QueryContext(
		ctx, `SELECT project_id FROM project_favorites WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list project favorites: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return nil, fmt.Errorf("scan project favorite: %w", err)
		}
		result = append(result, projectID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project favorites: %w", err)
	}
	return result, nil
}

func (repository *mysqlFavoriteRepository) SetFavorite(
	ctx context.Context, userID string, projectID string, favorite bool,
) error {
	if favorite {
		if _, err := repository.db.ExecContext(
			ctx, `INSERT IGNORE INTO project_favorites (user_id, project_id) VALUES (?, ?)`,
			userID, projectID,
		); err != nil {
			return fmt.Errorf("create project favorite: %w", err)
		}
		return nil
	}
	if _, err := repository.db.ExecContext(
		ctx, `DELETE FROM project_favorites WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	); err != nil {
		return fmt.Errorf("delete project favorite: %w", err)
	}
	return nil
}

var favoriteRepositoryStore favoriteRepository = newMemoryFavoriteRepository()

type favoriteListResponse struct {
	Data      []string `json:"data"`
	RequestID string   `json:"request_id"`
}

func favoritesHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	projectIDs, err := favoriteRepositoryStore.ListProjectIDs(request.Context(), user.ID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "收藏服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, favoriteListResponse{
		Data: projectIDs, RequestID: requestIDFromContext(request.Context()),
	})
}

func projectFavoriteHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost && request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	project, found := seedProjectDetails[request.PathValue("slug")]
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if err := favoriteRepositoryStore.SetFavorite(
		request.Context(), user.ID, project.ID, request.Method == http.MethodPost,
	); err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "收藏服务暂时不可用")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
