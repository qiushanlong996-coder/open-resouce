package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultProjectPageSize = 12
	maxProjectPageSize     = 50
)

type projectSummary struct {
	ID         string         `json:"id"`
	Slug       string         `json:"slug"`
	Name       string         `json:"name"`
	Summary    string         `json:"summary"`
	Category   string         `json:"category"`
	Tags       []string       `json:"tags"`
	Stack      []string       `json:"stack"`
	License    string         `json:"license"`
	Status     string         `json:"status"`
	Maintainer string         `json:"maintainer"`
	UpdatedAt  string         `json:"updated_at"`
	Metrics    projectMetrics `json:"metrics"`
}

type projectMetrics struct {
	Views     int `json:"views"`
	Downloads int `json:"downloads"`
	Stars     int `json:"stars"`
	Comments  int `json:"comments"`
}

type pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type projectListResponse struct {
	Data       []projectSummary `json:"data"`
	Pagination pagination       `json:"pagination"`
	RequestID  string           `json:"request_id"`
}

type projectDetail struct {
	projectSummary
	Description    string           `json:"description"`
	Highlights     []string         `json:"highlights"`
	UseCases       []string         `json:"use_cases"`
	Repository     string           `json:"repository"`
	CurrentVersion string           `json:"current_version"`
	Resources      projectResources `json:"resources"`
}

type projectResources struct {
	Cover    bool `json:"cover"`
	Document bool `json:"document"`
	Code     bool `json:"code"`
}

type projectDetailResponse struct {
	Data      projectDetail `json:"data"`
	RequestID string        `json:"request_id"`
}

var seedProjects = []projectSummary{
	{
		ID:         "atlas",
		Slug:       "atlas-agent",
		Name:       "Atlas Agent",
		Summary:    "面向复杂任务的多 Agent 协作运行时",
		Category:   "Multi-Agent",
		Tags:       []string{"多智能体", "工作流", "可观测"},
		Stack:      []string{"Python", "LangGraph", "OpenAI"},
		License:    "Apache-2.0",
		Status:     "活跃维护",
		Maintainer: "北岛实验室",
		UpdatedAt:  "2026-07-26T12:00:00Z",
		Metrics:    projectMetrics{Downloads: 12800, Stars: 2400, Comments: 18},
	},
	{
		ID:         "paperclip",
		Slug:       "paperclip-rag",
		Name:       "Paperclip RAG",
		Summary:    "为团队知识库设计的轻量 RAG Agent",
		Category:   "RAG Agent",
		Tags:       []string{"知识库", "引用回答", "中文"},
		Stack:      []string{"TypeScript", "LlamaIndex", "Elasticsearch"},
		License:    "MIT",
		Status:     "生产可用",
		Maintainer: "Paperclip",
		UpdatedAt:  "2026-07-25T08:30:00Z",
		Metrics:    projectMetrics{Downloads: 8100, Stars: 1800, Comments: 12},
	},
	{
		ID:         "forge",
		Slug:       "forge-runner",
		Name:       "Forge Runner",
		Summary:    "让 Coding Agent 安全执行真实工程任务",
		Category:   "Coding Agent",
		Tags:       []string{"代码 Agent", "沙箱", "工具调用"},
		Stack:      []string{"Go", "React", "Docker"},
		License:    "MIT",
		Status:     "实验项目",
		Maintainer: "Forge Team",
		UpdatedAt:  "2026-07-23T09:15:00Z",
		Metrics:    projectMetrics{Downloads: 5600, Stars: 960, Comments: 9},
	},
	{
		ID:         "relay",
		Slug:       "relay-mcp",
		Name:       "Relay MCP",
		Summary:    "面向工具调用的 MCP 服务编排层",
		Category:   "Agent Framework",
		Tags:       []string{"MCP", "工具调用", "服务治理"},
		Stack:      []string{"Go", "MCP", "Redis"},
		License:    "Apache-2.0",
		Status:     "活跃维护",
		Maintainer: "Relay Open",
		UpdatedAt:  "2026-07-19T10:00:00Z",
		Metrics:    projectMetrics{Downloads: 4200, Stars: 742, Comments: 7},
	},
}

var seedProjectDetails = map[string]projectDetail{
	"atlas-agent": {
		projectSummary: seedProjects[0],
		Description:    "把规划、检索、执行和复盘拆成可观察的 Agent 节点，让复杂任务保持清晰、可控、可复用。",
		Highlights:     []string{"可观察的多 Agent 运行时", "节点级失败策略", "完整任务链路回放"},
		UseCases:       []string{"复杂研究任务", "多工具自动化", "长流程任务编排"},
		Repository:     "https://github.com/atlas-lab/agent",
		CurrentVersion: "0.8.0",
	},
	"paperclip-rag": {
		projectSummary: seedProjects[1],
		Description:    "从文档清洗、切分、检索到带引用回答，提供一条适合内部知识库的可视化链路。",
		Highlights:     []string{"回答引用可追溯", "中文文档优化", "轻量部署"},
		UseCases:       []string{"团队知识库", "内部文档问答", "客户支持助手"},
		Repository:     "https://github.com/paperclip-ai/rag",
		CurrentVersion: "1.4.2",
	},
	"forge-runner": {
		projectSummary: seedProjects[2],
		Description:    "提供沙箱、工具调用、补丁预览和测试回放，让编码 Agent 的每一步都能被开发者检查。",
		Highlights:     []string{"隔离执行沙箱", "补丁预览", "测试过程回放"},
		UseCases:       []string{"代码维护 Agent", "自动化修复", "工程任务评测"},
		Repository:     "https://github.com/forge-runner/core",
		CurrentVersion: "0.6.1",
	},
	"relay-mcp": {
		projectSummary: seedProjects[3],
		Description:    "把分散的工具能力整理成可发现、可授权、可审计的服务目录，降低 Agent 接入成本。",
		Highlights:     []string{"MCP 服务发现", "细粒度工具授权", "调用审计"},
		UseCases:       []string{"企业工具目录", "Agent 工具治理", "多服务编排"},
		Repository:     "https://github.com/relay-open/mcp",
		CurrentVersion: "0.5.0",
	},
}

