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
	// searchAnalyzer 是精确字段用的内置 cjk 分析器：只输出 bigram，无需 IK 插件。
	//「猫咪管理系统」→ 猫咪/咪管/管理/理系/系统。
	searchAnalyzer = "cjk"
	// searchLooseAnalyzer 是召回字段用的分析器：单字 + bigram 都输出。
	//
	// 只有 bigram 时，查询「猫」切出单字 token 猫，而索引里只有 bigram 猫咪，
	// 两者永不相交——凡是查询词的切分与正文不对齐（单字、错位、跨词）就一律 0 命中。
	// 这是「必须精准匹配」的根因。
	//
	// 但也不能只留单字：那样「怎么部署」会命中任何含 怎/么/部/署 的文档，噪声极大。
	// 所以精确与召回分成两个字段，各自用各自的分析器，靠权重拉开差距。
	searchLooseAnalyzer = "cjk_loose"
	// searchMappingVersion 随映射或分析器变化递增。
	// 分析器改动对已存在的索引无效，靠它识别线上是否还是老映射（需要 recreate 重建）。
	searchMappingVersion = 2
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
	DocumentSlug string `json:"document_slug"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	// 以下三项是所属项目的元信息。文档记录也带上，
	// 这样按标签或分类搜索能把项目下的文档一并召回。
	Summary   string    `json:"summary"`
	Category  string    `json:"category"`
	Tags      []string  `json:"tags"`
	UpdatedAt time.Time `json:"updated_at"`
}

// searchHit 是返回给前端的一条命中。
type searchHit struct {
	ProjectSlug  string `json:"project_slug"`
	ProjectName  string `json:"project_name"`
	DocumentSlug string `json:"document_slug"`
	Title        string `json:"title"`
	// ProjectSummary 与 Category 供前端把命中按项目分组时显示组标题。
	ProjectSummary string `json:"project_summary,omitempty"`
	Category       string `json:"category,omitempty"`
	// Highlight 是带 <em> 标记的匹配片段，由 ES 生成。
	Highlight []string `json:"highlight"`
	Score     float64  `json:"score"`
	UpdatedAt string   `json:"updated_at"`
}

type searchIndex interface {
	// Ensure 幂等地创建索引与映射。
	Ensure(ctx context.Context) error
	// Recreate 删除并按当前映射重建索引。
	// 分析器与字段映射改动对已存在的索引无效，只能重建。
	Recreate(ctx context.Context) error
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
func (noopSearchIndex) Recreate(context.Context) error                { return nil }
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

// searchIndexMapping 返回索引的 settings 与 mappings。
//
// 每个可搜文本字段都是「精确主字段 + .loose 召回子字段」的组合：
// 主字段只切 bigram，权重高；.loose 子字段额外输出单字，权重低，只负责兜底召回。
// 单靠 bigram 会让「猫」搜不到「猫咪」，单靠单字又会让「怎么部署」命中一切，
// 拆成两个字段才能同时要到召回和排序质量。
func searchIndexMapping() map[string]any {
	// 与内置 cjk 分析器同构，只把 cjk_bigram 的 output_unigrams 打开。
	// 顺带加 asciifolding，让拉丁文的重音折叠（café ↔ cafe）也能命中。
	looseText := func() map[string]any {
		return map[string]any{
			"type":     "text",
			"analyzer": searchAnalyzer,
			"fields": map[string]any{
				"loose": map[string]any{"type": "text", "analyzer": searchLooseAnalyzer},
			},
		}
	}
	return map[string]any{
		"settings": map[string]any{
			// 单节点部署：0 副本，否则集群健康会一直停在 yellow。
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"analysis": map[string]any{
				"filter": map[string]any{
					"cjk_bigram_loose": map[string]any{
						"type":            "cjk_bigram",
						"output_unigrams": true,
					},
				},
				"analyzer": map[string]any{
					searchLooseAnalyzer: map[string]any{
						"tokenizer": "standard",
						"filter": []string{
							"cjk_width", "lowercase", "cjk_bigram_loose", "asciifolding",
						},
					},
				},
			},
		},
		"mappings": map[string]any{
			// 记录映射版本，便于发现线上索引还是老映射。
			"_meta": map[string]any{"mapping_version": searchMappingVersion},
			"properties": map[string]any{
				"project_id":    map[string]any{"type": "keyword"},
				"project_slug":  map[string]any{"type": "keyword"},
				"document_id":   map[string]any{"type": "keyword"},
				"document_slug": map[string]any{"type": "keyword"},
				"project_name":  looseText(),
				"title":         looseText(),
				"body":          looseText(),
				"summary":       looseText(),
				// 分类与标签是短文本，既要能分词搜也要能精确过滤。
				"category": map[string]any{
					"type": "text", "analyzer": searchAnalyzer,
					"fields": map[string]any{
						"loose": map[string]any{"type": "text", "analyzer": searchLooseAnalyzer},
						"raw":   map[string]any{"type": "keyword"},
					},
				},
				"tags": map[string]any{
					"type": "text", "analyzer": searchAnalyzer,
					"fields": map[string]any{
						"loose": map[string]any{"type": "text", "analyzer": searchLooseAnalyzer},
						"raw":   map[string]any{"type": "keyword"},
					},
				},
				"updated_at": map[string]any{"type": "date"},
			},
		},
	}
}

func (index *elasticSearchIndex) Ensure(ctx context.Context) error {
	status, _, err := index.do(ctx, http.MethodHead, "/"+index.indexName, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		// 索引已存在。分析器改动无法原地生效，所以这里只提醒，不擅自删库重建：
		// 重建要清空再灌数据，得由管理员显式触发（reindex?recreate=1）。
		index.warnOnStaleMapping(ctx)
		return nil
	}

	status, payload, err := index.do(ctx, http.MethodPut, "/"+index.indexName, searchIndexMapping())
	if err != nil {
		return err
	}
	// 并发启动时可能已被另一个实例建好，视为成功。
	if status == http.StatusOK || bytes.Contains(payload, []byte("resource_already_exists_exception")) {
		return nil
	}
	return fmt.Errorf("create search index: status %d: %s", status, truncateForLog(payload))
}

// warnOnStaleMapping 检查线上索引的映射版本，落后就打一条 warn。
// 让「代码升级了但索引还是老映射、召回没变好」这件事可被发现，而不是静默降级。
func (index *elasticSearchIndex) warnOnStaleMapping(ctx context.Context) {
	status, payload, err := index.do(ctx, http.MethodGet, "/"+index.indexName+"/_mapping", nil)
	if err != nil || status != http.StatusOK {
		return
	}
	var decoded map[string]struct {
		Mappings struct {
			Meta struct {
				MappingVersion int `json:"mapping_version"`
			} `json:"_meta"`
		} `json:"mappings"`
	}
	if json.Unmarshal(payload, &decoded) != nil {
		return
	}
	for _, entry := range decoded {
		if entry.Mappings.Meta.MappingVersion < searchMappingVersion {
			slog.WarnContext(ctx, "search index mapping is stale; run reindex with recreate=1",
				"index", index.indexName,
				"index_version", entry.Mappings.Meta.MappingVersion,
				"code_version", searchMappingVersion)
		}
	}
}

// Recreate 删除并按当前映射重建索引。
// 分析器与字段映射的改动对已存在的索引无效，只能重建；
// MySQL 是唯一真相源，索引是二级派生物，因此重建没有数据风险。
func (index *elasticSearchIndex) Recreate(ctx context.Context) error {
	status, payload, err := index.do(ctx, http.MethodDelete, "/"+index.indexName, nil)
	if err != nil {
		return err
	}
	// 404 表示索引本来就不存在，继续建即可。
	if status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("delete search index: status %d: %s", status, truncateForLog(payload))
	}
	status, payload, err = index.do(ctx, http.MethodPut, "/"+index.indexName, searchIndexMapping())
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("recreate search index: status %d: %s", status, truncateForLog(payload))
	}
	return nil
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
				Title            []string `json:"title"`
				TitleLoose       []string `json:"title.loose"`
				ProjectName      []string `json:"project_name"`
				ProjectNameLoose []string `json:"project_name.loose"`
				Tags             []string `json:"tags"`
				Category         []string `json:"category"`
				Summary          []string `json:"summary"`
				SummaryLoose     []string `json:"summary.loose"`
				Body             []string `json:"body"`
				BodyLoose        []string `json:"body.loose"`
			} `json:"highlight"`
		} `json:"hits"`
	} `json:"hits"`
}

// searchQueryClause 组装检索语句：召回优先，精确置顶。
//
// 四段 should 各司其职：
//  1. 短语匹配，权重最高——完整词组命中的排最前；
//  2. 精确字段（只切 bigram）的 or 匹配，负责主召回；
//  3. .loose 字段（额外输出单字）的 and 匹配，负责短词兜底；
//  4. 拉丁字段的模糊匹配，容忍英文错拼。
//
// 第 3 段必须是 and，不能是 or。这一点踩过坑：
// 用 or 时查询「凤凰于飞」会被拆成单字，其中「于」是极常见字，
// 于是「用于验证跨文档搜索的临时项目」之类毫不相关的内容全被召回。
// 改成 and 后要求全部 token 都出现在同一字段里：
// 单字查询「猫」只有一个 token，照样能命中「猫咪」（这是本次要修的核心）；
// 而「凤凰于飞」要求 凤/凰/于/飞 全中，噪声随之消失。
// 代价是「小猫」不再能命中「猫咪」——用精确度换掉一个附带效果，值得。
//
// 也刻意不设 minimum_should_match 百分比门槛：那会把单字兜底一起挡掉。
func searchQueryClause(query string) map[string]any {
	return map[string]any{
		"bool": map[string]any{
			"should": []any{
				map[string]any{"multi_match": map[string]any{
					"query": query,
					"type":  "phrase",
					"fields": []string{
						"title^6", "project_name^5", "summary^3", "body^2",
					},
					"boost": 4,
				}},
				map[string]any{"multi_match": map[string]any{
					"query":    query,
					"type":     "best_fields",
					"operator": "or",
					"fields": []string{
						"title^4", "project_name^3", "tags^3", "category^2", "summary^2", "body",
					},
				}},
				map[string]any{"multi_match": map[string]any{
					"query": query,
					"type":  "best_fields",
					// 见上：这里是 and，改成 or 会让常见单字把无关内容全捞出来。
					"operator": "and",
					"fields": []string{
						"title.loose^1.2", "project_name.loose^1", "tags.loose^1",
						"category.loose^0.8", "summary.loose^0.6", "body.loose^0.4",
					},
				}},
				map[string]any{"multi_match": map[string]any{
					"query":     query,
					"fields":    []string{"title^2", "project_name^2", "tags", "body"},
					"fuzziness": "AUTO",
					// 首字母不参与模糊，避免「猫」这类单字查询被改写成无关词。
					"prefix_length": 1,
					"boost":         0.5,
				}},
			},
			"minimum_should_match": 1,
		},
	}
}

func (index *elasticSearchIndex) Search(
	ctx context.Context, query string, limit int,
) ([]searchHit, error) {
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}
	body := map[string]any{
		"size":  limit,
		"query": searchQueryClause(query),
		"highlight": map[string]any{
			"pre_tags":  []string{"<em>"},
			"post_tags": []string{"</em>"},
			// 命中可能来自任一字段，都要能高亮，否则会出现
			//「搜到了但看不出为什么搜到」——例如只匹配项目名时片段是空的。
			"fields": map[string]any{
				"title":              map[string]any{"number_of_fragments": 0},
				"title.loose":        map[string]any{"number_of_fragments": 0},
				"project_name":       map[string]any{"number_of_fragments": 0},
				"project_name.loose": map[string]any{"number_of_fragments": 0},
				// 标签与分类也要能高亮，否则按标签搜出来的结果片段是空的。
				"tags":          map[string]any{"number_of_fragments": 0},
				"category":      map[string]any{"number_of_fragments": 0},
				"summary":       map[string]any{"fragment_size": 120, "number_of_fragments": 1},
				"summary.loose": map[string]any{"fragment_size": 120, "number_of_fragments": 1},
				"body":          map[string]any{"fragment_size": 120, "number_of_fragments": 2},
				"body.loose":    map[string]any{"fragment_size": 120, "number_of_fragments": 2},
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
		// 精确字段与 .loose 子字段可能同时命中同一处文本，去重后再回传，
		// 否则同一句话会在结果卡片里出现两遍。
		hits = append(hits, searchHit{
			ProjectSlug:    raw.Source.ProjectSlug,
			ProjectName:    raw.Source.ProjectName,
			DocumentSlug:   raw.Source.DocumentSlug,
			Title:          raw.Source.Title,
			ProjectSummary: raw.Source.Summary,
			Category:       raw.Source.Category,
			Highlight: dedupeFragments(
				raw.Highlight.Title, raw.Highlight.TitleLoose,
				raw.Highlight.ProjectName, raw.Highlight.ProjectNameLoose,
				raw.Highlight.Tags, raw.Highlight.Category,
				raw.Highlight.Summary, raw.Highlight.SummaryLoose,
				raw.Highlight.Body, raw.Highlight.BodyLoose,
			),
			Score:     raw.Score,
			UpdatedAt: raw.Source.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return hits, nil
}

// dedupeFragments 按出现顺序合并高亮片段并去重。
// 精确字段和它的 .loose 子字段常常返回一模一样的片段。
func dedupeFragments(groups ...[]string) []string {
	seen := make(map[string]struct{})
	merged := make([]string, 0)
	for _, group := range groups {
		for _, fragment := range group {
			if fragment == "" {
				continue
			}
			if _, exists := seen[fragment]; exists {
				continue
			}
			seen[fragment] = struct{}{}
			merged = append(merged, fragment)
		}
	}
	return merged
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
