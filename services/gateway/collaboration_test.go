package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func setupCollaborationTest(t *testing.T) (*http.Cookie, authUser, *http.Cookie, authUser, managedProject) {
	t.Helper()
	originalAuth := authRepositoryStore
	originalProjects := managedProjectRepositoryStore
	originalCollaboration := collaborationRepositoryStore
	originalHub := activeCollaborationHub
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	projectRepository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projectRepository
	collaborationRepositoryStore = newMemoryCollaborationRepository()
	activeCollaborationHub = newCollaborationHub()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		managedProjectRepositoryStore = originalProjects
		collaborationRepositoryStore = originalCollaboration
		activeCollaborationHub = originalHub
		authRateLimiter = originalLimiter
	})

	ownerCookie, owner := registerTestUser(t, "collaboration-owner@example.com", "项目所有者")
	editorCookie, editor := registerTestUser(t, "collaboration-editor@example.com", "协作编辑者")
	now := time.Now().UTC()
	project := managedProject{
		ID: "project-collaboration", OwnerID: owner.ID, Slug: "collaboration-project",
		Name: "协作项目", Summary: "用于测试已发布文档实时协作的项目",
		Description: "# 已发布文档\n\n这是协作编辑前的公开内容。",
		Category:    "Workflow Agent", Tags: []string{"协作"}, TechStack: []string{"Yjs"},
		License: "MIT", CurrentVersion: "1.0.0", Status: "published",
		PublishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	projectRepository.projects[project.ID] = project
	return ownerCookie, owner, editorCookie, editor, project
}

func TestCollaborationPermissions(t *testing.T) {
	ownerCookie, _, editorCookie, editor, _ := setupCollaborationTest(t)

	ownerAccess := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/collaboration-project/collaboration/access", nil)
	ownerAccess.AddCookie(ownerCookie)
	ownerAccessResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(ownerAccessResponse, ownerAccess)
	if ownerAccessResponse.Code != http.StatusOK ||
		!strings.Contains(ownerAccessResponse.Body.String(), `"can_manage":true`) {
		t.Fatalf("owner access = %d: %s", ownerAccessResponse.Code, ownerAccessResponse.Body)
	}

	add := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/collaboration-project/collaborators",
		strings.NewReader(`{"email":"collaboration-editor@example.com","role":"editor"}`))
	add.Header.Set("Content-Type", "application/json")
	add.AddCookie(ownerCookie)
	addResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(addResponse, add)
	if addResponse.Code != http.StatusCreated ||
		!strings.Contains(addResponse.Body.String(), `"role":"editor"`) {
		t.Fatalf("add collaborator = %d: %s", addResponse.Code, addResponse.Body)
	}

	editorAccess := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/collaboration-project/collaboration/access", nil)
	editorAccess.AddCookie(editorCookie)
	editorAccessResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(editorAccessResponse, editorAccess)
	if editorAccessResponse.Code != http.StatusOK ||
		!strings.Contains(editorAccessResponse.Body.String(), `"can_edit":true`) {
		t.Fatalf("editor access = %d: %s", editorAccessResponse.Code, editorAccessResponse.Body)
	}

	remove := httptest.NewRequest(http.MethodDelete,
		"/api/v1/projects/collaboration-project/collaborators/"+editor.ID, nil)
	remove.AddCookie(ownerCookie)
	removeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(removeResponse, remove)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("remove collaborator = %d: %s", removeResponse.Code, removeResponse.Body)
	}

	editorAccessAfterDelete := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/collaboration-project/collaboration/access", nil)
	editorAccessAfterDelete.AddCookie(editorCookie)
	editorAccessAfterDeleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(editorAccessAfterDeleteResponse, editorAccessAfterDelete)
	if editorAccessAfterDeleteResponse.Code != http.StatusOK ||
		!strings.Contains(editorAccessAfterDeleteResponse.Body.String(), `"can_edit":false`) {
		t.Fatalf("removed editor access = %d: %s",
			editorAccessAfterDeleteResponse.Code, editorAccessAfterDeleteResponse.Body)
	}
}

func TestCollaborationWebSocketSavesSnapshotAndPublishedMarkdown(t *testing.T) {
	ownerCookie, _, _, _, project := setupCollaborationTest(t)
	server := httptest.NewServer(newHandler())
	defer server.Close()

	headers := http.Header{}
	headers.Set("Cookie", ownerCookie.String())
	connection, _, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(server.URL, "http")+
			"/api/v1/projects/collaboration-project/collaboration/ws",
		&websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()

	var initial collaborationWireMessage
	if err := wsjson.Read(context.Background(), connection, &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Type != "init" || initial.Markdown != project.Description {
		t.Fatalf("initial message = %#v", initial)
	}

	markdown := "# 实时协作文档\n\n这段内容由 WebSocket 协作会话保存并立即公开。"
	save := collaborationWireMessage{
		Type: "snapshot", Snapshot: base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4}),
		Markdown: markdown,
	}
	if err := wsjson.Write(context.Background(), connection, save); err != nil {
		t.Fatal(err)
	}
	var saved collaborationWireMessage
	if err := wsjson.Read(context.Background(), connection, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Type != "saved" || saved.Revision != 1 {
		t.Fatalf("saved message = %#v", saved)
	}

	snapshot, found, err := collaborationRepositoryStore.LoadSnapshot(context.Background(), project.ID, "")
	if err != nil || !found || snapshot.Revision != 1 {
		t.Fatalf("snapshot = %#v found=%v err=%v", snapshot, found, err)
	}
	published, found, err := managedProjectRepositoryStore.FindPublishedBySlug(
		context.Background(), project.Slug)
	if err != nil || !found || published.Description != markdown {
		data, _ := json.Marshal(published)
		t.Fatalf("published = %s found=%v err=%v", data, found, err)
	}
}
