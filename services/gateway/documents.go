package main

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"time"
)

type documentNode struct {
	ID       string         `json:"id"`
	Slug     string         `json:"slug"`
	Title    string         `json:"title"`
	Order    int            `json:"order"`
	Children []documentNode `json:"children"`
}

type documentOutlineItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Level int    `json:"level"`
}

type documentBlock struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

type documentDetail struct {
	ID        string                `json:"id"`
	ProjectID string                `json:"project_id"`
	Slug      string                `json:"slug"`
	Title     string                `json:"title"`
	Version   string                `json:"version"`
	UpdatedAt string                `json:"updated_at"`
	Markdown  string                `json:"markdown"`
	Outline   []documentOutlineItem `json:"outline"`
	Blocks    []documentBlock       `json:"blocks"`
}

type documentListResponse struct {
	Data      []documentNode `json:"data"`
	RequestID string         `json:"request_id"`
}

type documentDetailResponse struct {
	Data      documentDetail `json:"data"`
	RequestID string         `json:"request_id"`
}

type documentRepository interface {
	List(ctx context.Context, projectSlug string) ([]documentNode, bool, error)
	Get(ctx context.Context, projectSlug, documentSlug string) (documentDetail, bool, bool, error)
}

// publishedDocumentSlug 是已发布项目正文的固定文档标识。
// 当前一个项目只有一篇正文（存于 managed_projects.description），
// 后续扩展为多篇文档时再改为真实的文档表。
const publishedDocumentSlug = "overview"

// managedDocumentRepository 将已发布项目的真实 Markdown 作为文档对外提供，
// 未命中时回退到种子演示项目，保证演示内容仍可浏览。
type managedDocumentRepository struct {
	fallback documentRepository
}

func (repository managedDocumentRepository) List(
	ctx context.Context, projectSlug string,
) ([]documentNode, bool, error) {
	project, found, err := repository.publishedProject(ctx, projectSlug)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return repository.fallback.List(ctx, projectSlug)
	}
	// 项目已建文档时返回多层目录；尚未建文档的旧项目继续把项目正文当单篇文档。
	stored, err := projectDocumentRepositoryStore.ListByProject(ctx, project.ID)
	if err != nil {
		return nil, false, err
	}
	if len(stored) > 0 {
		return buildDocumentTree(stored), true, nil
	}
	document := managedProjectDocument(project)
	return []documentNode{{
		ID: document.ID, Slug: document.Slug, Title: document.Title,
		Order: 1, Children: []documentNode{},
	}}, true, nil
}

func (repository managedDocumentRepository) Get(
	ctx context.Context, projectSlug, documentSlug string,
) (documentDetail, bool, bool, error) {
	project, found, err := repository.publishedProject(ctx, projectSlug)
	if err != nil {
		return documentDetail{}, false, false, err
	}
	if !found {
		return repository.fallback.Get(ctx, projectSlug, documentSlug)
	}
	stored, err := projectDocumentRepositoryStore.ListByProject(ctx, project.ID)
	if err != nil {
		return documentDetail{}, false, false, err
	}
	if len(stored) > 0 {
		target := documentSlug
		if target == "" {
			// 未指定时打开目录中的首篇文档。
			if tree := buildDocumentTree(stored); len(tree) > 0 {
				target = tree[0].Slug
			}
		}
		for _, document := range stored {
			if document.Slug == target {
				return documentDetailFrom(document, project.CurrentVersion), true, true, nil
			}
		}
		return documentDetail{}, true, false, nil
	}
	document := managedProjectDocument(project)
	if documentSlug == "" || documentSlug == document.Slug {
		return document, true, true, nil
	}
	return documentDetail{}, true, false, nil
}

