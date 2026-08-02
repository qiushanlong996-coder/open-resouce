package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 管理控制台后端。
//
// 在既有「项目审核中心」之外扩展出概览统计、用户管理、黑名单、项目管理与
// 审计日志。所有端点都经 requireAdminUser 校验（非管理员 401/403）。

// adminUserSummary 是用户管理列表项。banned 由 ban 仓库叠加填充。
type adminUserSummary struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Experience  int       `json:"experience"`
	Level       int       `json:"level"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
	Banned      bool      `json:"banned"`
	// LastLoginRegion / LastLoginAt 是最近一次登录的 IP 归属地与时间。
	// 仅管理后台可见，用于识别异常登录来源。只存归属地不存 IP；
	// 从未登录过的存量用户两者均为空。
	LastLoginRegion string     `json:"last_login_region,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
}

// commentCounter 是评论仓库的可选能力：并非所有实现（如测试替身）都提供。
type commentCounter interface {
	CountAll(ctx context.Context) (int, error)
}

// adminStatsHandler 汇总管理概览统计。
//
//	GET /api/v1/admin/stats
func adminStatsHandler(writer http.ResponseWriter, request *http.Request) {
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
	ctx := request.Context()
	userCount, err := authRepositoryStore.CountUsers(ctx)
	if err != nil {
		writeAdminStatsError(writer, request, err)
		return
	}
	statusCounts, err := managedProjectRepositoryStore.CountByStatus(ctx)
	if err != nil {
		writeAdminStatsError(writer, request, err)
		return
	}
	projectTotal := 0
	for _, count := range statusCounts {
		projectTotal += count
	}
	commentCount := 0
	if counter, ok := commentRepositoryStore.(commentCounter); ok {
		if value, err := counter.CountAll(ctx); err != nil {
			slog.WarnContext(ctx, "count comments failed",
				"request_id", requestIDFromContext(ctx), "error", err)
		} else {
			commentCount = value
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{
			"users":              userCount,
			"projects_total":     projectTotal,
			"projects_by_status": statusCounts,
			"pending_reviews":    statusCounts["pending_review"],
			"comments":           commentCount,
		},
		"request_id": requestIDFromContext(ctx),
	})
}

func writeAdminStatsError(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "load admin stats failed",
		"request_id", requestIDFromContext(request.Context()), "error", err)
	writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "统计服务暂时不可用")
}

// adminUsersHandler 分页/搜索列出用户，并叠加封禁状态。
//
//	GET /api/v1/admin/users?search=&page=&page_size=
func adminUsersHandler(writer http.ResponseWriter, request *http.Request) {
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
	page, pageSize := paginationParams(request, 20, 100)
	search := strings.TrimSpace(request.URL.Query().Get("search"))
	users, total, err := authRepositoryStore.ListUsers(request.Context(), search, pageSize, (page-1)*pageSize)
	if err != nil {
		slog.ErrorContext(request.Context(), "list users failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "用户服务暂时不可用")
		return
	}
	if banRepositoryStore != nil && len(users) > 0 {
		ids := make([]string, len(users))
		for index, user := range users {
			ids[index] = user.ID
		}
		if banned, err := banRepositoryStore.BannedSet(request.Context(), ids); err != nil {
			slog.WarnContext(request.Context(), "load banned set failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
		} else {
			for index := range users {
				users[index].Banned = banned[users[index].ID]
			}
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": users, "total": total, "page": page, "page_size": pageSize,
		"request_id": requestIDFromContext(request.Context()),
	})
}

// adminUserBanHandler 封禁/解封用户。
//
//	POST   /api/v1/admin/users/{userID}/ban   封禁
//	DELETE /api/v1/admin/users/{userID}/ban   解封
func adminUserBanHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	userID := request.PathValue("userID")
	if userID == "" {
		writeAPIError(writer, request, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}
	switch request.Method {
	case http.MethodPost:
		if userID == admin.ID {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "cannot_ban_self", "不能封禁自己")
			return
		}
		var input struct {
			Reason string `json:"reason"`
		}
		if request.Body != nil && request.ContentLength != 0 && decodeJSONBody(request, &input) != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "封禁数据格式不正确")
			return
		}
		input.Reason = strings.TrimSpace(input.Reason)
		if len([]rune(input.Reason)) > 500 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_ban_reason", "封禁原因不能超过 500 字")
			return
		}
		if err := banRepositoryStore.Ban(request.Context(), userID, admin.ID, input.Reason, time.Now().UTC()); err != nil {
			slog.ErrorContext(request.Context(), "ban user failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "封禁服务暂时不可用")
			return
		}
		recordAdminAudit(request, admin, "user_banned", userID, input.Reason)
		writer.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		unbanned, err := banRepositoryStore.Unban(request.Context(), userID)
		if err != nil {
			slog.ErrorContext(request.Context(), "unban user failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "封禁服务暂时不可用")
			return
		}
		if !unbanned {
			writeAPIError(writer, request, http.StatusNotFound, "ban_not_found", "该用户未被封禁")
			return
		}
		recordAdminAudit(request, admin, "user_unbanned", userID, "")
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.Header().Set("Allow", "POST, DELETE")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

// adminProjectsHandler 跨状态列出所有项目。
//
//	GET /api/v1/admin/projects?status=&page=&page_size=
func adminProjectsHandler(writer http.ResponseWriter, request *http.Request) {
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
	page, pageSize := paginationParams(request, 20, 100)
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	projects, total, err := managedProjectRepositoryStore.ListAll(request.Context(), status, pageSize, (page-1)*pageSize)
	if err != nil {
		writeManagedProjectError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": projects, "total": total, "page": page, "page_size": pageSize,
		"request_id": requestIDFromContext(request.Context()),
	})
}

// adminProjectTakedownHandler 下架项目：置为 archived 并清出公开目录与搜索索引。
//
//	POST /api/v1/admin/projects/{projectID}/takedown
func adminProjectTakedownHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projectID := request.PathValue("projectID")
	var input struct {
		Reason string `json:"reason"`
	}
	if request.Body != nil && request.ContentLength != 0 && decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "下架数据格式不正确")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	project, err := managedProjectRepositoryStore.Takedown(request.Context(), projectID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, errProjectImmutable) {
			writeAPIError(writer, request, http.StatusConflict, "project_already_down", "项目已下架")
			return
		}
		writeManagedProjectError(writer, request, err)
		return
	}
	// 复用审核索引同步：非 approve 分支会清空该项目的搜索记录。
	syncProjectSearchIndex(project, "takedown")
	recordAdminAudit(request, admin, "project_takedown", projectID, input.Reason)
	writeJSON(writer, http.StatusOK, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
}

// paginationParams 解析 page/page_size，套用默认值与上限。
func paginationParams(request *http.Request, defaultSize, maxSize int) (int, int) {
	page := 1
	if raw := request.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := defaultSize
	if raw := request.URL.Query().Get("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	if pageSize > maxSize {
		pageSize = maxSize
	}
	return page, pageSize
}
