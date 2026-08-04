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
	// 精确字段用 cjk（只 bigram），召回子字段用 cjk_loose（额外输出单字）。
	// 少了 output_unigrams，「猫」就永远搜不到「猫咪」，这是必须守住的点。
	for _, expected := range []string{
		`"analyzer":"cjk"`,
		`"analyzer":"cjk_loose"`,
		`"output_unigrams":true`,
		`"type":"cjk_bigram"`,
		`"cjk_bigram_loose"`,
		`"asciifolding"`,
		`"number_of_replicas":0`,
		`"type":"keyword"`,
		// 按标签/分类/简介搜索要能命中，这三个字段必须进索引。
		`"summary"`, `"category"`, `"tags"`,
		// 映射版本用于发现线上索引是否还是老映射。
		`"mapping_version":2`,
	} {
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
	// 索引已存在时不能擅自删库重建，只允许探测 + 读映射版本（用于落后告警）。
	requests := fake.captured()
	for _, request := range requests {
		if request.Method == http.MethodPut || request.Method == http.MethodDelete {
			t.Fatalf("existing index must not be modified by Ensure: %#v", requests)
		}
	}
	if requests[0].Method != http.MethodHead {
		t.Fatalf("first request should probe the index: %#v", requests[0])
	}
}

// TestElasticIndexRecreateDeletesThenCreates 验证重建会先删再建。
// 分析器改动对已存在的索引无效，不先删就等于没改。
func TestElasticIndexRecreateDeletesThenCreates(t *testing.T) {
	fake := newFakeElastic(t)
	fake.indexExists = true
	if err := fake.index(t).Recreate(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests := fake.captured()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2 (DELETE + PUT): %#v", len(requests), requests)
	}
	if requests[0].Method != http.MethodDelete || requests[0].Path != "/test-index" {
		t.Fatalf("first request should delete the index: %#v", requests[0])
	}
	if requests[1].Method != http.MethodPut || requests[1].Path != "/test-index" {
		t.Fatalf("second request should recreate the index: %#v", requests[1])
	}
	if !strings.Contains(requests[1].Body, `"analyzer":"cjk_loose"`) {
		t.Fatalf("recreate should use the current mapping: %s", requests[1].Body)
	}
}

// TestElasticIndexRecreateToleratesMissingIndex 索引本来就不存在时删除返回 404，应继续建。
func TestElasticIndexRecreateToleratesMissingIndex(t *testing.T) {
	fake := newFakeElastic(t)
	fake.indexExists = false
	if err := fake.index(t).Recreate(context.Background()); err != nil {
		t.Fatalf("recreate on missing index should succeed: %v", err)
	}
}

