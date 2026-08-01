package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	maxCollaborationUpdateBytes   = 2 << 20
	maxCollaborationSnapshotBytes = 8 << 20
)

type projectCollaborator struct {
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	InvitedBy   string    `json:"invited_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type collaborationSnapshot struct {
	ProjectID string
	// DocumentID 为空串时表示项目正文（尚未建文档的项目）。
	DocumentID string
	Data       []byte
	Revision   uint64
	UpdatedBy  string
	UpdatedAt  time.Time
}

// collaborationRoomKey 把项目与文档组合成协作房间键。
// 协作必须按文档隔离，否则同一项目下多篇文档同时编辑会共用一个 Yjs
// 文档而互相串内容。
func collaborationRoomKey(projectID, documentID string) string {
	return projectID + "\x00" + documentID
}

type collaborationRepository interface {
	ListCollaborators(context.Context, string) ([]projectCollaborator, error)
	FindCollaboratorRole(context.Context, string, string) (string, bool, error)
	UpsertCollaborator(context.Context, string, authUser, string, string, time.Time) (projectCollaborator, error)
	DeleteCollaborator(context.Context, string, string) (bool, error)
	LoadSnapshot(context.Context, string, string) (collaborationSnapshot, bool, error)
	SaveSnapshot(context.Context, string, string, string, []byte, time.Time) (collaborationSnapshot, error)
}

type memoryCollaborationRepository struct {
	sync.RWMutex
	collaborators map[string]map[string]projectCollaborator
	snapshots     map[string]collaborationSnapshot
}

func newMemoryCollaborationRepository() *memoryCollaborationRepository {
	return &memoryCollaborationRepository{
		collaborators: make(map[string]map[string]projectCollaborator),
		snapshots:     make(map[string]collaborationSnapshot),
	}
}

func (repository *memoryCollaborationRepository) ListCollaborators(
	_ context.Context, projectID string,
) ([]projectCollaborator, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]projectCollaborator, 0, len(repository.collaborators[projectID]))
	for _, collaborator := range repository.collaborators[projectID] {
		result = append(result, collaborator)
	}
	return result, nil
}

func (repository *memoryCollaborationRepository) FindCollaboratorRole(
	_ context.Context, projectID, userID string,
) (string, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	collaborator, found := repository.collaborators[projectID][userID]
	return collaborator.Role, found, nil
}

func (repository *memoryCollaborationRepository) UpsertCollaborator(
	_ context.Context, projectID string, user authUser, role, invitedBy string, now time.Time,
) (projectCollaborator, error) {
	repository.Lock()
	defer repository.Unlock()
	if repository.collaborators[projectID] == nil {
		repository.collaborators[projectID] = make(map[string]projectCollaborator)
	}
	collaborator, exists := repository.collaborators[projectID][user.ID]
	if !exists {
		collaborator = projectCollaborator{
			ProjectID: projectID, UserID: user.ID, Email: user.Email,
			DisplayName: user.DisplayName, InvitedBy: invitedBy, CreatedAt: now,
		}
	}
	collaborator.Role, collaborator.UpdatedAt = role, now
	repository.collaborators[projectID][user.ID] = collaborator
	return collaborator, nil
}

func (repository *memoryCollaborationRepository) DeleteCollaborator(
	_ context.Context, projectID, userID string,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	if _, found := repository.collaborators[projectID][userID]; !found {
		return false, nil
	}
	delete(repository.collaborators[projectID], userID)
	return true, nil
}

func (repository *memoryCollaborationRepository) LoadSnapshot(
	_ context.Context, projectID, documentID string,
) (collaborationSnapshot, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	snapshot, found := repository.snapshots[collaborationRoomKey(projectID, documentID)]
	snapshot.Data = append([]byte(nil), snapshot.Data...)
	return snapshot, found, nil
}

func (repository *memoryCollaborationRepository) SaveSnapshot(
	_ context.Context, projectID, documentID, userID string, data []byte, now time.Time,
) (collaborationSnapshot, error) {
	repository.Lock()
	defer repository.Unlock()
	key := collaborationRoomKey(projectID, documentID)
	snapshot := repository.snapshots[key]
	snapshot.ProjectID, snapshot.DocumentID = projectID, documentID
	snapshot.UpdatedBy, snapshot.UpdatedAt = userID, now
	snapshot.Data = append(snapshot.Data[:0], data...)
	snapshot.Revision++
	repository.snapshots[key] = snapshot
	return snapshot, nil
}

type mysqlCollaborationRepository struct{ db *sql.DB }

func newMySQLCollaborationRepository(db *sql.DB) *mysqlCollaborationRepository {
	return &mysqlCollaborationRepository{db: db}
}

func (repository *mysqlCollaborationRepository) ListCollaborators(
	ctx context.Context, projectID string,
) ([]projectCollaborator, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT project_collaborators.project_id,
		project_collaborators.user_id, users.email, users.display_name,
		project_collaborators.role, project_collaborators.invited_by,
		project_collaborators.created_at, project_collaborators.updated_at
		FROM project_collaborators
		JOIN users ON users.id=project_collaborators.user_id
		WHERE project_collaborators.project_id=?
		ORDER BY project_collaborators.created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project collaborators: %w", err)
	}
	defer rows.Close()
	result := make([]projectCollaborator, 0)
	for rows.Next() {
		var collaborator projectCollaborator
		if err := rows.Scan(
			&collaborator.ProjectID, &collaborator.UserID, &collaborator.Email,
			&collaborator.DisplayName, &collaborator.Role, &collaborator.InvitedBy,
			&collaborator.CreatedAt, &collaborator.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project collaborator: %w", err)
		}
		result = append(result, collaborator)
	}
	return result, rows.Err()
}

func (repository *mysqlCollaborationRepository) FindCollaboratorRole(
	ctx context.Context, projectID, userID string,
) (string, bool, error) {
	var role string
	err := repository.db.QueryRowContext(ctx,
		`SELECT role FROM project_collaborators WHERE project_id=? AND user_id=?`,
		projectID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find project collaborator: %w", err)
	}
	return role, true, nil
}

func (repository *mysqlCollaborationRepository) UpsertCollaborator(
	ctx context.Context, projectID string, user authUser, role, invitedBy string, now time.Time,
) (projectCollaborator, error) {
	_, err := repository.db.ExecContext(ctx, `INSERT INTO project_collaborators
		(project_id, user_id, role, invited_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE role=VALUES(role), invited_by=VALUES(invited_by), updated_at=VALUES(updated_at)`,
		projectID, user.ID, role, invitedBy, now, now)
	if err != nil {
		return projectCollaborator{}, fmt.Errorf("upsert project collaborator: %w", err)
	}
	var collaborator projectCollaborator
	err = repository.db.QueryRowContext(ctx, `SELECT project_collaborators.project_id,
		project_collaborators.user_id, users.email, users.display_name,
		project_collaborators.role, project_collaborators.invited_by,
		project_collaborators.created_at, project_collaborators.updated_at
		FROM project_collaborators JOIN users ON users.id=project_collaborators.user_id
		WHERE project_collaborators.project_id=? AND project_collaborators.user_id=?`,
		projectID, user.ID).Scan(
		&collaborator.ProjectID, &collaborator.UserID, &collaborator.Email,
		&collaborator.DisplayName, &collaborator.Role, &collaborator.InvitedBy,
		&collaborator.CreatedAt, &collaborator.UpdatedAt,
	)
	return collaborator, err
}

func (repository *mysqlCollaborationRepository) DeleteCollaborator(
	ctx context.Context, projectID, userID string,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx,
		`DELETE FROM project_collaborators WHERE project_id=? AND user_id=?`,
		projectID, userID)
	if err != nil {
		return false, fmt.Errorf("delete project collaborator: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlCollaborationRepository) LoadSnapshot(
	ctx context.Context, projectID, documentID string,
) (collaborationSnapshot, bool, error) {
	var snapshot collaborationSnapshot
	err := repository.db.QueryRowContext(ctx, `SELECT project_id, document_id, yjs_snapshot,
		revision, updated_by, updated_at FROM project_collaboration_snapshots
		WHERE project_id=? AND document_id=?`, projectID, documentID).Scan(
		&snapshot.ProjectID, &snapshot.DocumentID, &snapshot.Data, &snapshot.Revision,
		&snapshot.UpdatedBy, &snapshot.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return collaborationSnapshot{}, false, nil
	}
	if err != nil {
		return collaborationSnapshot{}, false, fmt.Errorf("load collaboration snapshot: %w", err)
	}
	return snapshot, true, nil
}

func (repository *mysqlCollaborationRepository) SaveSnapshot(
	ctx context.Context, projectID, documentID, userID string, data []byte, now time.Time,
) (collaborationSnapshot, error) {
	_, err := repository.db.ExecContext(ctx, `INSERT INTO project_collaboration_snapshots
		(project_id, document_id, yjs_snapshot, revision, updated_by, updated_at)
		VALUES (?, ?, ?, 1, ?, ?)
		ON DUPLICATE KEY UPDATE yjs_snapshot=VALUES(yjs_snapshot),
		revision=revision+1, updated_by=VALUES(updated_by), updated_at=VALUES(updated_at)`,
		projectID, documentID, data, userID, now)
	if err != nil {
		return collaborationSnapshot{}, fmt.Errorf("save collaboration snapshot: %w", err)
	}
	snapshot, found, err := repository.LoadSnapshot(ctx, projectID, documentID)
	if err != nil || !found {
		return collaborationSnapshot{}, fmt.Errorf("reload collaboration snapshot: %w", err)
	}
	return snapshot, nil
}

var collaborationRepositoryStore collaborationRepository = newMemoryCollaborationRepository()

type collaborationAccess struct {
	Role      string `json:"role"`
	CanEdit   bool   `json:"can_edit"`
	CanManage bool   `json:"can_manage"`
}

func resolveCollaborationAccess(
	ctx context.Context, project managedProject, user authUser, authenticated bool,
) (collaborationAccess, error) {
	if authenticated && (project.OwnerID == user.ID || user.IsAdmin) {
		return collaborationAccess{Role: "owner", CanEdit: true, CanManage: true}, nil
	}
	if authenticated {
		role, found, err := collaborationRepositoryStore.FindCollaboratorRole(ctx, project.ID, user.ID)
		if err != nil {
			return collaborationAccess{}, err
		}
		if found {
			return collaborationAccess{Role: role, CanEdit: role == "editor"}, nil
		}
	}
	return collaborationAccess{Role: "viewer"}, nil
}

func collaborationAccessHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(
		request.Context(), request.PathValue("slug"))
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "协作权限暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	user, authenticated := currentUser(request)
	access, err := resolveCollaborationAccess(request.Context(), project, user, authenticated)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "协作权限暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": access, "request_id": requestIDFromContext(request.Context()),
	})
}

func projectCollaboratorsHandler(writer http.ResponseWriter, request *http.Request) {
	user, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(
		request.Context(), request.PathValue("slug"))
	if err != nil || !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	access, err := resolveCollaborationAccess(request.Context(), project, user, true)
	if err != nil || !access.CanManage {
		writeAPIError(writer, request, http.StatusForbidden, "collaboration_forbidden", "只有项目所有者可以管理协作者")
		return
	}

	switch request.Method {
	case http.MethodGet:
		collaborators, err := collaborationRepositoryStore.ListCollaborators(request.Context(), project.ID)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "协作者列表暂时不可用")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": collaborators, "request_id": requestIDFromContext(request.Context()),
		})
	case http.MethodPost:
		var input struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if decodeJSONBody(request, &input) != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "请求格式不正确")
			return
		}
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		input.Role = strings.ToLower(strings.TrimSpace(input.Role))
		if input.Role != "viewer" && input.Role != "editor" {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_role", "权限必须是只读或可编辑")
			return
		}
		target, userFound, err := authRepositoryStore.FindUserByEmail(request.Context(), input.Email)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "用户查询失败")
			return
		}
		if !userFound {
			writeAPIError(writer, request, http.StatusNotFound, "user_not_found", "该邮箱尚未注册")
			return
		}
		if target.ID == project.OwnerID {
			writeAPIError(writer, request, http.StatusConflict, "owner_role_fixed", "项目所有者已拥有完整权限")
			return
		}
		collaborator, err := collaborationRepositoryStore.UpsertCollaborator(
			request.Context(), project.ID, target, input.Role, user.ID, time.Now().UTC())
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "协作者保存失败")
			return
		}
		if input.Role != "editor" {
			activeCollaborationHub.disconnectProjectUser(project.ID, target.ID)
		}
		auditAuth(request, "project_collaborator_updated", user.Email, target.ID)
		writeJSON(writer, http.StatusCreated, map[string]any{
			"data": collaborator, "request_id": requestIDFromContext(request.Context()),
		})
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

func projectCollaboratorHandler(writer http.ResponseWriter, request *http.Request) {
	user, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(
		request.Context(), request.PathValue("slug"))
	if err != nil || !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	access, err := resolveCollaborationAccess(request.Context(), project, user, true)
	if err != nil || !access.CanManage {
		writeAPIError(writer, request, http.StatusForbidden, "collaboration_forbidden", "只有项目所有者可以管理协作者")
		return
	}
	if request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	targetID := strings.TrimSpace(request.PathValue("userID"))
	if targetID == "" || targetID == project.OwnerID {
		writeAPIError(writer, request, http.StatusConflict, "owner_role_fixed", "不能移除项目所有者")
		return
	}
	deleted, err := collaborationRepositoryStore.DeleteCollaborator(request.Context(), project.ID, targetID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "移除协作者失败")
		return
	}
	if !deleted {
		writeAPIError(writer, request, http.StatusNotFound, "collaborator_not_found", "协作者不存在")
		return
	}
	activeCollaborationHub.disconnectProjectUser(project.ID, targetID)
	auditAuth(request, "project_collaborator_removed", user.Email, targetID)
	writer.WriteHeader(http.StatusNoContent)
}

type collaborationWireMessage struct {
	Type        string    `json:"type"`
	Update      string    `json:"update,omitempty"`
	Snapshot    string    `json:"snapshot,omitempty"`
	Markdown    string    `json:"markdown,omitempty"`
	Revision    uint64    `json:"revision,omitempty"`
	SavedAt     time.Time `json:"saved_at,omitempty"`
	ClientID    uint64    `json:"client_id,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	Color       string    `json:"color,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type collaborationClient struct {
	connection *websocket.Conn
	projectID  string
	// documentID 为空串时协作目标是项目正文。
	documentID string
	user       authUser
	clientID   uint64
	writeMu    sync.Mutex
}

// roomKey 返回当前客户端所属的协作房间键。
func (client *collaborationClient) roomKey() string {
	return collaborationRoomKey(client.projectID, client.documentID)
}

func (client *collaborationClient) write(message collaborationWireMessage) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, client.connection, message)
}

