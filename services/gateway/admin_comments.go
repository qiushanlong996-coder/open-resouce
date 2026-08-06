package main

import (
	"net/http"
	"strings"
)

// 管理端评论治理：全站评论列表 + 隐藏/恢复（软删除）。
// 隐藏复用 document_comments.deleted_at 软删除语义，不物理删除，可恢复。

type adminCommentRecord struct {
	ID            string `json:"id"`
	DocumentID    string `json:"document_id"`
	DocumentTitle string `json:"document_title"`
	ProjectSlug   string `json:"project_slug"`
	ProjectName   string `json:"project_name"`
	ParentID      string `json:"parent_id,omitempty"`
	AuthorID      string `json:"author_id,omitempty"`
	AuthorName    string `json:"author_name"`
	Body          string `json:"body"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	Hidden        bool   `json:"hidden"`
}

type adminCommentListResponse struct {
	Data      []adminCommentRecord `json:"data"`
	RequestID string               `json:"request_id"`
}

func adminCommentsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	switch status {
	case "", "all", "open", "resolved", "hidden":
	default:
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_query",
			"status 只能是 all / open / resolved / hidden")
		return
	}
	records, err := commentRepositoryStore.ListAllAdmin(request.Context(), status)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "评论列表暂时不可用")
		return
	}
	_ = user
	writeJSON(writer, http.StatusOK, adminCommentListResponse{
		Data: records, RequestID: requestIDFromContext(request.Context()),
	})
}

// adminCommentModerationHandler 处理 POST /api/v1/admin/comments/{id}/hide|restore。
func adminCommentModerationHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/comments/")
	hidden := strings.HasSuffix(path, "/hide")
	restored := strings.HasSuffix(path, "/restore")
	if !hidden && !restored {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "接口不存在")
		return
	}
	commentID := strings.TrimSuffix(strings.TrimSuffix(path, "/hide"), "/restore")
	if commentID == "" || strings.Contains(commentID, "/") {
		writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
		return
	}
	updated, err := commentRepositoryStore.SetAdminHidden(request.Context(), commentID, hidden)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "评论状态更新失败")
		return
	}
	if !updated {
		writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
		return
	}
	action := "comment_restored"
	if hidden {
		action = "comment_hidden"
	}
	auditAuth(request, action, user.Email, commentID)
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{"id": commentID, "hidden": hidden},
		"request_id": requestIDFromContext(request.Context()),
	})
}
