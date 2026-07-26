package main

import (
	"net/http"
	"sort"
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
	List(projectSlug string) ([]documentNode, bool)
	Get(projectSlug, documentSlug string) (documentDetail, bool, bool)
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

var documents documentRepository = seedDocumentRepository{documents: seedDocuments}

func (repository seedDocumentRepository) List(projectSlug string) ([]documentNode, bool) {
	projectDocuments, found := repository.documents[projectSlug]
	if !found {
		return nil, false
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
	return nodes, true
}

func (repository seedDocumentRepository) Get(projectSlug, documentSlug string) (documentDetail, bool, bool) {
	projectDocuments, projectFound := repository.documents[projectSlug]
	if !projectFound {
		return documentDetail{}, false, false
	}
	document, documentFound := projectDocuments[documentSlug]
	return document, true, documentFound
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
	nodes, found := documents.List(projectSlug)
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
	document, projectFound, documentFound := documents.Get(projectSlug, documentSlug)
	if !projectFound {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if !documentFound {
		writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
		return
	}

	writeJSON(writer, http.StatusOK, documentDetailResponse{
		Data:      document,
		RequestID: requestIDFromContext(request.Context()),
	})
}
