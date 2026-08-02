package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// 跨文档搜索。
//
// 设计取舍：
//   - MySQL 是唯一真相源，ES 只是二级索引。索引写入按 best-effort 处理，
//     ES 故障不能让文档保存失败——否则搜索的可用性会拖累核心写入路径。
//   - 由此带来的代价是最终一致：索引可能短暂落后或漂移，因此提供重建入口。
//   - 用接口抽象而不是直接调 ES：便于在 ES 不可用时降级，也便于将来换实现。
//   - 直接用 net/http 调 REST，不引入 ES 官方客户端。项目既有风格如此
//     （OSS V4 签名也是手写），且我们只用到少量端点。

var errSearchUnavailable = errors.New("search index is not available")

const (
	// 中文用内置 cjk 分析器做 bigram 切分，无需 IK 插件。
	searchAnalyzer = "cjk"
	// 单次搜索返回上限。
	maxSearchResults = 30
	// 索引正文截断长度。搜索只需片段，全文留在 MySQL。
	maxIndexedBodyBytes = 32 << 10
)

// searchDocument 是进入索引的一条记录。
type searchDocument struct {
	// ID 形如 "<projectID>:<documentID>"，文档为空时代表项目正文。
	ID          string `json:"-"`
	ProjectID   string `json:"project_id"`
	ProjectSlug string `json:"project_slug"`
	ProjectName string `json:"project_name"`
	DocumentID  string `json:"document_id"`
	// DocumentSlug 为空时表示项目正文（尚未建文档的项目）。
	DocumentSlug string    `json:"document_slug"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// searchHit 是返回给前端的一条命中。
type searchHit struct {
	ProjectSlug  string `json:"project_slug"`
	ProjectName  string `json:"project_name"`
	DocumentSlug string `json:"document_slug"`
	Title        string `json:"title"`
	// Highlight 是带 <em> 标记的匹配片段，由 ES 生成。
	Highlight []string `json:"highlight"`
	Score     float64  `json:"score"`
	UpdatedAt string   `json:"updated_at"`
}

type searchIndex interface {
	// Ensure 幂等地创建索引与映射。
	Ensure(ctx context.Context) error
	Index(ctx context.Context, document searchDocument) error
	Delete(ctx context.Context, id string) error
	// DeleteByProject 删除某项目下的全部记录，用于项目下架或重建前清理。
	DeleteByProject(ctx context.Context, projectID string) error
	Search(ctx context.Context, query string, limit int) ([]searchHit, error)
	Available() bool
}

// noopSearchIndex 在未配置 ES 时使用。搜索接口据此返回明确的不可用提示，
// 而不是 500，也不会让索引写入路径报错。
type noopSearchIndex struct{}

func (noopSearchIndex) Ensure(context.Context) error                  { return nil }
func (noopSearchIndex) Index(context.Context, searchDocument) error   { return nil }
func (noopSearchIndex) Delete(context.Context, string) error          { return nil }
func (noopSearchIndex) DeleteByProject(context.Context, string) error { return nil }
func (noopSearchIndex) Available() bool                               { return false }
func (noopSearchIndex) Search(context.Context, string, int) ([]searchHit, error) {
	return nil, errSearchUnavailable
}

type elasticSearchIndex struct {
	baseURL   string
	indexName string
	username  string
	password  string
	client    *http.Client
}

func newElasticSearchIndex(baseURL, indexName, username, password string) *elasticSearchIndex {
	return &elasticSearchIndex{
		baseURL:   strings.TrimRight(baseURL, "/"),
		indexName: indexName,
		username:  username,
		password:  password,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (index *elasticSearchIndex) Available() bool { return true }

// do 发起一次 ES 请求。body 为 nil 时不带正文。
func (index *elasticSearchIndex) do(
	ctx context.Context, method, path string, body any,
) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("encode search request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, index.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build search request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if index.username != "" {
		request.SetBasicAuth(index.username, index.password)
	}
	response, err := index.client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("call search index: %w", err)
	}
	defer response.Body.Close()
	// 限制读取量，避免异常响应占满内存。
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return response.StatusCode, nil, fmt.Errorf("read search response: %w", err)
	}
	return response.StatusCode, payload, nil
}

func (index *elasticSearchIndex) Ensure(ctx context.Context) error {
	status, _, err := index.do(ctx, http.MethodHead, "/"+index.indexName, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}

	// 单节点部署：0 副本，否则集群健康会一直停在 yellow。
	mapping := map[string]any{
		"settings": map[string]any{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"project_id":    map[string]any{"type": "keyword"},
				"project_slug":  map[string]any{"type": "keyword"},
				"project_name":  map[string]any{"type": "text", "analyzer": searchAnalyzer},
				"document_id":   map[string]any{"type": "keyword"},
				"document_slug": map[string]any{"type": "keyword"},
				"title":         map[string]any{"type": "text", "analyzer": searchAnalyzer},
				"body":          map[string]any{"type": "text", "analyzer": searchAnalyzer},
				"updated_at":    map[string]any{"type": "date"},
			},
		},
	}
	status, payload, err := index.do(ctx, http.MethodPut, "/"+index.indexName, mapping)
	if err != nil {
		return err
	}
	// 并发启动时可能已被另一个实例建好，视为成功。
	if status == http.StatusOK || bytes.Contains(payload, []byte("resource_already_exists_exception")) {
		return nil
	}
	return fmt.Errorf("create search index: status %d: %s", status, truncateForLog(payload))
}

func (index *elasticSearchIndex) Index(ctx context.Context, document searchDocument) error {
	if document.ID == "" {
		return errors.New("search document id is required")
	}
	document.Body = truncateUTF8(document.Body, maxIndexedBodyBytes)
	status, payload, err := index.do(ctx, http.MethodPut,
		"/"+index.indexName+"/_doc/"+url.PathEscape(document.ID), document)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("index document: status %d: %s", status, truncateForLog(payload))
	}
	return nil
}

func (index *elasticSearchIndex) Delete(ctx context.Context, id string) error {
	status, payload, err := index.do(ctx, http.MethodDelete,
		"/"+index.indexName+"/_doc/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	// 已不存在视为成功，删除应当幂等。
	if status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("delete document: status %d: %s", status, truncateForLog(payload))
	}
	return nil
}

func (index *elasticSearchIndex) DeleteByProject(ctx context.Context, projectID string) error {
	body := map[string]any{
		"query": map[string]any{"term": map[string]any{"project_id": projectID}},
	}
	status, payload, err := index.do(ctx, http.MethodPost,
		"/"+index.indexName+"/_delete_by_query?refresh=true&conflicts=proceed", body)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("delete by project: status %d: %s", status, truncateForLog(payload))
	}
	return nil
}

// elasticSearchResponse 只声明我们用到的字段。
type elasticSearchResponse struct {
	Hits struct {
		Hits []struct {
			Score     float64        `json:"_score"`
			Source    searchDocument `json:"_source"`
			Highlight struct {
				Title []string `json:"title"`
				Body  []string `json:"body"`
			} `json:"highlight"`
		} `json:"hits"`
	} `json:"hits"`
}

func (index *elasticSearchIndex) Search(
	ctx context.Context, query string, limit int,
) ([]searchHit, error) {
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}
	body := map[string]any{
		"size": limit,
		"query": map[string]any{
			"multi_match": map[string]any{
				"query": query,
				// 标题权重高于正文，项目名再低一档。
				"fields": []string{"title^3", "body", "project_name^2"},
			},
		},
		"highlight": map[string]any{
			"pre_tags":  []string{"<em>"},
			"post_tags": []string{"</em>"},
			"fields": map[string]any{
				"title": map[string]any{"number_of_fragments": 0},
				"body":  map[string]any{"fragment_size": 120, "number_of_fragments": 2},
			},
		},
	}
	status, payload, err := index.do(ctx, http.MethodPost,
		"/"+index.indexName+"/_search", body)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// 索引还没建起来时视为空结果，而不是报错。
		return []searchHit{}, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search: status %d: %s", status, truncateForLog(payload))
	}

	var decoded elasticSearchResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	hits := make([]searchHit, 0, len(decoded.Hits.Hits))
	for _, raw := range decoded.Hits.Hits {
		highlight := append([]string{}, raw.Highlight.Title...)
		highlight = append(highlight, raw.Highlight.Body...)
		hits = append(hits, searchHit{
			ProjectSlug:  raw.Source.ProjectSlug,
			ProjectName:  raw.Source.ProjectName,
			DocumentSlug: raw.Source.DocumentSlug,
			Title:        raw.Source.Title,
			Highlight:    highlight,
			Score:        raw.Score,
			UpdatedAt:    raw.Source.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return hits, nil
}

// truncateUTF8 把字符串截到不超过 limit 字节，且不切破多字节字符。
// 直接按字节下标切会把汉字斩成半个，产生乱码并可能让总长度超限。
func truncateUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	// 回退到上一个字符边界。
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// truncateForLog 截断响应正文，避免把整段 ES 错误灌进日志。
func truncateForLog(payload []byte) string {
	const limit = 300
	if len(payload) > limit {
		return string(payload[:limit]) + "…"
	}
	return string(payload)
}

var searchIndexStore searchIndex = noopSearchIndex{}

// searchDocumentID 组装索引主键。文档为空时代表项目正文。
func searchDocumentID(projectID, documentID string) string {
	if documentID == "" {
		return projectID + ":body"
	}
	return projectID + ":" + documentID
}

// indexDocumentBestEffort 异步写索引并吞掉错误。
// 索引失败不应影响文档保存，出错只记日志，由重建入口修复漂移。
func indexDocumentBestEffort(document searchDocument) {
	if !searchIndexStore.Available() {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := searchIndexStore.Index(ctx, document); err != nil {
			slog.Warn("index document failed", "document_id", document.ID, "error", err)
		}
	})
}

// removeFromIndexBestEffort 与 indexDocumentBestEffort 对称。
func removeFromIndexBestEffort(id string) {
	if !searchIndexStore.Available() {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := searchIndexStore.Delete(ctx, id); err != nil {
			slog.Warn("remove indexed document failed", "document_id", id, "error", err)
		}
	})
}

var _ searchIndex = noopSearchIndex{}
var _ searchIndex = (*elasticSearchIndex)(nil)
