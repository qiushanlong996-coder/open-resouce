package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 项目浏览量与下载量统计。
//
// 浏览：前端打开项目详情时打一个 beacon（POST .../view），进程无关的弱一致计数。
// 下载：资源下载接口命中文档/代码包时累加。
// 统计写入按 best-effort：失败只记日志，不影响浏览与下载主流程。
// project_id 兼容种子项目（如 "atlas"）与已发布项目 ID，因此不设外键。

type projectMetricSnapshot struct {
	Views     int `json:"views"`
	Downloads int `json:"downloads"`
}

type projectMetricsRepository interface {
	IncrementView(ctx context.Context, projectID string) error
	IncrementDownload(ctx context.Context, projectID string) error
	Snapshot(ctx context.Context, projectIDs []string) (map[string]projectMetricSnapshot, error)
}

type memoryProjectMetricsRepository struct {
	sync.RWMutex
	byProject map[string]projectMetricSnapshot
}

func newMemoryProjectMetricsRepository() *memoryProjectMetricsRepository {
	return &memoryProjectMetricsRepository{byProject: make(map[string]projectMetricSnapshot)}
}

func (repository *memoryProjectMetricsRepository) IncrementView(_ context.Context, projectID string) error {
	repository.Lock()
	defer repository.Unlock()
	entry := repository.byProject[projectID]
	entry.Views++
	repository.byProject[projectID] = entry
	return nil
}

func (repository *memoryProjectMetricsRepository) IncrementDownload(_ context.Context, projectID string) error {
	repository.Lock()
	defer repository.Unlock()
	entry := repository.byProject[projectID]
	entry.Downloads++
	repository.byProject[projectID] = entry
	return nil
}

func (repository *memoryProjectMetricsRepository) Snapshot(
	_ context.Context, projectIDs []string,
) (map[string]projectMetricSnapshot, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make(map[string]projectMetricSnapshot, len(projectIDs))
	for _, id := range projectIDs {
		if entry, ok := repository.byProject[id]; ok {
			result[id] = entry
		}
	}
	return result, nil
}

type mysqlProjectMetricsRepository struct{ db *sql.DB }

func newMySQLProjectMetricsRepository(db *sql.DB) *mysqlProjectMetricsRepository {
	return &mysqlProjectMetricsRepository{db: db}
}

func (repository *mysqlProjectMetricsRepository) IncrementView(ctx context.Context, projectID string) error {
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO project_metrics (project_id, views) VALUES (?, 1)
		 ON DUPLICATE KEY UPDATE views = views + 1`, projectID)
	if err != nil {
		return fmt.Errorf("increment project view: %w", err)
	}
	return nil
}

func (repository *mysqlProjectMetricsRepository) IncrementDownload(ctx context.Context, projectID string) error {
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO project_metrics (project_id, downloads) VALUES (?, 1)
		 ON DUPLICATE KEY UPDATE downloads = downloads + 1`, projectID)
	if err != nil {
		return fmt.Errorf("increment project download: %w", err)
	}
	return nil
}

func (repository *mysqlProjectMetricsRepository) Snapshot(
	ctx context.Context, projectIDs []string,
) (map[string]projectMetricSnapshot, error) {
	result := make(map[string]projectMetricSnapshot)
	if len(projectIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	arguments := make([]any, len(projectIDs))
	for index, id := range projectIDs {
		arguments[index] = id
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT project_id, views, downloads FROM project_metrics WHERE project_id IN (`+placeholders+`)`,
		arguments...)
	if err != nil {
		return nil, fmt.Errorf("snapshot project metrics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var snapshot projectMetricSnapshot
		if err := rows.Scan(&id, &snapshot.Views, &snapshot.Downloads); err != nil {
			return nil, fmt.Errorf("scan project metric: %w", err)
		}
		result[id] = snapshot
	}
	return result, rows.Err()
}

var projectMetricsRepositoryStore projectMetricsRepository = newMemoryProjectMetricsRepository()

var _ projectMetricsRepository = (*memoryProjectMetricsRepository)(nil)
var _ projectMetricsRepository = (*mysqlProjectMetricsRepository)(nil)

// overlayProjectMetrics 用真实统计覆盖一组项目摘要的 views/downloads。
// 查询失败时保持原值（种子项目的展示值），不阻塞列表。
func overlayProjectMetrics(ctx context.Context, summaries []projectSummary) {
	if projectMetricsRepositoryStore == nil || len(summaries) == 0 {
		return
	}
	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	snapshot, err := projectMetricsRepositoryStore.Snapshot(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "overlay project metrics failed", "error", err)
		return
	}
	for index := range summaries {
		if entry, ok := snapshot[summaries[index].ID]; ok {
			summaries[index].Metrics.Views = entry.Views
			summaries[index].Metrics.Downloads = entry.Downloads
		}
	}
}

// recordDownloadBestEffort 异步累加下载量，失败只记日志。
func recordDownloadBestEffort(projectID string) {
	if projectMetricsRepositoryStore == nil || projectID == "" {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := projectMetricsRepositoryStore.IncrementDownload(ctx, projectID); err != nil {
			slog.Warn("record project download failed", "project_id", projectID, "error", err)
		}
	})
}

// projectViewHandler 记录一次项目浏览（beacon）。公开、无需登录。
//
//	POST /api/v1/projects/{slug}/view
func projectViewHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projectID, found, err := resolveProjectID(request.Context(), request.PathValue("slug"))
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if err := projectMetricsRepositoryStore.IncrementView(request.Context(), projectID); err != nil {
		slog.WarnContext(request.Context(), "record project view failed", "project_id", projectID, "error", err)
	}
	writer.WriteHeader(http.StatusNoContent)
}
