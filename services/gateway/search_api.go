package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 搜索接口与索引重建。
//
//   GET  /api/v1/search?q=...&limit=...   公开搜索
//   POST /api/v1/admin/search/reindex     管理员重建索引

const (
	minSearchQueryRunes = 1
	maxSearchQueryRunes = 100
)

type searchResponse struct {
	Data      []searchHit `json:"data"`
	Query     string      `json:"query"`
	Total     int         `json:"total"`
	RequestID string      `json:"request_id"`
}

func searchHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	runes := len([]rune(query))
	if runes < minSearchQueryRunes || runes > maxSearchQueryRunes {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_query",
			"搜索关键词长度需在 1 到 100 个字符之间")
		return
	}

	limit := maxSearchResults
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_query", "limit 必须是正整数")
			return
		}
		limit = parsed
	}

	hits, err := searchIndexStore.Search(request.Context(), query, limit)
	switch {
	case errors.Is(err, errSearchUnavailable):
		// 明确告知搜索未启用，而不是笼统的 500。
		writeAPIError(writer, request, http.StatusServiceUnavailable, "search_unavailable",
			"搜索服务暂未启用")
		return
	case err != nil:
		slog.ErrorContext(request.Context(), "search failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusBadGateway, "search_failed", "搜索服务暂时不可用")
		return
	}

	// 记录热门搜索词（进程内聚合，用于热门词展示）。
	hotSearchTermsStore.record(query)

	writeJSON(writer, http.StatusOK, searchResponse{
		Data: hits, Query: query, Total: len(hits),
		RequestID: requestIDFromContext(request.Context()),
	})
}

type hotSearchTerm struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}

type hotSearchResponse struct {
	Data      []hotSearchTerm `json:"data"`
	RequestID string          `json:"request_id"`
}

// searchHotTermsHandler 返回进程内聚合的热门搜索词（公开）。
func searchHotTermsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	writeJSON(writer, http.StatusOK, hotSearchResponse{
		Data:      hotSearchTermsStore.top(10),
		RequestID: requestIDFromContext(request.Context()),
	})
}

type reindexResponse struct {
	Indexed int `json:"indexed"`
	Skipped int `json:"skipped"`
	// Recreated 表示索引是否被删除重建（而不是仅刷新文档）。
	Recreated bool   `json:"recreated"`
	RequestID string `json:"request_id"`
}

