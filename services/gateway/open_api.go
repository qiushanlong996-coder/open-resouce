package main

import (
	"net/http"
	"time"
)

// 开放写接口（Open Write API）。
//
// 面向外部 AI Agent（技能 / MCP）：以 Authorization: Bearer <api-key> 鉴权，
// 全程以密钥所有者（签发该密钥的管理员用户）身份操作，复用作者端的项目与文件能力。
//
// 发布流程：
//  1. POST /api/v1/open/files/presign          获取代码包 / 文档 / 封面的预签名 PUT
//  2. PUT  <presigned url>                      Agent 客户端把解析后的代码归档直传 OSS
//  3. POST /api/v1/open/projects                用返回的 object_key 创建草稿项目
//  4. POST /api/v1/open/projects/{id}/submit    提交进入管理员审核队列
//
// 发布仍需管理员审核放行：提交后状态为 pending_review，本接口绝不自动发布。
// 被封禁的密钥所有者会被 ensureNotBanned 拦截；只有密钥所有者能提交 / 修改自己的项目。

// openCreateProject 以密钥所有者身份创建草稿项目，逻辑与作者端 POST 完全一致。
func openCreateProject(writer http.ResponseWriter, request *http.Request, ownerID string) {
	if !ensureNotBanned(writer, request, ownerID) {
		return
	}
	var input managedProjectInput
	if decodeJSONBody(request, &input) != nil || !validateManagedProjectInput(input) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project", "项目资料不完整或格式不正确")
		return
	}
	input = normalizeManagedProjectInput(input)
	if !validProjectObjectOwnership(input, ownerID) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project_file", "项目文件不属于当前账号")
		return
	}
	project, err := managedProjectRepositoryStore.Create(request.Context(), ownerID, input)
	if err != nil {
		writeManagedProjectError(writer, request, err)
		return
	}
	auditAuth(request, "open_project_draft_created", "", ownerID)
	writeJSON(writer, http.StatusCreated, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
}

// openProjectSubmitHandler 提交草稿进入审核。
//
//	POST /api/v1/open/projects/{projectID}/submit
func openProjectSubmitHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	key, ok := requireAPIKey(writer, request)
	if !ok {
		return
	}
	if !ensureNotBanned(writer, request, key.OwnerID) {
		return
	}
	projectID := request.PathValue("projectID")
	// Submit 内部以 owner_id 过滤，非本人项目返回 errProjectNotFound（404）。
	project, err := managedProjectRepositoryStore.Submit(request.Context(), key.OwnerID, projectID, time.Now().UTC())
	if err != nil {
		writeManagedProjectError(writer, request, err)
		return
	}
	auditAuth(request, "open_project_submitted", "", key.OwnerID)
	writeJSON(writer, http.StatusOK, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
}

// openProjectDocumentsHandler 为项目追加一篇知识库文档。
//
//	POST /api/v1/open/projects/{projectID}/documents
func openProjectDocumentsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	key, ok := requireAPIKey(writer, request)
	if !ok {
		return
	}
	if !ensureNotBanned(writer, request, key.OwnerID) {
		return
	}
	projectID := request.PathValue("projectID")
	project, found, err := managedProjectRepositoryStore.FindByID(request.Context(), projectID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	// 只有密钥所有者能为自己的项目添加文档；其余一律按不存在处理。
	if !found || project.OwnerID != key.OwnerID {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
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
	document, err := projectDocumentRepositoryStore.Create(request.Context(), project.ID, key.OwnerID, input)
	if err != nil {
		writeDocumentError(writer, request, err)
		return
	}
	syncDocumentIndex(project, document)
	auditAuth(request, "open_project_document_created", "", key.OwnerID)
	writeJSON(writer, http.StatusCreated, projectDocumentResponse{
		Data: document, RequestID: requestIDFromContext(request.Context()),
	})
}

// openPresignHandler 以密钥所有者身份签发预签名上传 URL（复用 Cookie 链路的校验与限额）。
//
//	POST /api/v1/open/files/presign
func openPresignHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	key, ok := requireAPIKey(writer, request)
	if !ok {
		return
	}
	if !ensureNotBanned(writer, request, key.OwnerID) {
		return
	}
	var input objectUploadRequest
	if decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_file", "文件类型或大小不符合要求")
		return
	}
	authorization, fault := presignUserUpload(request.Context(), key.OwnerID, input)
	if fault != nil {
		writeAPIError(writer, request, fault.status, fault.code, fault.message)
		return
	}
	auditAuth(request, "open_object_upload_authorized", "", key.OwnerID)
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": authorization, "request_id": requestIDFromContext(request.Context()),
	})
}
