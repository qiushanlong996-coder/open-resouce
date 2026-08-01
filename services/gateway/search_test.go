package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// 搜索索引与搜索接口的测试。
// 用假 ES 服务器验证真实的线协议（请求路径、方法、查询体、鉴权头），
// 而不是只测一个 mock 接口——那样测不出协议写错。

// fakeElastic 记录收到的请求，并按预设返回响应。
type fakeElastic struct {
	sync.Mutex
	server   *httptest.Server
	requests []fakeElasticRequest
	// searchBody 是 _search 的返回体；为空时返回空命中。
	searchBody string
	// indexExists 控制 HEAD /index 的返回。
	indexExists bool
	// failStatus 非零时所有写操作返回该状态码。
	failStatus int
}

type fakeElasticRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

func newFakeElastic(t *testing.T) *fakeElastic {
	t.Helper()
	fake := &fakeElastic{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, _ := io.ReadAll(request.Body)
		username, _, _ := request.BasicAuth()
		fake.Lock()
		fake.requests = append(fake.requests, fakeElasticRequest{
			Method: request.Method, Path: request.URL.Path,
			Query: request.URL.RawQuery, Auth: username, Body: string(payload),
		})
		searchBody, indexExists, failStatus := fake.searchBody, fake.indexExists, fake.failStatus
		fake.Unlock()

		switch {
		case request.Method == http.MethodHead:
			if indexExists {
				writer.WriteHeader(http.StatusOK)
			} else {
				writer.WriteHeader(http.StatusNotFound)
			}
		case strings.HasSuffix(request.URL.Path, "/_search"):
			writer.Header().Set("Content-Type", "application/json")
			if searchBody == "" {
				searchBody = `{"hits":{"hits":[]}}`
			}
			_, _ = writer.Write([]byte(searchBody))
		default:
			if failStatus != 0 {
				writer.WriteHeader(failStatus)
				_, _ = writer.Write([]byte(`{"error":"forced failure"}`))
				return
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"acknowledged":true}`))
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *fakeElastic) index(t *testing.T) *elasticSearchIndex {
	t.Helper()
	return newElasticSearchIndex(fake.server.URL, "test-index", "elastic", "secret")
}

func (fake *fakeElastic) captured() []fakeElasticRequest {
	fake.Lock()
	defer fake.Unlock()
	return append([]fakeElasticRequest(nil), fake.requests...)
}

func TestElasticIndexEnsureCreatesMappingWithCJKAnalyzer(t *testing.T) {
	fake := newFakeElastic(t)
	index := fake.index(t)

	if err := index.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests := fake.captured()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2 (HEAD 探测 + PUT 创建): %#v", len(requests), requests)
	}
	if requests[0].Method != http.MethodHead || requests[0].Path != "/test-index" {
		t.Fatalf("probe request = %#v", requests[0])
	}
	create := requests[1]
	if create.Method != http.MethodPut || create.Path != "/test-index" {
		t.Fatalf("create request = %#v", create)
	}
	// 中文检索依赖 cjk 分析器，映射里必须带上。
	for _, expected := range []string{`"analyzer":"cjk"`, `"number_of_replicas":0`, `"type":"keyword"`} {
		if !strings.Contains(create.Body, expected) {
			t.Fatalf("mapping missing %s: %s", expected, create.Body)
		}
	}
	if create.Auth != "elastic" {
		t.Fatalf("basic auth user = %q", create.Auth)
	}
}

func TestElasticIndexEnsureSkipsWhenIndexExists(t *testing.T) {
	fake := newFakeElastic(t)
	fake.indexExists = true
	if err := fake.index(t).Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests := fake.captured(); len(requests) != 1 {
		t.Fatalf("existing index should only be probed, got %#v", requests)
	}
}

func TestElasticIndexSearchBuildsWeightedQuery(t *testing.T) {
	fake := newFakeElastic(t)
	fake.searchBody = `{"hits":{"hits":[{
		"_score": 4.5,
		"_source": {
			"project_id":"project-1","project_slug":"demo","project_name":"演示项目",
			"document_id":"document-1","document_slug":"guide","title":"使用指南",
			"body":"正文内容","updated_at":"2026-08-01T00:00:00Z"
		},
		"highlight": {"title":["<em>使用</em>指南"],"body":["匹配到的<em>正文</em>片段"]}
	}]}}`

	hits, err := fake.index(t).Search(context.Background(), "使用", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	hit := hits[0]
	if hit.ProjectSlug != "demo" || hit.DocumentSlug != "guide" || hit.Title != "使用指南" {
		t.Fatalf("hit = %#v", hit)
	}
	// 高亮片段来自 title 与 body 两处，都要带上。
	if len(hit.Highlight) != 2 || !strings.Contains(hit.Highlight[0], "<em>") {
		t.Fatalf("highlight = %#v", hit.Highlight)
	}
	if hit.Score != 4.5 || hit.UpdatedAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("score/updated = %v %q", hit.Score, hit.UpdatedAt)
	}

	body := fake.captured()[0].Body
	// 标题权重必须高于正文，否则搜索相关度会明显变差。
	for _, expected := range []string{`"title^3"`, `"project_name^2"`, `"multi_match"`, `"highlight"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("query missing %s: %s", expected, body)
		}
	}
}

