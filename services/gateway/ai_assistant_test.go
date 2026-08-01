package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AI 助手接口的测试。
// 用一个假的 Anthropic 服务器验证真实的线协议（POST /v1/messages、鉴权头、SSE 转发），
// 而不是只测一个 mock 接口——那样测不出请求写错。

// fakeAnthropic 记录收到的请求，并按预设返回 SSE 或错误。
type fakeAnthropic struct {
	server     *httptest.Server
	lastAPIKey string
	lastBody   string
	// sseBody 是 200 响应的 SSE 正文；failStatus 非零时改为返回该错误状态码。
	sseBody    string
	failStatus int
}

func newFakeAnthropic(t *testing.T, sseBody string, failStatus int) *fakeAnthropic {
	t.Helper()
	fake := &fakeAnthropic{sseBody: sseBody, failStatus: failStatus}
	fake.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, _ := io.ReadAll(request.Body)
		fake.lastAPIKey = request.Header.Get("x-api-key")
		fake.lastBody = string(payload)
		if fake.failStatus != 0 {
			writer.WriteHeader(fake.failStatus)
			_, _ = writer.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"nope"}}`))
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(fake.sseBody))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// cannedAnswerStream 是一段最小但合法的 Anthropic 流式响应。
const cannedAnswerStream = `event: message_start
data: {"type":"message_start"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"这是"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"答案"}}

event: message_stop
data: {"type":"message_stop"}

`

// seedPublishedProject 用内存仓库注入一个已发布项目并返回其 slug。
func seedPublishedProject(slug string) *memoryManagedProjectRepository {
	repository := newMemoryManagedProjectRepository()
	repository.projects["project-1"] = managedProject{
		ID: "project-1", Slug: slug, Name: "示例项目", Status: "published",
		Description: "# 示例项目\n\n这是用于测试的项目文档正文。",
	}
	return repository
}

func postAssistant(t *testing.T, slug string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"question":"这个项目是做什么的？"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+slug+"/assistant", body)
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	return response
}

func TestAssistantRequiresLogin(t *testing.T) {
	originalAuth := authRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	t.Cleanup(func() { authRepositoryStore = originalAuth })

	response := postAssistant(t, "demo", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", response.Code, response.Body)
	}
}

func TestAssistantUnavailableWithoutKey(t *testing.T) {
	originalAuth, originalStore, originalLimiter := authRepositoryStore, aiAssistantStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	aiAssistantStore = nil
	t.Cleanup(func() {
		authRepositoryStore, aiAssistantStore, authRateLimiter = originalAuth, originalStore, originalLimiter
	})

	cookie, _ := registerTestUser(t, "assistant-nokey@example.com", "无密钥用户")
	response := postAssistant(t, "demo", cookie)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "assistant_unavailable") {
		t.Fatalf("body = %s", response.Body)
	}
}

func TestAssistantUnknownProjectReturns404(t *testing.T) {
	originalAuth, originalStore, originalLimiter, originalProjects :=
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	fake := newFakeAnthropic(t, cannedAnswerStream, 0)
	aiAssistantStore = newAnthropicAssistant(fake.server.URL, "test-key", "claude-sonnet-5")
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	t.Cleanup(func() {
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore =
			originalAuth, originalStore, originalLimiter, originalProjects
	})

	cookie, _ := registerTestUser(t, "assistant-404@example.com", "找不到项目")
	response := postAssistant(t, "no-such-project", cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "project_not_found") {
		t.Fatalf("body = %s", response.Body)
	}
}

func TestAssistantStreamsCannedAnswer(t *testing.T) {
	originalAuth, originalStore, originalLimiter, originalProjects :=
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	fake := newFakeAnthropic(t, cannedAnswerStream, 0)
	aiAssistantStore = newAnthropicAssistant(fake.server.URL, "test-key", "claude-sonnet-5")
	managedProjectRepositoryStore = seedPublishedProject("demo")
	t.Cleanup(func() {
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore =
			originalAuth, originalStore, originalLimiter, originalProjects
	})

	cookie, _ := registerTestUser(t, "assistant-stream@example.com", "流式用户")
	response := postAssistant(t, "demo", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", contentType)
	}

	// 转发的 SSE 应包含逐段的文本增量与结束事件。
	answer := collectAssistantDeltas(t, response.Body.String())
	if answer != "这是答案" {
		t.Fatalf("streamed answer = %q, want 这是答案", answer)
	}
	if !strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("missing done event: %s", response.Body)
	}

	// 上游请求必须带上鉴权头与项目文档 grounding 上下文。
	if fake.lastAPIKey != "test-key" {
		t.Fatalf("upstream x-api-key = %q", fake.lastAPIKey)
	}
	if !strings.Contains(fake.lastBody, "测试的项目文档正文") {
		t.Fatalf("upstream body missing grounding context: %s", fake.lastBody)
	}
	if !strings.Contains(fake.lastBody, `"stream":true`) {
		t.Fatalf("upstream request should be streaming: %s", fake.lastBody)
	}
}

func TestAssistantUpstreamErrorReturns502(t *testing.T) {
	originalAuth, originalStore, originalLimiter, originalProjects :=
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	fake := newFakeAnthropic(t, "", http.StatusInternalServerError)
	aiAssistantStore = newAnthropicAssistant(fake.server.URL, "test-key", "claude-sonnet-5")
	managedProjectRepositoryStore = seedPublishedProject("demo")
	t.Cleanup(func() {
		authRepositoryStore, aiAssistantStore, authRateLimiter, managedProjectRepositoryStore =
			originalAuth, originalStore, originalLimiter, originalProjects
	})

	cookie, _ := registerTestUser(t, "assistant-502@example.com", "上游报错")
	response := postAssistant(t, "demo", cookie)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "assistant_upstream_error") {
		t.Fatalf("body = %s", response.Body)
	}
}

// collectAssistantDeltas 从本服务转发的 SSE 中拼出 delta 事件的文本。
func collectAssistantDeltas(t *testing.T, body string) string {
	t.Helper()
	var answer strings.Builder
	for _, line := range strings.Split(body, "\n") {
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		var delta assistantDeltaPayload
		if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &delta); err != nil {
			continue
		}
		answer.WriteString(delta.Text)
	}
	return answer.String()
}
