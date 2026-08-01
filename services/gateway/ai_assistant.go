package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 项目 AI 助手。
//
// 设计取舍：
//   - 后端是 Go，不引入 Python/langchain。这里手写一个极小的 Anthropic Messages API
//     客户端（net/http），与 search.go 的 ES 客户端、object_storage.go 的 OSS 签名
//     同属项目「手写 HTTP 客户端」的既有风格。
//   - 轻量 RAG：取当前项目的已发布文档 Markdown，截断到预算后作为 system 提示中的
//     grounding 上下文，要求模型只依据项目内容作答，不知道就说不知道。
//   - 以 SSE 把模型的增量输出转发给前端（沿用 notifications.go 的 SSE 写法），
//     让聊天窗口逐字呈现，而不是等整段响应。
//   - 未配置 ANTHROPIC_API_KEY 时返回 503 assistant_unavailable（对齐 search_api.go
//     的 search_unavailable），绝不崩溃，也绝不伪造答案。

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	// 默认模型：快且能力足够。可用 ANTHROPIC_MODEL 覆盖。
	defaultAnthropicModel = "claude-sonnet-5"
	anthropicVersion      = "2023-06-01"

	// 单次提问长度上限（字符）。
	maxAssistantQuestionRunes = 2000
	// 携带的历史消息条数上限，以及每条历史的字符上限。
	maxAssistantHistoryMessages = 10
	maxAssistantMessageRunes    = 4000
	// grounding 上下文预算（字节）。项目文档拼接后截断到此长度。
	assistantContextBudget = 48 << 10
	// 模型输出上限。文档问答不需要很长的回答。
	assistantMaxTokens = 2048
	// 整个请求（含上游流式响应）的超时时间。
	assistantRequestTimeout = 60 * time.Second
	// 每个登录用户的调用频率：窗口内最多 assistantRateLimit 次。
	assistantRateLimit  = 20
	assistantRateWindow = time.Minute
)

// anthropicAssistant 是 Anthropic Messages API 的最小流式客户端。
type anthropicAssistant struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// newAnthropicAssistant 构造客户端。不设置 client.Timeout：流式响应的读取时长不可预知，
// 超时统一交给传入的 context 控制。
func newAnthropicAssistant(baseURL, apiKey, model string) *anthropicAssistant {
	return &anthropicAssistant{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

// aiAssistantStore 未配置密钥时为 nil，处理器据此返回 503。
var aiAssistantStore *anthropicAssistant

type anthropicMessage struct {
	Role string `json:"role"`
	// content 用字符串简写形式，Anthropic 接受它作为单个 text block。
	Content string `json:"content"`
}

type anthropicThinking struct {
	Type string `json:"type"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Stream    bool               `json:"stream"`
	Thinking  *anthropicThinking `json:"thinking,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicStreamEvent 只声明我们用到的字段。data 行的 JSON 自带 type，
// 因此无需解析 SSE 的 event 行。
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// stream 发起一次流式请求，返回原始响应交由处理器解析。
func (assistant *anthropicAssistant) stream(
	ctx context.Context, system string, messages []anthropicMessage,
) (*http.Response, error) {
	payload := anthropicRequest{
		Model:     assistant.model,
		MaxTokens: assistantMaxTokens,
		System:    system,
		Stream:    true,
		// 文档问答走「直接作答」路径：关闭思考让首个 token 更快到达，避免聊天窗口长时间空白。
		Thinking: &anthropicThinking{Type: "disabled"},
		Messages: messages,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode assistant request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, assistant.baseURL+"/v1/messages", bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build assistant request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("x-api-key", assistant.apiKey)
	request.Header.Set("anthropic-version", anthropicVersion)
	return assistant.client.Do(request)
}

type assistantChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type assistantRequestBody struct {
	Question string                 `json:"question"`
	History  []assistantChatMessage `json:"history"`
}

func assistantHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	// 基础防滥用：登录用户的每分钟调用上限，复用鉴权限流器。
	allowed, retryAfter, err := authRateLimiter.Allow(
		request.Context(), "assistant:"+user.ID, assistantRateLimit, assistantRateWindow, time.Now())
	if err != nil {
		slog.ErrorContext(request.Context(), "assistant rate limiter failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "assistant_error", "AI 助手暂时不可用")
		return
	}
	if !allowed {
		writer.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeAPIError(writer, request, http.StatusTooManyRequests, "rate_limited", "请求过于频繁，请稍后重试")
		return
	}

	var input assistantRequestBody
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的提问数据")
		return
	}
	question := strings.TrimSpace(input.Question)
	if runes := len([]rune(question)); runes < 1 || runes > maxAssistantQuestionRunes {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_question",
			"问题长度需在 1 到 2000 个字符之间")
		return
	}

	// 未配置密钥时明确告知未启用，而不是笼统的 500，也不伪造答案。
	if aiAssistantStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "assistant_unavailable", "AI 助手暂未启用")
		return
	}

	slug := request.PathValue("slug")
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), slug)
	if err != nil {
		slog.ErrorContext(request.Context(), "assistant project lookup failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}

	system := buildAssistantSystemPrompt(request.Context(), project)
	messages := buildAssistantMessages(input.History, question)

	// 先建立上游连接：状态未落定前不要写 SSE，这样上游报错还能干净地返回 502。
	streamContext, cancel := context.WithTimeout(request.Context(), assistantRequestTimeout)
	defer cancel()
	upstream, err := aiAssistantStore.stream(streamContext, system, messages)
	if err != nil {
		slog.ErrorContext(request.Context(), "assistant upstream request failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusBadGateway, "assistant_upstream_error", "AI 助手暂时不可用")
		return
	}
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(upstream.Body, 2<<10))
		slog.ErrorContext(request.Context(), "assistant upstream returned error",
			"request_id", requestIDFromContext(request.Context()),
			"status", upstream.StatusCode, "body", truncateForLog(snippet))
		writeAPIError(writer, request, http.StatusBadGateway, "assistant_upstream_error", "AI 助手暂时不可用")
		return
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeAPIError(writer, request, http.StatusInternalServerError, "streaming_unsupported", "当前连接不支持实时流式响应")
		return
	}

	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	streamAssistantResponse(request.Context(), writer, flusher, upstream.Body)
}