// searchReindexHandler 全量重建索引。
// 索引写入是 best-effort 的，长期运行必然产生漂移，这里是修复手段。
//
// 带 ?recreate=1 时先删索引再按当前映射重建：分析器和字段映射的改动
// 对已存在的索引无效，改了分析器就必须走这条路径，否则召回不会变好。
func searchReindexHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	// 必须用 requireAdminUser：管理员身份由 ADMIN_EMAILS 白名单现算，
	// requireCurrentUser 返回的 authUser.IsAdmin 并未填充，直读永远是 false。
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if !searchIndexStore.Available() {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "search_unavailable", "搜索服务暂未启用")
		return
	}

	recreate := request.URL.Query().Get("recreate") == "1"
	if recreate {
		if err := searchIndexStore.Recreate(request.Context()); err != nil {
			slog.ErrorContext(request.Context(), "recreate search index failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			writeAPIError(writer, request, http.StatusBadGateway, "reindex_failed", "重建索引失败")
			return
		}
	}
	indexed, skipped, err := rebuildSearchIndex(request.Context())
	if err != nil {
		slog.ErrorContext(request.Context(), "reindex failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusBadGateway, "reindex_failed", "重建索引失败")
		return
	}
	action := "search_reindexed"
	if recreate {
		action = "search_index_recreated"
	}
	auditAuth(request, action, user.Email, user.ID)
	writeJSON(writer, http.StatusOK, reindexResponse{
		Indexed: indexed, Skipped: skipped, Recreated: recreate,
		RequestID: requestIDFromContext(request.Context()),
	})
}

// rebuildSearchIndex 把所有已发布项目及其文档重新写入索引。
// 返回成功条数与跳过条数（跳过指单条写入失败，不中断整体重建）。
func rebuildSearchIndex(ctx context.Context) (int, int, error) {
	if err := searchIndexStore.Ensure(ctx); err != nil {
		return 0, 0, err
	}
	projects, err := managedProjectRepositoryStore.ListPublished(ctx)
	if err != nil {
		return 0, 0, err
	}

	indexed, skipped := 0, 0
	for _, project := range projects {
		// 先清掉该项目的旧记录，避免已删除的文档残留在索引里。
		if err := searchIndexStore.DeleteByProject(ctx, project.ID); err != nil {
			return indexed, skipped, err
		}
		for _, document := range projectSearchDocuments(ctx, project) {
			if err := searchIndexStore.Index(ctx, document); err != nil {
				slog.WarnContext(ctx, "reindex document failed",
					"document_id", document.ID, "error", err)
				skipped++
				continue
			}
			indexed++
		}
	}
	return indexed, skipped, nil
}

// projectSearchDocuments 列出一个项目应当进入索引的所有记录。
// 项目已建文档时索引各篇文档；否则把项目正文当作单条记录。
func projectSearchDocuments(ctx context.Context, project managedProject) []searchDocument {
	stored, err := projectDocumentRepositoryStore.ListByProject(ctx, project.ID)
	if err != nil {
		slog.WarnContext(ctx, "list documents for reindex failed",
			"project_id", project.ID, "error", err)
		return nil
	}
	if len(stored) == 0 {
		return []searchDocument{projectBodySearchDocument(project)}
	}
	documents := make([]searchDocument, 0, len(stored))
	for _, document := range stored {
		documents = append(documents, projectDocumentSearchRecord(project, document))
	}
	return documents
}

// projectBodySearchDocument 把项目正文转成索引记录。
func projectBodySearchDocument(project managedProject) searchDocument {
	parsed := parseMarkdownDocument(project.Description)
	title := parsed.Title
	if title == "" {
		title = project.Name
	}
	return searchDocument{
		ID:          searchDocumentID(project.ID, ""),
		ProjectID:   project.ID,
		ProjectSlug: project.Slug,
		ProjectName: project.Name,
		// 项目正文对应阅读页的 overview 地址。
		DocumentSlug: publishedDocumentSlug,
		Title:        title,
		Body:         project.Description,
		Summary:      project.Summary,
		Category:     project.Category,
		Tags:         project.Tags,
		UpdatedAt:    project.UpdatedAt.UTC(),
	}
}

// projectDocumentSearchRecord 把一篇文档转成索引记录。
func projectDocumentSearchRecord(project managedProject, document projectDocument) searchDocument {
	return searchDocument{
		ID:           searchDocumentID(project.ID, document.ID),
		ProjectID:    project.ID,
		ProjectSlug:  project.Slug,
		ProjectName:  project.Name,
		DocumentID:   document.ID,
		DocumentSlug: document.Slug,
		Title:        document.Title,
		Body:         document.Markdown,
		// 文档记录也带上所属项目的元信息，
		// 这样按标签或分类搜索能把项目下的文档一并召回。
		Summary:   project.Summary,
		Category:  project.Category,
		Tags:      project.Tags,
		UpdatedAt: document.UpdatedAt.UTC(),
	}
}

// syncDocumentIndex 在文档写入后同步索引。调用方无需处理错误。
// 仅已发布项目进入搜索，草稿不应被公开检索到。
func syncDocumentIndex(project managedProject, document projectDocument) {
	if project.Status != "published" {
		return
	}
	indexDocumentBestEffort(projectDocumentSearchRecord(project, document))
}

// syncProjectSearchIndex 在审核结果落定后调整索引。
// 通过则全量入索引（项目可能已有多篇文档），驳回则清空。
func syncProjectSearchIndex(project managedProject, action string) {
	if !searchIndexStore.Available() {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// 无论通过还是驳回，先清旧记录，避免残留。
		if err := searchIndexStore.DeleteByProject(ctx, project.ID); err != nil {
			slog.Warn("clear project search index failed",
				"project_id", project.ID, "error", err)
			return
		}
		if action != "approve" || project.Status != "published" {
			return
		}
		for _, document := range projectSearchDocuments(ctx, project) {
			if err := searchIndexStore.Index(ctx, document); err != nil {
				slog.Warn("index project document failed",
					"document_id", document.ID, "error", err)
			}
		}
	})
}

// warmSearchIndex 在启动时确保索引存在；索引为空时从 MySQL 全量回填。
// 索引一旦丢失或重建，不会有单条写入事件触发同步，只靠管理员手动重建
// 会让搜索长期空结果。启动时发现空索引自动回填，避免同类问题再次发生。
// 失败只告警，不阻断服务启动。
func warmSearchIndex() {
	if !searchIndexStore.Available() {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := searchIndexStore.Ensure(ctx); err != nil {
			slog.Warn("ensure search index failed", "error", err)
			return
		}
		count, err := searchIndexStore.Count(ctx)
		if err != nil {
			slog.Warn("count search index failed", "error", err)
			return
		}
		if count > 0 {
			slog.Info("search index ready", "documents", count)
			return
		}
		slog.Warn("search index is empty; rebuilding from MySQL")
		indexed, skipped, err := rebuildSearchIndex(ctx)
		if err != nil {
			slog.Error("startup search index rebuild failed", "error", err)
			return
		}
		slog.Info("search index rebuilt on startup", "indexed", indexed, "skipped", skipped)
	})
}