type collaborationHub struct {
	sync.RWMutex
	rooms map[string]map[*collaborationClient]struct{}
}

func newCollaborationHub() *collaborationHub {
	return &collaborationHub{rooms: make(map[string]map[*collaborationClient]struct{})}
}

func (hub *collaborationHub) join(client *collaborationClient) {
	hub.Lock()
	defer hub.Unlock()
	key := client.roomKey()
	if hub.rooms[key] == nil {
		hub.rooms[key] = make(map[*collaborationClient]struct{})
	}
	hub.rooms[key][client] = struct{}{}
}

func (hub *collaborationHub) leave(client *collaborationClient) {
	hub.Lock()
	defer hub.Unlock()
	key := client.roomKey()
	delete(hub.rooms[key], client)
	if len(hub.rooms[key]) == 0 {
		delete(hub.rooms, key)
	}
}

func (hub *collaborationHub) broadcast(sender *collaborationClient, message collaborationWireMessage) {
	key := sender.roomKey()
	hub.RLock()
	clients := make([]*collaborationClient, 0, len(hub.rooms[key]))
	for client := range hub.rooms[key] {
		if client != sender {
			clients = append(clients, client)
		}
	}
	hub.RUnlock()
	for _, client := range clients {
		if err := client.write(message); err != nil {
			slog.Warn("collaboration broadcast failed", "project_id", sender.projectID,
				"document_id", sender.documentID, "error", err)
		}
	}
}