func TestElasticIndexSearchBuildsWeightedQuery(t *testing.T) {
	fake := newFakeElastic(t)
	fake.searchBody = `{"hits":{"hits":[{
		"_score": 4.5,
		"_source": {
			"project_id":"project-1","project_slug":"demo","project_name":"演示项目",
			"document_id":"document-1","document_slug":"guide","title":"使用指南",
			"body":"正文内容","summary":"项目简介","category":"Coding Agent",
			"tags":["Agent"],"updated_at":"2026-08-01T00:00:00Z"
		},
		"highlight": {
			"title":["<em>使用</em>指南"],
			"title.loose":["<em>使用</em>指南"],
			"body":["匹配到的<em>正文</em>片段"]
		}
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
	// 精确字段与 .loose 子字段返回了同一个片段，必须去重，
	// 否则同一句话会在结果卡片里出现两遍。
	if len(hit.Highlight) != 2 || !strings.Contains(hit.Highlight[0], "<em>") {
		t.Fatalf("highlight should merge and dedupe fragments: %#v", hit.Highlight)
	}
	if hit.Score != 4.5 || hit.UpdatedAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("score/updated = %v %q", hit.Score, hit.UpdatedAt)
	}
	// 前端按项目分组时要用这两项做组标题。
	if hit.ProjectSummary != "项目简介" || hit.Category != "Coding Agent" {
		t.Fatalf("hit group fields = %#v", hit)
	}

	body := fake.captured()[0].Body
	for _, expected := range []string{
		// 短语命中排最前。
		`"type":"phrase"`,
		// 精确字段权重高于对应的 .loose 子字段，保证排序质量。
		`"title^4"`, `"title.loose^1.2"`,
		`"project_name^3"`, `"body.loose^0.4"`,
		// 标签与分类可搜。
		`"tags^3"`, `"category^2"`,
		// 拉丁文错拼容忍。
		`"fuzziness":"AUTO"`,
		// 命中项目名或简介时也要有高亮片段。
		`"project_name"`, `"summary"`,
		`"highlight"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("query missing %s: %s", expected, body)
		}
	}
	// 刻意不设百分比门槛：单字查询「猫」只有一个 token，
	// 任何百分比门槛都会把它的兜底召回一起挡掉。
	if strings.Contains(body, `"minimum_should_match":"`) {
		t.Fatalf("query must not set a percentage minimum_should_match: %s", body)
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

// TestSearchRecordsCarryProjectMetadata 验证索引记录带上项目的简介/分类/标签。
// 少了这三项，按标签或分类搜索就是静默失效——接口 200、结果永远为空。
func TestSearchRecordsCarryProjectMetadata(t *testing.T) {
	project := managedProject{
		ID: "project-1", Slug: "demo", Name: "演示项目",
		Summary: "一个用于验证搜索的示例项目", Description: "# 标题\n\n项目正文。",
		Category: "Coding Agent", Tags: []string{"Agent", "检索"},
		UpdatedAt: time.Now().UTC(),
	}

	body := projectBodySearchDocument(project)
	if body.Summary != project.Summary || body.Category != project.Category {
		t.Fatalf("project body record = %#v", body)
	}
	if len(body.Tags) != 2 || body.Tags[0] != "Agent" {
		t.Fatalf("project body tags = %#v", body.Tags)
	}

	// 文档记录也要继承项目元信息，否则按标签搜索搜不到项目下的文档。
	record := projectDocumentSearchRecord(project, projectDocument{
		ID: "document-1", Slug: "guide", Title: "使用指南",
		Markdown: "正文", UpdatedAt: time.Now().UTC(),
	})
	if record.Summary != project.Summary || record.Category != project.Category {
		t.Fatalf("document record = %#v", record)
	}
	if len(record.Tags) != 2 {
		t.Fatalf("document record tags = %#v", record.Tags)
	}
}

// TestSearchQueryClauseKeepsSingleCharRecall 用真实的分析器语义反证查询结构。
//
// 这条守的是本次修复的核心：单字查询「猫」经 cjk_loose 只切出一个 token，
// 必须靠 .loose 字段才能命中只含 bigram「猫咪」的文档；
// 任何 minimum_should_match 百分比门槛都会把这条兜底召回挡掉。
func TestSearchQueryClauseKeepsSingleCharRecall(t *testing.T) {
	encoded, err := json.Marshal(searchQueryClause("猫"))
	if err != nil {
		t.Fatal(err)
	}
	clause := string(encoded)
	if strings.Contains(clause, `"minimum_should_match":"`) {
		t.Fatalf("百分比门槛会挡掉单字召回: %s", clause)
	}
	if !strings.Contains(clause, `"minimum_should_match":1`) {
		t.Fatalf("外层 bool 应要求至少命中一条 should: %s", clause)
	}
	// .loose 子字段是单字召回的唯一来源，权重必须低于精确字段。
	if !strings.Contains(clause, `"title.loose^1.2"`) || !strings.Contains(clause, `"title^4"`) {
		t.Fatalf("缺少精确/召回双字段: %s", clause)
	}
}

// TestSearchQueryClauseRequiresAllLooseTokens 守一个踩过的坑。
//
// .loose 字段会把中文拆到单字。如果这一段用 or，查询「凤凰于飞」里的「于」
// 是极常见字，会把「用于验证跨文档搜索的临时项目」这类毫不相关的内容全召回
// （上线验证时实测到 4 条假命中）。必须是 and：
// 单字查询只有一个 token，照样能命中；长查询则要求全部字都出现。
func TestSearchQueryClauseRequiresAllLooseTokens(t *testing.T) {
	encoded, err := json.Marshal(searchQueryClause("凤凰于飞"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Bool struct {
			Should []struct {
				MultiMatch struct {
					Fields   []string `json:"fields"`
					Operator string   `json:"operator"`
					Type     string   `json:"type"`
				} `json:"multi_match"`
			} `json:"should"`
		} `json:"bool"`
	}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatal(err)
	}
	looseFound := false
	for _, clause := range parsed.Bool.Should {
		isLoose := false
		for _, field := range clause.MultiMatch.Fields {
			if strings.Contains(field, ".loose") {
				isLoose = true
				break
			}
		}
		if !isLoose {
			continue
		}
		looseFound = true
		if clause.MultiMatch.Operator != "and" {
			t.Fatalf(".loose 段的 operator = %q，必须是 and，否则常见单字会把无关内容全召回",
				clause.MultiMatch.Operator)
		}
		// 精确字段不能混进这一段：它们该走 or，混在 and 里会误杀正常召回。
		for _, field := range clause.MultiMatch.Fields {
			if !strings.Contains(field, ".loose") {
				t.Fatalf(".loose 段不应混入精确字段 %q", field)
			}
		}
	}
	if !looseFound {
		t.Fatal("查询里没有 .loose 兜底段，单字查询会搜不到东西")
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

// TestSearchReindexRecreateFlag 验证只有带 recreate=1 才会删索引重建。
// 默认路径必须不删：日常修漂移不该顺手清空索引。
func TestSearchReindexRecreateFlag(t *testing.T) {
	setup := func(t *testing.T) (*fakeElastic, *http.Cookie) {
		t.Helper()
		// 管理员身份由 ADMIN_EMAILS 白名单现算，不是库里的标记。
		t.Setenv("ADMIN_EMAILS", "admin@example.com")
		originalAuth, originalIndex, originalProjects, originalLimiter :=
			authRepositoryStore, searchIndexStore, managedProjectRepositoryStore, authRateLimiter
		authRepositoryStore = newMemoryAuthRepository()
		managedProjectRepositoryStore = newMemoryManagedProjectRepository()
		authRateLimiter = newFixedWindowLimiter()
		fake := newFakeElastic(t)
		fake.indexExists = true
		searchIndexStore = fake.index(t)
		t.Cleanup(func() {
			authRepositoryStore, searchIndexStore, managedProjectRepositoryStore, authRateLimiter =
				originalAuth, originalIndex, originalProjects, originalLimiter
		})
		cookie, _ := registerTestUser(t, "admin@example.com", "管理员")
		return fake, cookie
	}
	deletes := func(fake *fakeElastic) int {
		count := 0
		for _, request := range fake.captured() {
			if request.Method == http.MethodDelete && request.Path == "/test-index" {
				count++
			}
		}
		return count
	}

	t.Run("默认不删索引", func(t *testing.T) {
		fake, cookie := setup(t)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search/reindex", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if deletes(fake) != 0 {
			t.Fatalf("默认路径不该删索引: %#v", fake.captured())
		}
		if !strings.Contains(response.Body.String(), `"recreated":false`) {
			t.Fatalf("body = %s", response.Body)
		}
	})

	t.Run("recreate=1 先删再建", func(t *testing.T) {
		fake, cookie := setup(t)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search/reindex?recreate=1", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body)
		}
		if deletes(fake) != 1 {
			t.Fatalf("recreate 应该删一次索引: %#v", fake.captured())
		}
		if !strings.Contains(response.Body.String(), `"recreated":true`) {
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