func TestElasticIndexSearchTreatsMissingIndexAsEmpty(t *testing.T) {
	// 索引尚未建立时不应报错，返回空结果即可。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	hits, err := newElasticSearchIndex(server.URL, "missing", "", "").
		Search(context.Background(), "任意", 10)
	if err != nil {
		t.Fatalf("missing index should not error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %#v", hits)
	}
}

func TestElasticIndexDeleteIsIdempotent(t *testing.T) {
	// 删除不存在的文档返回 404，应视为成功。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	if err := newElasticSearchIndex(server.URL, "test", "", "").
		Delete(context.Background(), "project-1:document-1"); err != nil {
		t.Fatalf("delete missing document should succeed: %v", err)
	}
}

func TestElasticIndexTruncatesLongBody(t *testing.T) {
	fake := newFakeElastic(t)
	// 用汉字（3 字节/字）填到刚好越过上限，可暴露按字节切断的 bug：
	// 上限不是 3 的倍数时，直接切会斩碎字符并让长度反而超限。
	long := strings.Repeat("字", maxIndexedBodyBytes)
	err := fake.index(t).Index(context.Background(), searchDocument{
		ID: "project-1:document-1", Body: long, Title: "长文档",
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded searchDocument
	if err := json.Unmarshal([]byte(fake.captured()[0].Body), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Body) > maxIndexedBodyBytes {
		t.Fatalf("indexed body = %d bytes, want <= %d", len(decoded.Body), maxIndexedBodyBytes)
	}
	// 截断不能产生乱码。
	if !utf8.ValidString(decoded.Body) {
		t.Fatal("truncated body is not valid UTF-8（汉字被切碎了）")
	}
	if strings.ContainsRune(decoded.Body, utf8.RuneError) {
		t.Fatal("truncated body contains replacement characters")
	}
}

func TestSearchHandlerValidatesQuery(t *testing.T) {
	original := searchIndexStore
	fake := newFakeElastic(t)
	searchIndexStore = fake.index(t)
	t.Cleanup(func() { searchIndexStore = original })

	for name, testCase := range map[string]struct {
		query  string
		status int
	}{
		"空关键词":  {"", http.StatusUnprocessableEntity},
		"仅空白":   {"%20%20", http.StatusUnprocessableEntity},
		"超长关键词": {strings.Repeat("a", 101), http.StatusUnprocessableEntity},
		"正常关键词": {"知识库", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+testCase.query, nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != testCase.status {
			t.Fatalf("%s status = %d, want %d: %s", name, response.Code, testCase.status, response.Body)
		}
	}
}

func TestSearchHandlerRejectsInvalidLimit(t *testing.T) {
	original := searchIndexStore
	fake := newFakeElastic(t)
	searchIndexStore = fake.index(t)
	t.Cleanup(func() { searchIndexStore = original })

	for _, limit := range []string{"0", "-1", "abc"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=测试&limit="+limit, nil)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("limit=%q status = %d, want 422", limit, response.Code)
		}
	}
}

// TestSearchHandlerReportsUnavailableInsteadOf500 未配置 ES 时应给出明确提示。
func TestSearchHandlerReportsUnavailableInsteadOf500(t *testing.T) {
	original := searchIndexStore
	searchIndexStore = noopSearchIndex{}
	t.Cleanup(func() { searchIndexStore = original })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=测试", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "search_unavailable") {
		t.Fatalf("body = %s", response.Body)
	}
}

func TestSearchHandlerOnlyAcceptsGet(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/search?q=测试", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
}

func TestSearchReindexRequiresAdmin(t *testing.T) {
	originalAuth, originalIndex, originalLimiter := authRepositoryStore, searchIndexStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	fake := newFakeElastic(t)
	searchIndexStore = fake.index(t)
	t.Cleanup(func() {
		authRepositoryStore, searchIndexStore, authRateLimiter = originalAuth, originalIndex, originalLimiter
	})

	t.Run("匿名被拒", func(t *testing.T) {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/search/reindex", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("普通用户被拒", func(t *testing.T) {
		cookie, _ := registerTestUser(t, "search-user@example.com", "普通用户")
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search/reindex", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", response.Code, response.Body)
		}
	})

	t.Run("管理员可重建", func(t *testing.T) {
		// 管理员身份由 ADMIN_EMAILS 白名单现算，
		// 必须用 requireAdminUser 而不是直读 authUser.IsAdmin。
		t.Setenv("ADMIN_EMAILS", "admin@example.com")
		cookie, _ := registerTestUser(t, "admin@example.com", "管理员")
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search/reindex", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("admin status = %d, want 200: %s", response.Code, response.Body)
		}
		if !strings.Contains(response.Body.String(), `"indexed"`) {
			t.Fatalf("body = %s", response.Body)
		}
	})
}

