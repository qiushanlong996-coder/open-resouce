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

// 作者端文档树管理接口。
//
// 路由形态：
//   GET    /api/v1/author/projects/{projectID}/documents
//   POST   /api/v1/author/projects/{projectID}/documents
//   PUT    /api/v1/author/projects/{projectID}/documents/{documentID}
//   DELETE /api/v1/author/projects/{projectID}/documents/{documentID}
//   POST   /api/v1/author/projects/{projectID}/documents/{documentID}/move
//   GET    /api/v1/author/projects/{projectID}/documents/{documentID}/revisions
//   GET    /api/v1/author/projects/{projectID}/documents/{documentID}/revisions/{version}
//   POST   /api/v1/author/projects/{projectID}/documents/{documentID}/revisions/{version}/restore
//
// 权限：项目所有者，或拥有 editor 权限的协作者。

type projectDocumentResponse struct {
	Data      projectDocument `json:"data"`
	RequestID string          `json:"request_id"`
}

type projectDocumentListResponse struct {
	Data      []projectDocument `json:"data"`
	Tree      []documentNode    `json:"tree"`
	RequestID string            `json:"request_id"`
}

// authorDocumentContext 解析路径并完成权限校验，返回项目与目标文档 ID。
type authorDocumentContext struct {
	project    managedProject
	documentID string
	action     string
	// hasRevisionSegment 表示 /revisions/ 后面还带了段路径。
	// 用它区分「列表请求」和「版本号写错的请求」：后者必须 404，
	// 不能因为解析不出版本号就悄悄退化成返回整个列表。
	hasRevisionSegment bool
	// revisionVersion 是 /revisions/{version} 里的版本号，非正整数时为 0。
	revisionVersion int
	// revisionAction 是版本号之后的子动作，目前只有 restore。
	revisionAction string
}

func authorProjectDocumentsHandler(writer http.ResponseWriter, request *http.Request) {
	scope, ok := resolveAuthorDocumentScope(writer, request)
	if !ok {
		return
	}

	switch {
	case request.Method == http.MethodGet && scope.documentID == "":
		listAuthorDocuments(writer, request, scope.project)
	case request.Method == http.MethodPost && scope.documentID == "":
		createAuthorDocument(writer, request, scope.project)
	case request.Method == http.MethodPut && scope.documentID != "" && scope.action == "":
		updateAuthorDocument(writer, request, scope)
	case request.Method == http.MethodDelete && scope.documentID != "" && scope.action == "":
		deleteAuthorDocument(writer, request, scope)
	case request.Method == http.MethodPost && scope.action == "move":
		moveAuthorDocument(writer, request, scope)
	case request.Method == http.MethodGet && scope.action == "revisions" && !scope.hasRevisionSegment:
		listAuthorDocumentRevisions(writer, request, scope)
	case scope.action == "revisions" && scope.revisionVersion == 0:
		// /revisions/abc 之类的非法版本号：明确报 404，而不是当成列表请求。
		writeAPIError(writer, request, http.StatusNotFound, "revision_not_found", "该历史版本不存在")
	case request.Method == http.MethodGet && scope.action == "revisions" &&
		scope.revisionVersion > 0 && scope.revisionAction == "":
		showAuthorDocumentRevision(writer, request, scope)
	case request.Method == http.MethodPost && scope.action == "revisions" &&
		scope.revisionVersion > 0 && scope.revisionAction == "restore":
		restoreAuthorDocumentRevision(writer, request, scope)
	default:
		writer.Header().Set("Allow", "GET, POST, PUT, DELETE")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

// resolveAuthorDocumentScope 校验登录态、项目归属与编辑权限。
func resolveAuthorDocumentScope(
	writer http.ResponseWriter, request *http.Request,
) (authorDocumentContext, bool) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return authorDocumentContext{}, false
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/author/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[1] != "documents" {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return authorDocumentContext{}, false
	}
	scope := authorDocumentContext{}
	if len(parts) >= 3 {
		scope.documentID = parts[2]
	}
	if len(parts) >= 4 {
		scope.action = parts[3]
	}
	// /revisions/{version}[/restore]：版本号必须是正整数。
	// 非法值让 revisionVersion 保持 0，由上层路由判定为 404。
	if len(parts) >= 5 {
		scope.hasRevisionSegment = true
		if version, err := strconv.Atoi(parts[4]); err == nil && version > 0 {
			scope.revisionVersion = version
		}
	}
	if len(parts) >= 6 {
		scope.revisionAction = parts[5]
	}

	project, found, err := managedProjectRepositoryStore.FindByID(request.Context(), parts[0])
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return authorDocumentContext{}, false
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return authorDocumentContext{}, false
	}
	// 所有者直接放行；其他人必须持有 editor 协作权限。
	access, err := resolveCollaborationAccess(request.Context(), project, user, true)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "协作服务暂时不可用")
		return authorDocumentContext{}, false
	}
	if !access.CanEdit {
		auditAuth(request, "document_edit_denied", user.Email, user.ID)
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return authorDocumentContext{}, false
	}
	scope.project = project
	return scope, true
}

