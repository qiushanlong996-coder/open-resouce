package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentCommentLifecycle(t *testing.T) {
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent/documents/quick-start/comments", nil)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResponse.Code, http.StatusOK)
	}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/documents/quick-start/comments", strings.NewReader(`{
		"block_id":"block-atlas-task",
		"author":"测试用户",
		"quote":"规划器会把目标拆成步骤。",
		"body":"请补充失败重试说明。"
	}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createResponse.Code, http.StatusCreated, createResponse.Body.String())
	}
	var created commentResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.BlockID != "block-atlas-task" || created.Data.Status != "open" {
		t.Fatalf("unexpected created comment: %#v", created.Data)
	}

	editRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/atlas-agent/documents/quick-start/comments/"+created.Data.ID, strings.NewReader(`{"body":"更新后的评论正文"}`))
	editResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(editResponse, editRequest)
	if editResponse.Code != http.StatusOK {
		t.Fatalf("edit status = %d, want %d: %s", editResponse.Code, http.StatusOK, editResponse.Body.String())
	}
	var edited commentResponse
	if err := json.Unmarshal(editResponse.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.Data.Body != "更新后的评论正文" || edited.Data.UpdatedAt == "" {
		t.Fatalf("unexpected edited comment: %#v", edited.Data)
	}

	resolveRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/atlas-agent/documents/quick-start/comments/"+created.Data.ID, strings.NewReader(`{"status":"resolved"}`))
	resolveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(resolveResponse, resolveRequest)
	if resolveResponse.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, want %d: %s", resolveResponse.Code, http.StatusOK, resolveResponse.Body.String())
	}
	var resolved commentResponse
	if err := json.Unmarshal(resolveResponse.Body.Bytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved.Data.Status != "resolved" || resolved.Data.ResolvedAt == nil {
		t.Fatalf("unexpected resolved comment: %#v", resolved.Data)
	}
}

func TestDocumentCommentReply(t *testing.T) {
	commentRepositoryStore = newMemoryCommentRepository()
	t.Cleanup(func() { commentRepositoryStore = newMemoryCommentRepository() })

	replyRequest := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/replies",
		strings.NewReader(`{"author":"回复者","body":"这是一个回复"}`),
	)
	replyRequest.Header.Set("Content-Type", "application/json")
	replyResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(replyResponse, replyRequest)
	if replyResponse.Code != http.StatusCreated {
		t.Fatalf("expected reply status %d, got %d: %s", http.StatusCreated, replyResponse.Code, replyResponse.Body)
	}

	var reply commentResponse
	if err := json.Unmarshal(replyResponse.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Data.ParentID == nil || *reply.Data.ParentID != "comment-atlas-001" || reply.Data.BlockID != "block-atlas-collaboration" {
		t.Fatalf("unexpected reply: %#v", reply.Data)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent/documents/quick-start/comments", nil)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	var listed commentListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode threaded comments: %v", err)
	}
	if len(listed.Data) != 1 || len(listed.Data[0].Replies) != 1 || listed.Data[0].Replies[0].ID != reply.Data.ID {
		t.Fatalf("reply not listed in thread: %#v", listed.Data)
	}
	if listed.Data[0].ReplyCount != 1 {
		t.Fatalf("reply count = %d, want 1", listed.Data[0].ReplyCount)
	}

	editRequest := httptest.NewRequest(http.MethodPatch,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/replies/"+reply.Data.ID,
		strings.NewReader(`{"body":"更新后的回复"}`),
	)
	editResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(editResponse, editRequest)
	if editResponse.Code != http.StatusOK {
		t.Fatalf("expected edit status %d, got %d: %s", http.StatusOK, editResponse.Code, editResponse.Body)
	}
	var edited commentResponse
	if err := json.Unmarshal(editResponse.Body.Bytes(), &edited); err != nil {
		t.Fatalf("decode edited reply: %v", err)
	}
	if edited.Data.Body != "更新后的回复" || edited.Data.UpdatedAt == "" {
		t.Fatalf("unexpected edited reply: %#v", edited.Data)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/replies/"+reply.Data.ID,
		nil,
	)
	deleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d: %s", http.StatusNoContent, deleteResponse.Code, deleteResponse.Body)
	}
}