// TestSyncDocumentIndexSkipsUnpublishedProject 草稿不应进入公开搜索。
func TestSyncDocumentIndexSkipsUnpublishedProject(t *testing.T) {
	original := searchIndexStore
	fake := newFakeElastic(t)
	searchIndexStore = fake.index(t)
	t.Cleanup(func() { searchIndexStore = original })

	draft := managedProject{ID: "project-draft", Slug: "draft", Name: "草稿项目", Status: "draft"}
	syncDocumentIndex(draft, projectDocument{ID: "document-1", Slug: "guide", Title: "指南"})
	// 索引写入是异步的，留一点时间确认确实没有请求发出。
	time.Sleep(200 * time.Millisecond)
	if requests := fake.captured(); len(requests) != 0 {
		t.Fatalf("draft project must not be indexed, got %#v", requests)
	}

	published := managedProject{ID: "project-live", Slug: "live", Name: "已发布项目", Status: "published"}
	syncDocumentIndex(published, projectDocument{ID: "document-1", Slug: "guide", Title: "指南"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fake.captured()) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("published project document should be indexed")
}

func TestSearchDocumentIDDistinguishesBodyAndDocuments(t *testing.T) {
	if searchDocumentID("project-1", "") == searchDocumentID("project-1", "document-1") {
		t.Fatal("项目正文与文档不能共用索引主键")
	}
	if searchDocumentID("project-1", "document-1") == searchDocumentID("project-2", "document-1") {
		t.Fatal("不同项目下的同名文档不能共用索引主键")
	}
}