func listAuthorDocuments(writer http.ResponseWriter, request *http.Request, project managedProject) {
	documents, err := projectDocumentRepositoryStore.ListByProject(request.Context(), project.ID)
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, projectDocumentListResponse{
		Data: documents, Tree: buildDocumentTree(documents),
		RequestID: requestIDFromContext(request.Context()),
	})
}

func createAuthorDocument(writer http.ResponseWriter, request *http.Request, project managedProject) {
	var input projectDocumentInput
	if decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的文档数据")
		return
	}
	input = normalizeDocumentInput(input)
	if !validDocumentInput(input) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_document",
			"文档字段不符合要求：slug 只能是小写字母、数字和连字符，标题不能为空")
		return
	}
	user, _ := currentUser(request)
	document, err := projectDocumentRepositoryStore.Create(request.Context(), project.ID, user.ID, input)
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	syncDocumentIndex(project, document)
	document = syncDocumentRevision(request.Context(), document, user.ID, revisionSourceCreate, nil)
	writeJSON(writer, http.StatusCreated, projectDocumentResponse{
		Data: document, RequestID: requestIDFromContext(request.Context()),
	})
}

func updateAuthorDocument(writer http.ResponseWriter, request *http.Request, scope authorDocumentContext) {
	var input projectDocumentInput
	if decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的文档数据")
		return
	}
	input = normalizeDocumentInput(input)
	if !validDocumentInput(input) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_document",
			"文档字段不符合要求：slug 只能是小写字母、数字和连字符，标题不能为空")
		return
	}
	document, err := projectDocumentRepositoryStore.Update(
		request.Context(), scope.project.ID, scope.documentID, input)
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	syncDocumentIndex(scope.project, document)
	user, _ := currentUser(request)
	document = syncDocumentRevision(request.Context(), document, user.ID, revisionSourceEdit, nil)
	writeJSON(writer, http.StatusOK, projectDocumentResponse{
		Data: document, RequestID: requestIDFromContext(request.Context()),
	})
}

func moveAuthorDocument(writer http.ResponseWriter, request *http.Request, scope authorDocumentContext) {
	var move documentMove
	if decodeJSONBody(request, &move) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的目录调整数据")
		return
	}
	if move.SortOrder < 0 || move.SortOrder > maxDocumentsPerProject {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_document", "排序值超出范围")
		return
	}
	if move.ParentID != nil {
		parent := strings.TrimSpace(*move.ParentID)
		if parent == "" {
			move.ParentID = nil
		} else {
			move.ParentID = &parent
		}
	}
	document, err := projectDocumentRepositoryStore.Move(
		request.Context(), scope.project.ID, scope.documentID, move)
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, projectDocumentResponse{
		Data: document, RequestID: requestIDFromContext(request.Context()),
	})
}

func deleteAuthorDocument(writer http.ResponseWriter, request *http.Request, scope authorDocumentContext) {
	// 删除前先取出待删文档及其子文档，以便同步清理索引。
	// 删除后就查不到它们了。
	removable := documentsToRemoveFromIndex(request.Context(), scope.project.ID, scope.documentID)
	err := projectDocumentRepositoryStore.Delete(
		request.Context(), scope.project.ID, scope.documentID, time.Now().UTC())
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	for _, documentID := range removable {
		removeFromIndexBestEffort(searchDocumentID(scope.project.ID, documentID))
	}
	writer.WriteHeader(http.StatusNoContent)
}

type documentRevisionListResponse struct {
	Data []documentRevisionSummary `json:"data"`
	// CurrentVersion 是文档正文当前所处的版本号，前端用它标出「当前版本」。
	CurrentVersion int    `json:"current_version"`
	RequestID      string `json:"request_id"`
}

type documentRevisionResponse struct {
	Data      documentRevision `json:"data"`
	RequestID string           `json:"request_id"`
}

// findScopedDocument 在已鉴权的项目下按 ID 取出文档。
func findScopedDocument(
	writer http.ResponseWriter, request *http.Request, scope authorDocumentContext,
) (projectDocument, bool) {
	documents, err := projectDocumentRepositoryStore.ListByProject(request.Context(), scope.project.ID)
	if err != nil {
		writeDocumentError(writer, request, err)
		return projectDocument{}, false
	}
	for _, document := range documents {
		if document.ID == scope.documentID {
			return document, true
		}
	}
	writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
	return projectDocument{}, false
}