func projectListHandler(writer http.ResponseWriter, request *http.Request) {
	page, ok := positiveQueryInteger(request, writer, "page", 1, 1, 1_000_000)
	if !ok {
		return
	}
	pageSize, ok := positiveQueryInteger(request, writer, "page_size", defaultProjectPageSize, 1, maxProjectPageSize)
	if !ok {
		return
	}

	query := strings.TrimSpace(request.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_query", "搜索关键词不能超过 100 个字符")
		return
	}
	category := strings.TrimSpace(request.URL.Query().Get("category"))
	sortBy := strings.TrimSpace(request.URL.Query().Get("sort"))
	if sortBy == "" {
		sortBy = "updated"
	}
	if sortBy != "updated" && sortBy != "downloads" && sortBy != "stars" {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_query", "sort 仅支持 updated、downloads 或 stars")
		return
	}

	projects := append([]projectSummary(nil), seedProjects...)
	managed, err := managedProjectRepositoryStore.ListPublished(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	for _, project := range managed {
		projects = append(projects, managedProjectSummary(project))
	}
	// 用真实统计覆盖 views/downloads，使排序与展示反映实际数据。
	overlayProjectMetrics(request.Context(), projects)
	filtered := filterProjects(projects, query, category)
	sortProjects(filtered, sortBy)

	total := len(filtered)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := min(start+pageSize, total)

	writeJSON(writer, http.StatusOK, projectListResponse{
		Data: filtered[start:end],
		Pagination: pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
		RequestID: requestIDFromContext(request.Context()),
	})
}

func projectDetailHandler(writer http.ResponseWriter, request *http.Request) {
	slug := strings.TrimPrefix(request.URL.Path, "/api/v1/projects/")
	if slug == "" || strings.Contains(slug, "/") {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}

	project, found := seedProjectDetails[slug]
	if !found {
		managed, managedFound, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), slug)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
			return
		}
		if !managedFound {
			writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		project = projectDetail{
			projectSummary: managedProjectSummary(managed),
			Description:    managed.Description, Repository: managed.RepositoryURL,
			CurrentVersion: managed.CurrentVersion,
			Resources: projectResources{
				Cover: managed.CoverObjectKey != "", Document: managed.DocumentObjectKey != "",
				Code: managed.CodeObjectKey != "",
			},
		}
	}

	// 覆盖真实 views/downloads。
	if snapshot, err := projectMetricsRepositoryStore.Snapshot(request.Context(), []string{project.ID}); err == nil {
		if entry, ok := snapshot[project.ID]; ok {
			project.Metrics.Views = entry.Views
			project.Metrics.Downloads = entry.Downloads
		}
	}

	writeJSON(writer, http.StatusOK, projectDetailResponse{
		Data:      project,
		RequestID: requestIDFromContext(request.Context()),
	})
}

func managedProjectSummary(project managedProject) projectSummary {
	return projectSummary{
		ID: project.ID, Slug: project.Slug, Name: project.Name, Summary: project.Summary,
		Category: project.Category, Tags: project.Tags, Stack: project.TechStack,
		License: project.License, Status: "已发布", Maintainer: "社区作者",
		UpdatedAt: project.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func positiveQueryInteger(request *http.Request, writer http.ResponseWriter, name string, fallback, minimum, maximum int) (int, bool) {
	rawValue := strings.TrimSpace(request.URL.Query().Get(name))
	if rawValue == "" {
		return fallback, true
	}

	value, err := strconv.Atoi(rawValue)
	if err != nil || value < minimum || value > maximum {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_query",
			name+" 必须是 "+strconv.Itoa(minimum)+" 到 "+strconv.Itoa(maximum)+" 之间的整数")
		return 0, false
	}
	return value, true
}

func filterProjects(projects []projectSummary, query, category string) []projectSummary {
	normalizedQuery := strings.ToLower(query)
	filtered := make([]projectSummary, 0, len(projects))

	for _, project := range projects {
		if category != "" && !strings.EqualFold(project.Category, category) {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			project.Name,
			project.Summary,
			project.Category,
			strings.Join(project.Tags, " "),
			strings.Join(project.Stack, " "),
		}, " "))
		if normalizedQuery != "" && !strings.Contains(haystack, normalizedQuery) {
			continue
		}
		filtered = append(filtered, project)
	}
	return filtered
}

func sortProjects(projects []projectSummary, sortBy string) {
	sort.SliceStable(projects, func(left, right int) bool {
		switch sortBy {
		case "downloads":
			return projects[left].Metrics.Downloads > projects[right].Metrics.Downloads
		case "stars":
			return projects[left].Metrics.Stars > projects[right].Metrics.Stars
		default:
			return projects[left].UpdatedAt > projects[right].UpdatedAt
		}
	})
}
