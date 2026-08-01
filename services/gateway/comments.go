package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type documentComment struct {
	ID          string            `json:"id"`
	DocumentID  string            `json:"document_id"`
	ParentID    *string           `json:"parent_id"`
	BlockID     string            `json:"block_id"`
	AuthorID    string            `json:"author_id,omitempty"`
	Author      string            `json:"author"`
	AuthorLevel int               `json:"author_level,omitempty"`
	Quote       string            `json:"quote"`
	Body        string            `json:"body"`
	Status      string            `json:"status"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	ResolvedAt  *string           `json:"resolved_at"`
	Replies     []documentComment `json:"replies"`
	ReplyCount  int               `json:"reply_count"`
}

type commentListResponse struct {
	Data      []documentComment `json:"data"`
	RequestID string            `json:"request_id"`
}

type commentResponse struct {
	Data      documentComment `json:"data"`
	RequestID string          `json:"request_id"`
}

type createCommentRequest struct {
	BlockID string `json:"block_id"`
	Author  string `json:"author"`
	Quote   string `json:"quote"`
	Body    string `json:"body"`
}

type updateCommentRequest struct {
	Status string  `json:"status"`
	Body   *string `json:"body"`
}

type createReplyRequest struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

type updateReplyRequest struct {
	Body string `json:"body"`
}

type commentRepository interface {
	List(ctx context.Context, documentID string) ([]documentComment, error)
	Create(ctx context.Context, comment documentComment) (documentComment, error)
	CreateReply(ctx context.Context, parentID string, comment documentComment) (documentComment, bool, error)
	DeleteComment(ctx context.Context, documentID, commentID, actorID, deletedAt string) (bool, error)
	DeleteReply(ctx context.Context, documentID, parentID, replyID, actorID, deletedAt string) (bool, error)
	UpdateComment(ctx context.Context, documentID, commentID, actorID, body, updatedAt string) (documentComment, bool, error)
	UpdateReply(ctx context.Context, documentID, parentID, replyID, actorID, body, updatedAt string) (documentComment, bool, error)
	Resolve(ctx context.Context, documentID, commentID, actorID, resolvedAt string) (documentComment, bool, error)
}

type memoryCommentRepository struct {
	sync.RWMutex
	byDocument map[string][]documentComment
}

func newMemoryCommentRepository() *memoryCommentRepository {
	return &memoryCommentRepository{byDocument: map[string][]documentComment{
		"doc-atlas-quick-start": {
			{
				ID: "comment-atlas-001", DocumentID: "doc-atlas-quick-start",
				BlockID: "block-atlas-collaboration", Author: "林默",
				Quote:  "每个节点都需要声明输入、输出和失败策略。",
				Body:   "这里是否可以补充一个最小节点示例？新用户会更容易理解。",
				Status: "open", CreatedAt: "2026-07-26T11:42:00Z",
			},
		},
	}}
}

func (repository *memoryCommentRepository) List(_ context.Context, documentID string) ([]documentComment, error) {
	repository.RLock()
	defer repository.RUnlock()
	comments := append([]documentComment(nil), repository.byDocument[documentID]...)
	if comments == nil {
		return []documentComment{}, nil
	}
	return nestDocumentComments(comments), nil
}

func (repository *memoryCommentRepository) Create(_ context.Context, comment documentComment) (documentComment, error) {
	repository.Lock()
	defer repository.Unlock()
	comment.Replies = []documentComment{}
	if comment.UpdatedAt == "" {
		comment.UpdatedAt = comment.CreatedAt
	}
	repository.byDocument[comment.DocumentID] = append(repository.byDocument[comment.DocumentID], comment)
	return comment, nil
}

func (repository *memoryCommentRepository) CreateReply(_ context.Context, parentID string, reply documentComment) (documentComment, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for _, comment := range repository.byDocument[reply.DocumentID] {
		if comment.ID != parentID || comment.ParentID != nil {
			continue
		}
		reply.ParentID = &parentID
		reply.BlockID = comment.BlockID
		reply.Replies = []documentComment{}
		if reply.UpdatedAt == "" {
			reply.UpdatedAt = reply.CreatedAt
		}
		repository.byDocument[reply.DocumentID] = append(repository.byDocument[reply.DocumentID], reply)
		return reply, true, nil
	}
	return documentComment{}, false, nil
}

func (repository *memoryCommentRepository) DeleteComment(_ context.Context, documentID, commentID, actorID, _ string) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	comments := repository.byDocument[documentID]
	found := false
	for _, comment := range comments {
		if comment.ID == commentID && comment.ParentID == nil &&
			(actorID == "" || comment.AuthorID == actorID) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	filtered := make([]documentComment, 0, len(comments))
	for _, comment := range comments {
		if comment.ID == commentID && comment.ParentID == nil {
			continue
		}
		if comment.ParentID != nil && *comment.ParentID == commentID {
			continue
		}
		filtered = append(filtered, comment)
	}
	repository.byDocument[documentID] = filtered
	return true, nil
}

func (repository *memoryCommentRepository) DeleteReply(_ context.Context, documentID, parentID, replyID, actorID, _ string) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	comments := repository.byDocument[documentID]
	for index, comment := range comments {
		if comment.ID != replyID || comment.ParentID == nil || *comment.ParentID != parentID ||
			(actorID != "" && comment.AuthorID != actorID) {
			continue
		}
		repository.byDocument[documentID] = append(comments[:index], comments[index+1:]...)
		return true, nil
	}
	return false, nil
}

func (repository *memoryCommentRepository) UpdateComment(_ context.Context, documentID, commentID, actorID, body, updatedAt string) (documentComment, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byDocument[documentID] {
		comment := &repository.byDocument[documentID][index]
		if comment.ID != commentID || comment.ParentID != nil ||
			(actorID != "" && comment.AuthorID != actorID) {
			continue
		}
		comment.Body = body
		comment.UpdatedAt = updatedAt
		return *comment, true, nil
	}
	return documentComment{}, false, nil
}

func (repository *memoryCommentRepository) UpdateReply(_ context.Context, documentID, parentID, replyID, actorID, body, updatedAt string) (documentComment, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byDocument[documentID] {
		reply := &repository.byDocument[documentID][index]
		if reply.ID != replyID || reply.ParentID == nil || *reply.ParentID != parentID ||
			(actorID != "" && reply.AuthorID != actorID) {
			continue
		}
		reply.Body = body
		reply.UpdatedAt = updatedAt
		return *reply, true, nil
	}
	return documentComment{}, false, nil
}

func (repository *memoryCommentRepository) Resolve(_ context.Context, documentID, commentID, actorID, resolvedAt string) (documentComment, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byDocument[documentID] {
		comment := &repository.byDocument[documentID][index]
		if comment.ID != commentID || (actorID != "" && comment.AuthorID != actorID) {
			continue
		}
		if comment.Status != "resolved" {
			comment.Status = "resolved"
			comment.ResolvedAt = &resolvedAt
			comment.UpdatedAt = resolvedAt
		}
		return *comment, true, nil
	}
	return documentComment{}, false, nil
}

var commentRepositoryStore commentRepository = newMemoryCommentRepository()

var _ commentRepository = (*memoryCommentRepository)(nil)

// normalizeCommentReplies 保证单条评论响应的 replies 始终是数组。
// 仓库实现（如 MySQL 的 Create）可能不填该字段，nil 切片会序列化为 null，
// 导致客户端对数组做遍历时报错。
func normalizeCommentReplies(comment documentComment) documentComment {
	if comment.Replies == nil {
		comment.Replies = []documentComment{}
	}
	return comment
}

func nestDocumentComments(flat []documentComment) []documentComment {
	roots := make([]documentComment, 0)
	rootIndexes := make(map[string]int)
	for _, comment := range flat {
		comment.Replies = []documentComment{}
		comment.ReplyCount = 0
		if comment.UpdatedAt == "" {
			comment.UpdatedAt = comment.CreatedAt
		}
		if comment.ParentID == nil {
			rootIndexes[comment.ID] = len(roots)
			roots = append(roots, comment)
		}
	}
	for _, comment := range flat {
		if comment.ParentID == nil {
			continue
		}
		index, found := rootIndexes[*comment.ParentID]
		if !found {
			continue
		}
		comment.Replies = []documentComment{}
		roots[index].Replies = append(roots[index].Replies, comment)
		roots[index].ReplyCount = len(roots[index].Replies)
	}
	return roots
}

func documentCommentsHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}

	switch request.Method {
	case http.MethodGet:
		result, err := commentRepositoryStore.List(request.Context(), document.ID)
		if err != nil {
			writeRepositoryError(writer, request, err)
			return
		}
		result = enrichCommentLevels(request.Context(), result)
		writeJSON(writer, http.StatusOK, commentListResponse{Data: result, RequestID: requestIDFromContext(request.Context())})
	case http.MethodPost:
		actor, authenticated := requireCurrentUser(writer, request)
		if !authenticated {
			return
		}
		var input createCommentRequest
		if err := decodeJSONBody(request, &input); err != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的评论数据")
			return
		}
		input.BlockID = strings.TrimSpace(input.BlockID)
		input.Author = strings.TrimSpace(input.Author)
		if actor.ID != "" {
			input.Author = actor.DisplayName
		}
		input.Quote = strings.TrimSpace(input.Quote)
		input.Body = strings.TrimSpace(input.Body)
		if !documentHasBlock(document, input.BlockID) {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "block_not_found", "评论锚点不存在")
			return
		}
		if input.Author == "" || len([]rune(input.Author)) > 80 || input.Body == "" || len([]rune(input.Body)) > 2000 || len([]rune(input.Quote)) > 500 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_comment", "评论字段不符合要求")
			return
		}
		comment := documentComment{
			ID: "comment-" + newRequestID(), DocumentID: document.ID, BlockID: input.BlockID,
			AuthorID: actor.ID, Author: input.Author, Quote: input.Quote, Body: input.Body, Status: "open",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		comment, err := commentRepositoryStore.Create(request.Context(), comment)
		if err != nil {
			writeRepositoryError(writer, request, err)
			return
		}
		publishCommentEvent("comment.created", document.ID, comment.ID, "")
		awardExperienceBestEffort(actor.ID, xpActionComment, comment.ID, xpComment)
		writeJSON(writer, http.StatusCreated, commentResponse{Data: normalizeCommentReplies(comment), RequestID: requestIDFromContext(request.Context())})
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

func documentCommentHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}
	if request.Method != http.MethodPatch && request.Method != http.MethodDelete {
		writer.Header().Set("Allow", "DELETE, PATCH")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	actor, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}
	if request.Method == http.MethodDelete {
		deleted, err := commentRepositoryStore.DeleteComment(
			request.Context(), document.ID, request.PathValue("commentID"), actor.ID,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			writeRepositoryError(writer, request, err)
			return
		}
		if !deleted {
			auditCommentPermission(request, "comment_delete_denied")
			writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
			return
		}
		publishCommentEvent("comment.deleted", document.ID, request.PathValue("commentID"), "")
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	var input updateCommentRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的评论更新")
		return
	}
	if input.Body != nil && input.Status == "" {
		body := strings.TrimSpace(*input.Body)
		if body == "" || len([]rune(body)) > 2000 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_comment", "评论正文不符合要求")
			return
		}
		comment, found, err := commentRepositoryStore.UpdateComment(
			request.Context(), document.ID, request.PathValue("commentID"), actor.ID, body,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			writeRepositoryError(writer, request, err)
			return
		}
		if !found {
			auditCommentPermission(request, "comment_update_denied")
			writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
			return
		}
		publishCommentEvent("comment.updated", document.ID, comment.ID, "")
		writeJSON(writer, http.StatusOK, commentResponse{Data: normalizeCommentReplies(comment), RequestID: requestIDFromContext(request.Context())})
		return
	}
	if input.Status != "resolved" || input.Body != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "评论更新字段组合无效")
		return
	}

	resolvedAt := time.Now().UTC().Format(time.RFC3339)
	comment, found, err := commentRepositoryStore.Resolve(
		request.Context(), document.ID, request.PathValue("commentID"), actor.ID, resolvedAt,
	)
	if err != nil {
		writeRepositoryError(writer, request, err)
		return
	}
	if found {
		publishCommentEvent("comment.resolved", document.ID, comment.ID, "")
		writeJSON(writer, http.StatusOK, commentResponse{Data: normalizeCommentReplies(comment), RequestID: requestIDFromContext(request.Context())})
		return
	}
	auditCommentPermission(request, "comment_resolve_denied")
	writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
}

func documentCommentRepliesHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	actor, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}

	var input createReplyRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的回复数据")
		return
	}
	input.Author = strings.TrimSpace(input.Author)
	if actor.ID != "" {
		input.Author = actor.DisplayName
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Author == "" || len([]rune(input.Author)) > 80 || input.Body == "" || len([]rune(input.Body)) > 2000 {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_reply", "回复字段不符合要求")
		return
	}

	reply := documentComment{
		ID: "comment-" + newRequestID(), DocumentID: document.ID,
		AuthorID: actor.ID, Author: input.Author, Body: input.Body, Status: "open",
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Replies: []documentComment{},
	}
	reply, found, err := commentRepositoryStore.CreateReply(
		request.Context(), request.PathValue("commentID"), reply,
	)
	if err != nil {
		writeRepositoryError(writer, request, err)
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "父评论不存在")
		return
	}
	publishCommentEvent("reply.created", document.ID, request.PathValue("commentID"), reply.ID)
	awardExperienceBestEffort(actor.ID, xpActionReply, reply.ID, xpReply)
	notifyCommentReply(
		request.Context(), request.PathValue("slug"), request.PathValue("documentSlug"),
		document.ID, request.PathValue("commentID"), reply,
	)
	writeJSON(writer, http.StatusCreated, commentResponse{
		Data: normalizeCommentReplies(reply), RequestID: requestIDFromContext(request.Context()),
	})
}

func documentCommentReplyHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}
	if request.Method != http.MethodDelete && request.Method != http.MethodPatch {
		writer.Header().Set("Allow", "DELETE, PATCH")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	actor, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}
	if request.Method == http.MethodPatch {
		var input updateReplyRequest
		if err := decodeJSONBody(request, &input); err != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的回复更新")
			return
		}
		input.Body = strings.TrimSpace(input.Body)
		if input.Body == "" || len([]rune(input.Body)) > 2000 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_reply", "回复正文不符合要求")
			return
		}
		reply, found, err := commentRepositoryStore.UpdateReply(
			request.Context(), document.ID, request.PathValue("commentID"), request.PathValue("replyID"),
			actor.ID, input.Body, time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			writeRepositoryError(writer, request, err)
			return
		}
		if !found {
			auditCommentPermission(request, "reply_update_denied")
			writeAPIError(writer, request, http.StatusNotFound, "reply_not_found", "回复不存在")
			return
		}
		publishCommentEvent("reply.updated", document.ID, request.PathValue("commentID"), reply.ID)
		writeJSON(writer, http.StatusOK, commentResponse{Data: normalizeCommentReplies(reply), RequestID: requestIDFromContext(request.Context())})
		return
	}
	deleted, err := commentRepositoryStore.DeleteReply(
		request.Context(),
		document.ID,
		request.PathValue("commentID"),
		request.PathValue("replyID"),
		actor.ID,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		writeRepositoryError(writer, request, err)
		return
	}
	if !deleted {
		auditCommentPermission(request, "reply_delete_denied")
		writeAPIError(writer, request, http.StatusNotFound, "reply_not_found", "回复不存在")
		return
	}
	publishCommentEvent("reply.deleted", document.ID, request.PathValue("commentID"), request.PathValue("replyID"))
	writer.WriteHeader(http.StatusNoContent)
}

func auditCommentPermission(request *http.Request, event string) {
	user, _ := currentUser(request)
	slog.WarnContext(
		request.Context(),
		"security audit",
		"event", event,
		"request_id", requestIDFromContext(request.Context()),
		"user_id", user.ID,
		"comment_id", request.PathValue("commentID"),
		"reply_id", request.PathValue("replyID"),
	)
}

func writeRepositoryError(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "comment repository operation failed",
		"request_id", requestIDFromContext(request.Context()),
		"error", err,
	)
	writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "评论服务暂时不可用")
}

func findDocument(request *http.Request, writer http.ResponseWriter) (documentDetail, bool) {
	document, projectFound, documentFound, err := documents.Get(
		request.Context(), request.PathValue("slug"), request.PathValue("documentSlug"),
	)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档服务暂时不可用")
		return documentDetail{}, false
	}
	if !projectFound {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return documentDetail{}, false
	}
	if !documentFound {
		writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
		return documentDetail{}, false
	}
	return document, true
}

func documentHasBlock(document documentDetail, blockID string) bool {
	for _, block := range document.Blocks {
		if block.ID == blockID {
			return true
		}
	}
	return false
}

func decodeJSONBody(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
