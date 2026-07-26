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

var commentStore = struct {
	sync.RWMutex
	byDocument map[string][]documentComment
}{
	byDocument: map[string][]documentComment{
		"doc-atlas-quick-start": {
			{
				ID: "comment-atlas-001", DocumentID: "doc-atlas-quick-start",
				BlockID: "block-atlas-collaboration", Author: "林默",
				Quote:  "每个节点都需要声明输入、输出和失败策略。",
				Body:   "这里是否可以补充一个最小节点示例？新用户会更容易理解。",
				Status: "open", CreatedAt: "2026-07-26T11:42:00Z",
			},
		},
	},
}

func documentCommentsHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}

	switch request.Method {
	case http.MethodGet:
		commentStore.RLock()
		comments := append([]documentComment(nil), commentStore.byDocument[document.ID]...)
		commentStore.RUnlock()
		if comments == nil {
			comments = []documentComment{}
		}
		writeJSON(writer, http.StatusOK, commentListResponse{Data: comments, RequestID: requestIDFromContext(request.Context())})
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
		commentStore.Lock()
		commentStore.byDocument[document.ID] = append(commentStore.byDocument[document.ID], comment)
		commentStore.Unlock()
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

	commentStore.Lock()
	defer commentStore.Unlock()
	for index := range commentStore.byDocument[document.ID] {
		if commentStore.byDocument[document.ID][index].ID != request.PathValue("commentID") {
			continue
		}
		if commentStore.byDocument[document.ID][index].Status != "resolved" {
			resolvedAt := time.Now().UTC().Format(time.RFC3339)
			commentStore.byDocument[document.ID][index].Status = "resolved"
			commentStore.byDocument[document.ID][index].ResolvedAt = &resolvedAt
		}
		writeJSON(writer, http.StatusOK, commentResponse{Data: commentStore.byDocument[document.ID][index], RequestID: requestIDFromContext(request.Context())})
		return
	}
	writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
}

func findDocument(request *http.Request, writer http.ResponseWriter) (documentDetail, bool) {
	documents, found := seedDocuments[request.PathValue("slug")]
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return documentDetail{}, false
	}
	document, found := documents[request.PathValue("documentSlug")]
	if !found {
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
