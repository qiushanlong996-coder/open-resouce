package main

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// 公开用户资料。
//
// GET /api/v1/users/{id}/profile 返回一个可公开展示的作者主页：昵称、等级、注册时间、
// 其已发布项目摘要以及浏览/下载汇总。绝不返回邮箱或任何 PII——邮箱仅在服务端用于计算展示等级。

type publicUserStats struct {
	ProjectsCount  int `json:"projects_count"`
	TotalViews     int `json:"total_views"`
	TotalDownloads int `json:"total_downloads"`
}

type publicUserProfile struct {
	ID          string           `json:"id"`
	DisplayName string           `json:"display_name"`
	Level       int              `json:"level"`
	AvatarFrame string           `json:"avatar_frame,omitempty"`
	JoinedAt    string           `json:"joined_at"`
	Projects    []projectSummary `json:"projects"`
	Stats       publicUserStats  `json:"stats"`
}

type publicUserProfileResponse struct {
	Data      publicUserProfile `json:"data"`
	RequestID string            `json:"request_id"`
}

func userProfileHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if authRepositoryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "auth_unavailable", "身份服务暂不可用")
		return
	}
	userID := strings.TrimSpace(request.PathValue("id"))
	if userID == "" {
		writeAPIError(writer, request, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}
	record, found, err := authRepositoryStore.FindPublicUserByID(request.Context(), userID)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}

	// 只展示已发布的项目：草稿、待审、驳回、下架都不公开。
	owned, err := managedProjectRepositoryStore.ListByOwner(request.Context(), userID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	published := make([]projectSummary, 0, len(owned))
	for _, project := range owned {
		if project.Status != "published" {
			continue
		}
		published = append(published, managedProjectSummary(project))
	}
	// 稳定排序：更新时间倒序，便于展示最新作品在前。
	sort.SliceStable(published, func(i, j int) bool {
		return published[i].UpdatedAt > published[j].UpdatedAt
	})
	// 用真实统计覆盖展示值，并汇总总浏览/下载。
	overlayProjectMetrics(request.Context(), published)
	stats := publicUserStats{ProjectsCount: len(published)}
	for _, project := range published {
		stats.TotalViews += project.Metrics.Views
		stats.TotalDownloads += project.Metrics.Downloads
	}

	profile := publicUserProfile{
		ID:          record.ID,
		DisplayName: record.DisplayName,
		Level:       levelForUser(record.Email, record.Experience),
		AvatarFrame: record.AvatarFrame,
		JoinedAt:    record.CreatedAt.UTC().Format(time.RFC3339),
		Projects:    published,
		Stats:       stats,
	}
	writeJSON(writer, http.StatusOK, publicUserProfileResponse{
		Data:      profile,
		RequestID: requestIDFromContext(request.Context()),
	})
}
