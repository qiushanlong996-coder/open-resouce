package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// 首页推荐位管理：管理员挑选已发布项目进入推荐，前台按顺序展示。
// 推荐位只是展示层配置，不改变项目的发布状态。

type featuredProjectRepository interface {
	// List 返回推荐项目 ID（按 sort_order 升序）。
	List(ctx context.Context) ([]string, error)
	// Replace 全量替换推荐列表，写审计由调用方负责。
	Replace(ctx context.Context, projectIDs []string, byUser string) error
}

type memoryFeaturedProjectRepository struct {
	sync.RWMutex
	ids []string
}

func newMemoryFeaturedProjectRepository() *memoryFeaturedProjectRepository {
	return &memoryFeaturedProjectRepository{}
}

func (repository *memoryFeaturedProjectRepository) List(_ context.Context) ([]string, error) {
	repository.RLock()
	defer repository.RUnlock()
	return append([]string(nil), repository.ids...), nil
}

func (repository *memoryFeaturedProjectRepository) Replace(
	_ context.Context, projectIDs []string, _ string,
) error {
	repository.Lock()
	defer repository.Unlock()
	repository.ids = append([]string(nil), projectIDs...)
	return nil
}

type mysqlFeaturedProjectRepository struct {
	db *sql.DB
}

func newMySQLFeaturedProjectRepository(db *sql.DB) *mysqlFeaturedProjectRepository {
	return &mysqlFeaturedProjectRepository{db: db}
}

func (repository *mysqlFeaturedProjectRepository) List(ctx context.Context) ([]string, error) {
	rows, err := repository.db.QueryContext(ctx,
		`SELECT project_id FROM featured_projects ORDER BY sort_order, project_id`)
	if err != nil {
		return nil, fmt.Errorf("list featured projects: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan featured project: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (repository *mysqlFeaturedProjectRepository) Replace(
	ctx context.Context, projectIDs []string, byUser string,
) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace featured: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `DELETE FROM featured_projects`); err != nil {
		return fmt.Errorf("clear featured projects: %w", err)
	}
	now := time.Now().UTC()
	for index, projectID := range projectIDs {
		if _, err := transaction.ExecContext(ctx,
			`INSERT INTO featured_projects (project_id, sort_order, created_by, created_at)
			 VALUES (?, ?, ?, ?)`,
			projectID, index, byUser, now); err != nil {
			return fmt.Errorf("insert featured project: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit replace featured: %w", err)
	}
	return nil
}

var featuredProjectRepositoryStore featuredProjectRepository = newMemoryFeaturedProjectRepository()

var errFeaturedProjectNotPublished = errors.New("featured project must be published")

// resolveFeaturedProjects 把推荐 ID 解析为已发布项目（缺失/未发布跳过），保持顺序。
func resolveFeaturedProjects(ctx context.Context, ids []string) ([]managedProject, error) {
	projects := make([]managedProject, 0, len(ids))
	for _, id := range ids {
		project, found, err := managedProjectRepositoryStore.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if !found || project.Status != "published" {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

type featuredProjectResponse struct {
	Data      []managedProject `json:"data"`
	RequestID string           `json:"request_id"`
}

// publicFeaturedHandler 返回前台推荐项目（公开）。
func publicFeaturedHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	ids, err := featuredProjectRepositoryStore.List(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "推荐列表暂时不可用")
		return
	}
	projects, err := resolveFeaturedProjects(request.Context(), ids)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "推荐列表暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, featuredProjectResponse{
		Data: projects, RequestID: requestIDFromContext(request.Context()),
	})
}

type adminFeaturedResponse struct {
	Data struct {
		Featured []managedProject `json:"featured"`
		// Candidates 是所有已发布项目，供管理端勾选。
		Candidates []managedProject `json:"candidates"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

func adminFeaturedHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		ids, err := featuredProjectRepositoryStore.List(request.Context())
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "推荐列表暂时不可用")
			return
		}
		featured, err := resolveFeaturedProjects(request.Context(), ids)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "推荐列表暂时不可用")
			return
		}
		candidates, err := managedProjectRepositoryStore.ListPublished(request.Context())
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目列表暂时不可用")
			return
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Name < candidates[j].Name
		})
		var payload adminFeaturedResponse
		payload.Data.Featured = featured
		payload.Data.Candidates = candidates
		payload.RequestID = requestIDFromContext(request.Context())
		writeJSON(writer, http.StatusOK, payload)
	case http.MethodPut:
		var input struct {
			ProjectIDs []string `json:"project_ids"`
		}
		if err := decodeJSONBody(request, &input); err != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的推荐配置")
			return
		}
		seen := make(map[string]struct{}, len(input.ProjectIDs))
		ids := make([]string, 0, len(input.ProjectIDs))
		for _, raw := range input.ProjectIDs {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			project, found, err := managedProjectRepositoryStore.FindByID(request.Context(), id)
			if err != nil {
				writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目查询失败")
				return
			}
			if !found {
				writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project", "包含不存在的项目")
				return
			}
			if project.Status != "published" {
				writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project", "只能推荐已发布项目")
				return
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		if err := featuredProjectRepositoryStore.Replace(request.Context(), ids, user.ID); err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "推荐位保存失败")
			return
		}
		auditAuth(request, "featured_projects_updated", user.Email, user.ID)
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": map[string]any{"featured": ids},
			"request_id": requestIDFromContext(request.Context()),
		})
	default:
		writer.Header().Set("Allow", "GET, PUT")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}