func (repository managedDocumentRepository) publishedProject(
	ctx context.Context, projectSlug string,
) (managedProject, bool, error) {
	if managedProjectRepositoryStore == nil || projectSlug == "" {
		return managedProject{}, false, nil
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(ctx, projectSlug)
	if err != nil || !found {
		return managedProject{}, false, err
	}
	return project, true, nil
}

// managedProjectDocument 把项目正文转成阅读页需要的文档结构。
func managedProjectDocument(project managedProject) documentDetail {
	parsed := parseMarkdownDocument(project.Description)
	title := parsed.Title
	if title == "" {
		title = project.Name
	}
	version := project.CurrentVersion
	if version == "" {
		version = "—"
	}
	return documentDetail{
		ID:        "doc-" + project.ID,
		ProjectID: project.ID,
		Slug:      publishedDocumentSlug,
		Title:     title,
		Version:   version,
		UpdatedAt: project.UpdatedAt.UTC().Format(time.RFC3339),
		Markdown:  project.Description,
		Outline:   parsed.Outline,
		Blocks:    parsed.Blocks,
	}
}

type seedDocumentRepository struct {
	documents map[string]map[string]documentDetail
}

var seedDocuments = map[string]map[string]documentDetail{
	"atlas-agent": {
		"quick-start": {
			ID:        "doc-atlas-quick-start",
			ProjectID: "atlas",
			Slug:      "quick-start",
			Title:     "快速开始",
			Version:   "0.8.0",
			UpdatedAt: "2026-07-26T12:00:00Z",
			Markdown:  "# 快速开始\n\nAtlas Agent 是一个面向复杂任务的多 Agent 协作运行时。\n\n## 从一个清晰的任务开始\n\n规划器会把目标拆成检索、工具调用和结果检查步骤。\n\n```python\nfrom atlas import Runtime\nruntime = Runtime(model=\"gpt-4.1\", trace=True)\nresult = runtime.run(\"分析用户反馈并给出行动计划\")\n```\n\n## 节点之间如何协作\n\n每个节点都需要声明输入、输出和失败策略。\n\n## 安装依赖\n\n```bash\npip install atlas-agent\n```\n",
			Outline: []documentOutlineItem{
				{ID: "quick-start", Title: "快速开始", Level: 1},
				{ID: "clear-task", Title: "从一个清晰的任务开始", Level: 2},
				{ID: "node-collaboration", Title: "节点之间如何协作", Level: 2},
				{ID: "installation", Title: "安装依赖", Level: 2},
			},
			Blocks: []documentBlock{
				{ID: "block-atlas-intro", Type: "paragraph", Text: "Atlas Agent 是一个面向复杂任务的多 Agent 协作运行时。"},
				{ID: "block-atlas-task", Type: "paragraph", Text: "规划器会把目标拆成检索、工具调用和结果检查步骤。"},
				{ID: "block-atlas-collaboration", Type: "paragraph", Text: "每个节点都需要声明输入、输出和失败策略。"},
				{ID: "block-atlas-install", Type: "code", Text: "pip install atlas-agent"},
			},
		},
	},
	"paperclip-rag": {
		"quick-start": basicQuickStartDocument("paperclip", "1.4.2", "从文档清洗、切分和索引开始搭建带引用的知识库问答。"),
	},
	"forge-runner": {
		"quick-start": basicQuickStartDocument("forge", "0.6.1", "创建隔离沙箱并运行第一个可回放的编码任务。"),
	},
	"relay-mcp": {
		"quick-start": basicQuickStartDocument("relay", "0.5.0", "注册 MCP 服务并为 Agent 配置最小工具权限。"),
	},
}

var documents documentRepository = managedDocumentRepository{
	fallback: seedDocumentRepository{documents: seedDocuments},
}

func (repository seedDocumentRepository) List(
	_ context.Context, projectSlug string,
) ([]documentNode, bool, error) {
	projectDocuments, found := repository.documents[projectSlug]
	if !found {
		return nil, false, nil
	}
	nodes := make([]documentNode, 0, len(projectDocuments))
	for _, document := range projectDocuments {
		nodes = append(nodes, documentNode{
			ID:       document.ID,
			Slug:     document.Slug,
			Title:    document.Title,
			Order:    1,
			Children: []documentNode{},
		})
	}
	sort.Slice(nodes, func(left, right int) bool {
		if nodes[left].Order == nodes[right].Order {
			return nodes[left].Slug < nodes[right].Slug
		}
		return nodes[left].Order < nodes[right].Order
	})
	return nodes, true, nil
}

func (repository seedDocumentRepository) Get(
	_ context.Context, projectSlug, documentSlug string,
) (documentDetail, bool, bool, error) {
	projectDocuments, projectFound := repository.documents[projectSlug]
	if !projectFound {
		return documentDetail{}, false, false, nil
	}
	document, documentFound := projectDocuments[documentSlug]
	return document, true, documentFound, nil
}

func basicQuickStartDocument(projectID, version, introduction string) documentDetail {
	return documentDetail{
		ID:        "doc-" + projectID + "-quick-start",
		ProjectID: projectID,
		Slug:      "quick-start",
		Title:     "快速开始",
		Version:   version,
		UpdatedAt: "2026-07-26T12:00:00Z",
		Markdown:  "# 快速开始\n\n" + introduction + "\n",
		Outline: []documentOutlineItem{
			{ID: "quick-start", Title: "快速开始", Level: 1},
		},
		Blocks: []documentBlock{
			{ID: "block-" + projectID + "-intro", Type: "paragraph", Text: introduction},
		},
	}
}

func documentListHandler(writer http.ResponseWriter, request *http.Request) {
	projectSlug := request.PathValue("slug")
	nodes, found, err := documents.List(request.Context(), projectSlug)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}

	writeJSON(writer, http.StatusOK, documentListResponse{
		Data:      nodes,
		RequestID: requestIDFromContext(request.Context()),
	})
}