// disconnectProjectUser 踢掉某用户在该项目**全部文档房间**中的连接。
// 权限是项目级的，掉权后不能只断开其中一篇文档。
func (hub *collaborationHub) disconnectProjectUser(projectID, userID string) {
	hub.RLock()
	clients := make([]*collaborationClient, 0)
	for _, room := range hub.rooms {
		for client := range room {
			if client.projectID == projectID && client.user.ID == userID {
				clients = append(clients, client)
			}
		}
	}
	hub.RUnlock()
	for _, client := range clients {
		_ = client.connection.Close(websocket.StatusPolicyViolation, "编辑权限已被移除")
	}
}

var activeCollaborationHub = newCollaborationHub()

// collaborationTarget 描述一次协作会话的编辑对象。
type collaborationTarget struct {
	// documentID 为空串表示项目正文。
	documentID string
	markdown   string
}

// resolveCollaborationTarget 把请求中的文档 slug 解析为协作对象。
// slug 为空时继续协作项目正文，保证还没建文档的项目与旧客户端仍可用。
func resolveCollaborationTarget(
	ctx context.Context, project managedProject, documentSlug string,
) (collaborationTarget, error) {
	slug := strings.TrimSpace(documentSlug)
	if slug == "" {
		return collaborationTarget{markdown: project.Description}, nil
	}
	document, found, err := projectDocumentRepositoryStore.FindBySlug(ctx, project.ID, slug)
	if err != nil {
		return collaborationTarget{}, err
	}
	if !found {
		return collaborationTarget{}, errDocumentNotFound
	}
	return collaborationTarget{documentID: document.ID, markdown: document.Markdown}, nil
}

func projectCollaborationWebSocketHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, authenticated := requireCurrentUser(writer, request)
	if !authenticated {
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(
		request.Context(), request.PathValue("slug"))
	if err != nil || !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	access, err := resolveCollaborationAccess(request.Context(), project, user, true)
	if err != nil || !access.CanEdit {
		writeAPIError(writer, request, http.StatusForbidden, "collaboration_forbidden", "当前账号没有编辑权限")
		return
	}

	// 协作目标由 ?document=<slug> 指定；缺省为项目正文。
	target, err := resolveCollaborationTarget(request.Context(), project, request.URL.Query().Get("document"))
	if err != nil {
		if errors.Is(err, errDocumentNotFound) {
			writeAPIError(writer, request, http.StatusNotFound, "document_not_found", "文档不存在")
			return
		}
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "文档服务暂时不可用")
		return
	}

	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		slog.Warn("collaboration websocket upgrade failed", "project_id", project.ID, "error", err)
		return
	}
	connection.SetReadLimit(maxCollaborationSnapshotBytes + (1 << 20))
	client := &collaborationClient{
		connection: connection, projectID: project.ID,
		documentID: target.documentID, user: user,
	}
	activeCollaborationHub.join(client)
	defer func() {
		activeCollaborationHub.leave(client)
		if client.clientID != 0 {
			activeCollaborationHub.broadcast(client, collaborationWireMessage{
				Type: "presence-left", ClientID: client.clientID, UserID: user.ID,
			})
		}
		_ = connection.Close(websocket.StatusNormalClosure, "")
	}()

	snapshot, snapshotFound, err := collaborationRepositoryStore.LoadSnapshot(
		context.Background(), project.ID, target.documentID)
	if err != nil {
		_ = client.write(collaborationWireMessage{Type: "error", Message: "协作文档加载失败"})
		return
	}
	initial := collaborationWireMessage{
		Type: "init", Markdown: target.markdown, UserID: user.ID,
		DisplayName: user.DisplayName,
	}
	if snapshotFound {
		initial.Snapshot = base64.StdEncoding.EncodeToString(snapshot.Data)
		initial.Revision = snapshot.Revision
	}
	if err := client.write(initial); err != nil {
		return
	}
	activeCollaborationHub.broadcast(client, collaborationWireMessage{Type: "presence-request"})

	for {
		var message collaborationWireMessage
		if err := wsjson.Read(context.Background(), connection, &message); err != nil {
			return
		}
		switch message.Type {
		case "update", "awareness":
			decoded, err := base64.StdEncoding.DecodeString(message.Update)
			if err != nil || len(decoded) == 0 || len(decoded) > maxCollaborationUpdateBytes {
				_ = client.write(collaborationWireMessage{Type: "error", Message: "协作更新格式不正确"})
				continue
			}
			if message.Type == "awareness" && message.ClientID != 0 {
				client.clientID = message.ClientID
			}
			message.UserID, message.DisplayName = user.ID, user.DisplayName
			activeCollaborationHub.broadcast(client, message)
		case "presence":
			client.clientID = message.ClientID
			message.UserID, message.DisplayName = user.ID, user.DisplayName
			activeCollaborationHub.broadcast(client, message)
		case "presence-request":
			activeCollaborationHub.broadcast(client, message)
		case "snapshot":
			data, err := base64.StdEncoding.DecodeString(message.Snapshot)
			markdown := strings.TrimSpace(message.Markdown)
			// 项目正文作为发布简介有最小长度要求；子文档允许更短、也允许更长。
			minimum, maximum := 20, 50000
			if target.documentID != "" {
				minimum, maximum = 0, maxDocumentMarkdown
			}
			if err != nil || len(data) == 0 || len(data) > maxCollaborationSnapshotBytes ||
				len(markdown) < minimum || len(markdown) > maximum {
				_ = client.write(collaborationWireMessage{Type: "error", Message: "协作文档内容不正确"})
				continue
			}
			now := time.Now().UTC()
			saved, err := collaborationRepositoryStore.SaveSnapshot(
				context.Background(), project.ID, target.documentID, user.ID, data, now)
			if err == nil {
				// 正文回写到各自的存储位置，避免子文档覆盖项目简介。
				if target.documentID != "" {
					var saved projectDocument
					saved, err = projectDocumentRepositoryStore.UpdateMarkdown(
						context.Background(), project.ID, target.documentID, markdown, now)
					if err == nil {
						// 协作保存是文档正文的主要更新路径，索引要跟上。
						syncDocumentIndex(project, saved)
					}
				} else {
					var saved managedProject
					saved, err = managedProjectRepositoryStore.UpdatePublishedDescription(
						context.Background(), project.ID, markdown, now)
					if err == nil {
						indexDocumentBestEffort(projectBodySearchDocument(saved))
					}
				}
			}
			if err != nil {
				_ = client.write(collaborationWireMessage{Type: "error", Message: "协作文档保存失败"})
				continue
			}
			savedMessage := collaborationWireMessage{
				Type: "saved", Revision: saved.Revision, SavedAt: saved.UpdatedAt,
				UserID: user.ID, DisplayName: user.DisplayName,
			}
			_ = client.write(savedMessage)
			activeCollaborationHub.broadcast(client, savedMessage)
			auditAuth(request, "published_document_collaboratively_saved", user.Email, project.ID)
		default:
			_ = client.write(collaborationWireMessage{Type: "error", Message: "未知的协作消息"})
		}
	}
}
