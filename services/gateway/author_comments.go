package main

import (
	"net/http"
	"strings"
)

// 作者后台的评论与批注管理：列出项目下所有文档的评论（仅所有者/编辑者）。
// 评论以文档为单位存储，这里按文档展开并带上文档标题，便于作者定位处理。

type authorCommentItem struct {
	ID            string `json:"id"`
	DocumentID    string `json:"document_id"`
	DocumentTitle string `json:"document_title"`
	AuthorName    string `json:"author_name"`
	Body           string `json:"body"`
	Quote          string `json:"quote"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

type authorCommentListResponse struct {
	Data      []authorCommentItem `json:"data"`
	RequestID string              `json:"request_id"`
}

func authorProjectCommentsHandler(writer http.ResponseWriter, request *http.Request) {
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
		strings.TrimPrefix(request.URL.Path, "/api/v1/author/projects/"), "/comments")
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
		access, err := resolveCollaborationAccess(request.Context(), project, user, true)
		if err != nil || !access.CanEdit {
			writeAPIError(writer, request, http.StatusForbidden, "project_forbidden", "只有所有者或编辑者可查看评论")
			return
		}
	}

	documents, err := projectDocumentRepositoryStore.ListByProject(request.Context(), projectID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档列表暂时不可用")
		return
	}
	items := make([]authorCommentItem, 0)
	for _, document := range documents {
		comments, err := commentRepositoryStore.List(request.Context(), document.ID)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "评论列表暂时不可用")
			return
		}
		for _, comment := range comments {
			items = append(items, authorCommentItem{
				ID: comment.ID, DocumentID: comment.DocumentID,
				DocumentTitle: document.Title, AuthorName: comment.Author,
				Body: comment.Body, Quote: comment.Quote,
				Status: comment.Status, CreatedAt: comment.CreatedAt,
			})
			for _, reply := range comment.Replies {
				items = append(items, authorCommentItem{
					ID: reply.ID, DocumentID: reply.DocumentID,
					DocumentTitle: document.Title, AuthorName: reply.Author,
					Body: reply.Body, Quote: reply.Quote,
					Status: reply.Status, CreatedAt: reply.CreatedAt,
				})
			}
		}
	}
	writeJSON(writer, http.StatusOK, authorCommentListResponse{
		Data: items, RequestID: requestIDFromContext(request.Context()),
	})
}