func documentDetailHandler(writer http.ResponseWriter, request *http.Request) {
	projectSlug := request.PathValue("slug")
	documentSlug := request.PathValue("documentSlug")
	document, projectFound, documentFound, err := documents.Get(request.Context(), projectSlug, documentSlug)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档服务暂时不可用")
		return
	}
	if !projectFound {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if !documentFound {
		// 当前 slug 找不到时再查历史标识：作者改过 slug 后，
		// 已分享出去的旧链接应重定向到新地址而不是直接 404。
		if redirected := redirectRenamedDocument(writer, request, projectSlug, documentSlug); redirected {
			return
		}
		writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
		return
	}

	writeJSON(writer, http.StatusOK, documentDetailResponse{
		Data:      document,
		RequestID: requestIDFromContext(request.Context()),
	})
}

// redirectRenamedDocument 在命中历史 slug 时回 301，并告知当前地址。
// 返回是否已处理本次请求。
func redirectRenamedDocument(
	writer http.ResponseWriter, request *http.Request, projectSlug, documentSlug string,
) bool {
	if managedProjectRepositoryStore == nil || projectDocumentRepositoryStore == nil {
		return false
	}
	if projectSlug == "" || documentSlug == "" {
		return false
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), projectSlug)
	if err != nil || !found {
		return false
	}
	document, found, err := projectDocumentRepositoryStore.FindByAliasSlug(
		request.Context(), project.ID, documentSlug)
	if err != nil || !found {
		return false
	}
	target := "/api/v1/projects/" + url.PathEscape(projectSlug) +
		"/documents/" + url.PathEscape(document.Slug)
	writer.Header().Set("Location", target)
	// 带上当前 slug，方便客户端不跟随重定向时也能直接知道新地址。
	writer.Header().Set("X-Document-Slug", document.Slug)
	// 301 而非 302：slug 变更是永久的，希望中间层与搜索引擎更新记录。
	http.Redirect(writer, request, target, http.StatusMovedPermanently)
	return true
}