// streamAssistantResponse 解析 Anthropic 的 SSE，并把文本增量转发为本服务的 SSE 事件：
//
//	event: delta  data: {"text":"..."}
//	event: error  data: {"code":"...","message":"..."}
//	event: done   data: {}
func streamAssistantResponse(
	ctx context.Context, writer http.ResponseWriter, flusher http.Flusher, body io.Reader,
) {
	scanner := bufio.NewScanner(body)
	// SSE 单行一般较短，但个别 data 行可能偏长，放大缓冲避免 ErrTooLong。
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	upstreamFailed := false
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				writeAssistantEvent(writer, flusher, "delta", assistantDeltaPayload{Text: event.Delta.Text})
			}
		case "error":
			upstreamFailed = true
			writeAssistantEvent(writer, flusher, "error",
				assistantErrorPayload{Code: "assistant_upstream_error", Message: "AI 助手生成回答时出错"})
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(ctx, "assistant stream read failed",
			"request_id", requestIDFromContext(ctx), "error", err)
		if !upstreamFailed {
			writeAssistantEvent(writer, flusher, "error",
				assistantErrorPayload{Code: "assistant_upstream_error", Message: "AI 助手连接中断"})
			upstreamFailed = true
		}
	}
	if !upstreamFailed {
		writeAssistantEvent(writer, flusher, "done", struct{}{})
	}
}

type assistantDeltaPayload struct {
	Text string `json:"text"`
}

type assistantErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAssistantEvent(writer http.ResponseWriter, flusher http.Flusher, event string, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, encoded)
	flusher.Flush()
}

// buildAssistantSystemPrompt 组装 system 提示：作答规则 + 项目信息 + 文档 grounding 上下文。
func buildAssistantSystemPrompt(ctx context.Context, project managedProject) string {
	var builder strings.Builder
	builder.WriteString("你是「新猿译码」平台上某个开源项目的 AI 助手。")
	builder.WriteString("请仅依据下方提供的项目资料回答用户关于该项目的问题。")
	builder.WriteString("若资料中没有相关信息，请如实说明你无法从当前项目内容中找到答案，不要编造。")
	builder.WriteString("默认用中文回答；若用户使用其他语言提问，则用用户的语言回答。\n\n")

	builder.WriteString("## 项目信息\n")
	builder.WriteString("名称：" + project.Name + "\n")
	if summary := strings.TrimSpace(project.Summary); summary != "" {
		builder.WriteString("简介：" + summary + "\n")
	}
	builder.WriteString("\n## 项目文档内容\n")
	builder.WriteString(gatherAssistantContext(ctx, project))
	return builder.String()
}

// gatherAssistantContext 拼接项目文档正文，截断到预算内作为 grounding 上下文。
// 复用 search_api.go 里已有的文档收集逻辑（已发布项目的多篇文档，或回退到项目正文）。
func gatherAssistantContext(ctx context.Context, project managedProject) string {
	documents := projectSearchDocuments(ctx, project)
	var builder strings.Builder
	for _, document := range documents {
		title := strings.TrimSpace(document.Title)
		if title == "" {
			title = "（无标题）"
		}
		builder.WriteString("### " + title + "\n")
		builder.WriteString(strings.TrimSpace(document.Body))
		builder.WriteString("\n\n")
		if builder.Len() >= assistantContextBudget {
			break
		}
	}
	content := strings.TrimSpace(builder.String())
	if content == "" {
		return "（该项目暂无文档内容。）"
	}
	return truncateUTF8(content, assistantContextBudget)
}

// buildAssistantMessages 把可选的历史对话与本次提问拼成 Anthropic messages。
// 历史只保留最近若干条、角色合法且内容非空的消息，逐条裁剪长度，最后追加用户提问。
func buildAssistantMessages(history []assistantChatMessage, question string) []anthropicMessage {
	if len(history) > maxAssistantHistoryMessages {
		history = history[len(history)-maxAssistantHistoryMessages:]
	}
	messages := make([]anthropicMessage, 0, len(history)+1)
	for _, entry := range history {
		if entry.Role != "user" && entry.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			continue
		}
		messages = append(messages, anthropicMessage{
			Role:    entry.Role,
			Content: truncateUTF8(content, maxAssistantMessageRunes*4),
		})
	}
	messages = append(messages, anthropicMessage{Role: "user", Content: question})
	return messages
}
