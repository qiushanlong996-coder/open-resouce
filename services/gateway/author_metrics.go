package main

import (
	"net/http"
	"strings"
)

// 作者后台的项目数据概览：浏览量 + 下载量。
// 只允许项目所有者查看，指标来自 project_metrics（best-effort，缺省为 0）。

type authorProjectMetricsResponse struct {
	Data struct {
		Views     int `json:"views"`
		Downloads int `json:"downloads"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

func authorProjectMetricsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	projectID := strings.TrimSuffix(
		strings.TrimPrefix(request.URL.Path, "/api/v1/author/projects/"), "/metrics")
	if projectID == "" || strings.Contains(projectID, "/") {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	project, found, err := managedProjectRepositoryStore.FindByID(request.Context(), projectID)
	if err != nil || !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if project.OwnerID != user.ID {
		writeAPIError(writer, request, http.StatusForbidden, "project_forbidden", "只能查看自己项目的数据")
		return
	}

	views, downloads := 0, 0
	if projectMetricsRepositoryStore != nil {
		snapshot, err := projectMetricsRepositoryStore.Snapshot(request.Context(), []string{projectID})
		if err == nil {
			if metric, exists := snapshot[projectID]; exists {
				views, downloads = metric.Views, metric.Downloads
			}
		}
	}
	var payload authorProjectMetricsResponse
	payload.Data.Views = views
	payload.Data.Downloads = downloads
	payload.RequestID = requestIDFromContext(request.Context())
	writeJSON(writer, http.StatusOK, payload)
}
