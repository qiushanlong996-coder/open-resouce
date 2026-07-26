package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type documentComment struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	BlockID    string  `json:"block_id"`
	Author     string  `json:"author"`
	Quote      string  `json:"quote"`
	Body       string  `json:"body"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	ResolvedAt *string `json:"resolved_at"`
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
	Status string `json:"status"`
}

type commentRepository interface {
	List(documentID string) []documentComment
	Create(comment documentComment) documentComment
	Resolve(documentID, commentID, resolvedAt string) (documentComment, bool)
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

func (repository *memoryCommentRepository) List(documentID string) []documentComment {
	repository.RLock()
	defer repository.RUnlock()
	comments := append([]documentComment(nil), repository.byDocument[documentID]...)
	if comments == nil {
		return []documentComment{}
	}
	return comments
}

func (repository *memoryCommentRepository) Create(comment documentComment) documentComment {
	repository.Lock()
	defer repository.Unlock()
	repository.byDocument[comment.DocumentID] = append(repository.byDocument[comment.DocumentID], comment)
	return comment
}

func (repository *memoryCommentRepository) Resolve(documentID, commentID, resolvedAt string) (documentComment, bool) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byDocument[documentID] {
		comment := &repository.byDocument[documentID][index]
		if comment.ID != commentID {
			continue
		}
		if comment.Status != "resolved" {
			comment.Status = "resolved"
			comment.ResolvedAt = &resolvedAt
		}
		return *comment, true
	}
	return documentComment{}, false
}

var commentRepositoryStore commentRepository = newMemoryCommentRepository()

var _ commentRepository = (*memoryCommentRepository)(nil)

func documentCommentsHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}

	switch request.Method {
	case http.MethodGet:
		result := commentRepositoryStore.List(document.ID)
		writeJSON(writer, http.StatusOK, commentListResponse{Data: result, RequestID: requestIDFromContext(request.Context())})
	case http.MethodPost:
		var input createCommentRequest
		if err := decodeJSONBody(request, &input); err != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的评论数据")
			return
		}
		input.BlockID = strings.TrimSpace(input.BlockID)
		input.Author = strings.TrimSpace(input.Author)
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
			Author: input.Author, Quote: input.Quote, Body: input.Body, Status: "open",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		comment = commentRepositoryStore.Create(comment)
		writeJSON(writer, http.StatusCreated, commentResponse{Data: comment, RequestID: requestIDFromContext(request.Context())})
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
	if request.Method != http.MethodPatch {
		writer.Header().Set("Allow", http.MethodPatch)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	var input updateCommentRequest
	if err := decodeJSONBody(request, &input); err != nil || input.Status != "resolved" {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "仅支持将评论状态更新为 resolved")
		return
	}

	resolvedAt := time.Now().UTC().Format(time.RFC3339)
	comment, found := commentRepositoryStore.Resolve(document.ID, request.PathValue("commentID"), resolvedAt)
	if found {
		writeJSON(writer, http.StatusOK, commentResponse{Data: comment, RequestID: requestIDFromContext(request.Context())})
		return
	}
	writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
}

func findDocument(request *http.Request, writer http.ResponseWriter) (documentDetail, bool) {
	document, projectFound, documentFound := documents.Get(request.PathValue("slug"), request.PathValue("documentSlug"))
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