func TestCreateCommentRejectsUnknownBlock(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/documents/quick-start/comments", strings.NewReader(`{
		"block_id":"block-missing",
		"author":"测试用户",
		"body":"无效锚点"
	}`))
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnprocessableEntity)
	}
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "block_not_found" {
		t.Fatalf("code = %q, want block_not_found", body.Error.Code)
	}
}

// 空 block_id 表示文档级评论：即使文档没有可解析的稳定块，也应能评论成功。
func TestCreateCommentAllowsEmptyBlock(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/documents/quick-start/comments", strings.NewReader(`{
		"block_id":"",
		"author":"测试用户",
		"body":"文档级评论"
	}`))
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	var created commentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.BlockID != "" || created.Data.Body != "文档级评论" {
		t.Fatalf("created comment = %#v", created.Data)
	}
}

func TestCommentMethodAndBodyErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "unsupported method", method: http.MethodDelete, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
		{name: "malformed body", method: http.MethodPost, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments", body: "{", status: http.StatusBadRequest, code: "invalid_body"},
		{name: "missing comment", method: http.MethodPatch, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments/not-found", body: `{"status":"resolved"}`, status: http.StatusNotFound, code: "comment_not_found"},
		{name: "missing reply parent", method: http.MethodPost, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments/not-found/replies", body: `{"author":"测试","body":"回复"}`, status: http.StatusNotFound, code: "comment_not_found"},
		{name: "missing reply", method: http.MethodDelete, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/replies/not-found", status: http.StatusNotFound, code: "reply_not_found"},
		{name: "missing reply update", method: http.MethodPatch, path: "/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/replies/not-found", body: `{"body":"更新"}`, status: http.StatusNotFound, code: "reply_not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			var body errorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Error.Code, test.code)
			}
		})
	}
}

// nilRepliesCommentRepository 模拟 MySQL 仓库的行为：写操作返回的评论不填充 Replies。
// 内存仓库会主动置空数组，因此只有这个桩能覆盖 replies 序列化为 null 的回归。
type nilRepliesCommentRepository struct{}

func (nilRepliesCommentRepository) List(context.Context, string) ([]documentComment, error) {
	return []documentComment{}, nil
}

func (nilRepliesCommentRepository) Create(_ context.Context, comment documentComment) (documentComment, error) {
	comment.Replies = nil
	return comment, nil
}

func (nilRepliesCommentRepository) CreateReply(
	_ context.Context, parentID string, reply documentComment,
) (documentComment, bool, error) {
	reply.ParentID, reply.Replies = &parentID, nil
	return reply, true, nil
}

func (nilRepliesCommentRepository) DeleteComment(context.Context, string, string, string, string) (bool, error) {
	return true, nil
}

func (nilRepliesCommentRepository) DeleteReply(context.Context, string, string, string, string, string) (bool, error) {
	return true, nil
}

func (nilRepliesCommentRepository) UpdateComment(
	_ context.Context, documentID, commentID, _, body, updatedAt string,
) (documentComment, bool, error) {
	return documentComment{
		ID: commentID, DocumentID: documentID, Body: body,
		Status: "open", UpdatedAt: updatedAt, Replies: nil,
	}, true, nil
}

func (nilRepliesCommentRepository) UpdateReply(
	_ context.Context, documentID, parentID, replyID, _, body, updatedAt string,
) (documentComment, bool, error) {
	return documentComment{
		ID: replyID, DocumentID: documentID, ParentID: &parentID, Body: body,
		Status: "open", UpdatedAt: updatedAt, Replies: nil,
	}, true, nil
}

func (nilRepliesCommentRepository) Resolve(
	_ context.Context, documentID, commentID, _, resolvedAt string,
) (documentComment, bool, error) {
	return documentComment{
		ID: commentID, DocumentID: documentID, Status: "resolved",
		ResolvedAt: &resolvedAt, UpdatedAt: resolvedAt, Replies: nil,
	}, true, nil
}

func (nilRepliesCommentRepository) ListAllAdmin(
	_ context.Context, _ string,
) ([]adminCommentRecord, error) {
	return []adminCommentRecord{}, nil
}

func (nilRepliesCommentRepository) SetAdminHidden(
	_ context.Context, _ string, _ bool,
) (bool, error) {
	return false, nil
}

// TestCommentResponsesNeverReturnNullReplies 锁定单条评论响应中的 replies 始终是数组。
// 若序列化为 null，客户端对其调用数组方法会在渲染阶段抛异常并导致页面白屏。
func TestCommentResponsesNeverReturnNullReplies(t *testing.T) {
	originalAuth, originalComments, originalLimiter :=
		authRepositoryStore, commentRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = nilRepliesCommentRepository{}
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, commentRepositoryStore, authRateLimiter =
			originalAuth, originalComments, originalLimiter
	})
	cookie, _ := registerTestUser(t, "null-replies@example.com", "契约用户")
	basePath := "/api/v1/projects/atlas-agent/documents/quick-start/comments"

	assertRepliesIsArray := func(name string, body []byte) {
		t.Helper()
		if strings.Contains(string(body), `"replies":null`) {
			t.Fatalf("%s response serialized replies as null: %s", name, body)
		}
		var payload commentResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		if payload.Data.Replies == nil {
			t.Fatalf("%s replies decoded as nil: %s", name, body)
		}
	}

	send := func(name, method, path, body string, wantStatus int) []byte {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != wantStatus {
			t.Fatalf("%s status = %d, want %d: %s", name, response.Code, wantStatus, response.Body)
		}
		return response.Body.Bytes()
	}

	assertRepliesIsArray("create", send("create", http.MethodPost, basePath,
		`{"block_id":"block-atlas-intro","author":"忽略","body":"契约评论"}`, http.StatusCreated))
	assertRepliesIsArray("edit", send("edit", http.MethodPatch, basePath+"/comment-1",
		`{"body":"编辑后的契约评论"}`, http.StatusOK))
	assertRepliesIsArray("resolve", send("resolve", http.MethodPatch, basePath+"/comment-1",
		`{"status":"resolved"}`, http.StatusOK))
	assertRepliesIsArray("reply", send("reply", http.MethodPost, basePath+"/comment-1/replies",
		`{"author":"忽略","body":"契约回复"}`, http.StatusCreated))
	assertRepliesIsArray("edit reply", send("edit reply", http.MethodPatch,
		basePath+"/comment-1/replies/reply-1", `{"body":"编辑后的契约回复"}`, http.StatusOK))
}
