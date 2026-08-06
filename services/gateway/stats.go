package main

import (
	"context"
	"net/http"
	"time"
)

// 公开站点统计：首页指标条的数据源。
// 全部只读、对匿名开放；任一仓库故障时整体降级为空数据，不阻塞首页。

type siteStats struct {
	// Projects 是已发布项目数。
	Projects int `json:"projects"`
	// UpdatedToday 是今天（UTC 日界）有过更新的已发布项目数。
	UpdatedToday int `json:"updated_today"`
	// Downloads 是所有已发布项目的累计下载量。
	Downloads int `json:"downloads"`
	// Documents 是已发布项目下的真实文档篇数（不含项目正文兜底）。
	Documents int `json:"documents"`
}

type siteStatsResponse struct {
	Data      siteStats `json:"data"`
	RequestID string    `json:"request_id"`
}

func siteStatsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	stats, err := collectSiteStats(request.Context())
	if err != nil {
		// 统计是增强项：失败时返回空数据而不是 5xx，避免首页被统计拖垮。
		writeJSON(writer, http.StatusOK, siteStatsResponse{
			Data:      siteStats{},
			RequestID: requestIDFromContext(request.Context()),
		})
		return
	}
	writeJSON(writer, http.StatusOK, siteStatsResponse{
		Data:      stats,
		RequestID: requestIDFromContext(request.Context()),
	})
}

func collectSiteStats(ctx context.Context) (siteStats, error) {
	projects, err := managedProjectRepositoryStore.ListPublished(ctx)
	if err != nil {
		return siteStats{}, err
	}
	stats := siteStats{Projects: len(projects)}
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
		if !project.UpdatedAt.Before(todayStart) {
			stats.UpdatedToday++
		}
		documents, err := projectDocumentRepositoryStore.ListByProject(ctx, project.ID)
		if err != nil {
			return siteStats{}, err
		}
		stats.Documents += len(documents)
	}
	if len(ids) > 0 && projectMetricsRepositoryStore != nil {
		snapshots, err := projectMetricsRepositoryStore.Snapshot(ctx, ids)
		if err != nil {
			return siteStats{}, err
		}
		for _, snapshot := range snapshots {
			stats.Downloads += snapshot.Downloads
		}
	}
	return stats, nil
}