// listAuthorDocumentRevisions 返回文章历史版本列表（不含正文）。
func listAuthorDocumentRevisions(
	writer http.ResponseWriter, request *http.Request, scope authorDocumentContext,
) {
	document, ok := findScopedDocument(writer, request, scope)
	if !ok {
		return
	}
	revisions, err := documentRevisionRepositoryStore.List(
		request.Context(), document.ID, documentRevisionListLimit)
	if err != nil {
		slog.ErrorContext(request.Context(), "list document revisions failed",
			"document_id", document.ID, "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "版本历史服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, documentRevisionListResponse{
		Data:           summarizeRevisions(request.Context(), revisions, document.Version),
		CurrentVersion: document.Version,
		RequestID:      requestIDFromContext(request.Context()),
	})
}

// showAuthorDocumentRevision 返回指定历史版本的完整正文，供预览与对比使用。
func showAuthorDocumentRevision(
	writer http.ResponseWriter, request *http.Request, scope authorDocumentContext,
) {
	document, ok := findScopedDocument(writer, request, scope)
	if !ok {
		return
	}
	revision, found, err := documentRevisionRepositoryStore.Find(
		request.Context(), document.ID, scope.revisionVersion)
	if err != nil {
		slog.ErrorContext(request.Context(), "load document revision failed",
			"document_id", document.ID, "version", scope.revisionVersion, "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "版本历史服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "revision_not_found", "该历史版本不存在")
		return
	}
	writeJSON(writer, http.StatusOK, documentRevisionResponse{
		Data: revision, RequestID: requestIDFromContext(request.Context()),
	})
}

// restoreAuthorDocumentRevision 把文章正文回滚到指定历史版本。
//
// 回滚不是删除历史：被还原的内容会作为一个新版本追加在历史顶端，
// 因此回滚之后仍能再回滚回去，不存在误操作丢内容的情况。
func restoreAuthorDocumentRevision(
	writer http.ResponseWriter, request *http.Request, scope authorDocumentContext,
) {
	document, ok := findScopedDocument(writer, request, scope)
	if !ok {
		return
	}
	revision, found, err := documentRevisionRepositoryStore.Find(
		request.Context(), document.ID, scope.revisionVersion)
	if err != nil {
		slog.ErrorContext(request.Context(), "load document revision failed",
			"document_id", document.ID, "version", scope.revisionVersion, "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "版本历史服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "revision_not_found", "该历史版本不存在")
		return
	}
	// 只还原正文。标题、slug 和目录位置留给作者自己决定，
	// 避免回滚正文时把已经分享出去的阅读链接一起改回旧值。
	restored, err := projectDocumentRepositoryStore.UpdateMarkdown(
		request.Context(), scope.project.ID, document.ID, revision.Markdown, time.Now().UTC())
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	syncDocumentIndex(scope.project, restored)
	user, _ := currentUser(request)
	restoredFrom := revision.Version
	restored = syncDocumentRevision(
		request.Context(), restored, user.ID, revisionSourceRestore, &restoredFrom)
	writeJSON(writer, http.StatusOK, projectDocumentResponse{
		Data: restored, RequestID: requestIDFromContext(request.Context()),
	})
}

// documentsToRemoveFromIndex 收集目标文档及其全部后代的 ID。
// 删除是级联的，索引也必须跟着清，否则搜索会命中已删文档。
func documentsToRemoveFromIndex(ctx context.Context, projectID, documentID string) []string {
	stored, err := projectDocumentRepositoryStore.ListByProject(ctx, projectID)
	if err != nil {
		// 取不到就至少清自身，剩下的交给重建索引修复。
		return []string{documentID}
	}
	children := make(map[string][]string)
	for _, document := range stored {
		if document.ParentID != nil {
			children[*document.ParentID] = append(children[*document.ParentID], document.ID)
		}
	}
	removable := []string{documentID}
	for cursor := 0; cursor < len(removable); cursor++ {
		removable = append(removable, children[removable[cursor]]...)
	}
	return removable
}

func writeDocumentError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, errDocumentNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
	case errors.Is(err, errDocumentSlugExists):
		writeAPIError(writer, request, http.StatusConflict, "document_slug_exists", "该项目下文档标识已被使用")
	case errors.Is(err, errDocumentCycle):
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_document_parent",
			"目录层级不合法：不能移动到自身或子文档下，层级也不能超过 5 层")
	default:
		slog.ErrorContext(request.Context(), "project document repository failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档服务暂时不可用")
	}
}
