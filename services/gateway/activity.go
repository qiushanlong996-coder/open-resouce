package main

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// 社区动态：最近发布/更新的项目 + 最近评论，合并按时间倒序。
// 前端首页据此展示社区活跃度；评论条目复用管理端评论仓库（含项目信息）。

type activityItem struct {
	Type        string `json:"type"` // project_published | project_updated | comment
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	ProjectSlug string `json:"project_slug"`
	CreatedAt   string `json:"created_at"`
}

type activityResponse struct {
	Data      []activityItem `json:"data"`
	RequestID string         `json:"request_id"`
}

const maxActivityItems = 20

func activityHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projects, err := managedProjectRepositoryStore.ListPublished(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "社区动态暂时不可用")
		return
	}
	items := make([]activityItem, 0, len(projects)+10)
	for _, project := range projects {
		if project.PublishedAt != nil && !project.PublishedAt.IsZero() {
			items = append(items, activityItem{
				Type: "project_published", Title: project.Name,
				Summary: project.Summary, ProjectSlug: project.Slug,
				CreatedAt: project.PublishedAt.UTC().Format(time.RFC3339),
			})
		}
		items = append(items, activityItem{
			Type: "project_updated", Title: project.Name,
			Summary: project.Summary, ProjectSlug: project.Slug,
			CreatedAt: project.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	// 最近评论：复用管理评论仓库的跨项目列表（过滤软删除）。
	if commentRepositoryStore != nil {
		comments, err := commentRepositoryStore.ListAllAdmin(request.Context(), "all")
		if err == nil {
			for _, comment := range comments {
				slug := comment.ProjectSlug
				title := comment.ProjectName
				if title == "" {
					title = comment.DocumentTitle
				}
				if title == "" {
					continue
				}
				items = append(items, activityItem{
					Type: "comment", Title: title,
					Summary: comment.AuthorName + "：" + comment.Body,
					ProjectSlug: slug, CreatedAt: comment.CreatedAt,
				})
			}
		}
	}
	sort.SliceStable(items, func(left, right int) bool {
		return items[left].CreatedAt > items[right].CreatedAt
	})
	if len(items) > maxActivityItems {
		items = items[:maxActivityItems]
	}
	for index := range items {
		items[index].Summary = strings.TrimSpace(items[index].Summary)
	}
	writeJSON(writer, http.StatusOK, activityResponse{
		Data: items, RequestID: requestIDFromContext(request.Context()),
	})
}
