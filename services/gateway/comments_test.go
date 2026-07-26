package main

import (
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
