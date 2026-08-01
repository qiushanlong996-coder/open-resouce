package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// 协作编辑必须按文档隔离。此前协作房间与快照都只按项目分组，
// 同一项目下多篇文档同时编辑会共用一个 Yjs 文档而互相串内容。

// newCollaborationTestServer 启动测试服务器并返回其地址。
func newCollaborationTestServer(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(newHandler())
	t.Cleanup(server.Close)
	return server.URL
}

// dialCollaboration 连接指定项目（可选文档）的协作通道并读取 init 消息。
func dialCollaboration(
	t *testing.T, serverURL string, cookie *http.Cookie, projectSlug, documentSlug string,
) (*websocket.Conn, collaborationWireMessage) {
	t.Helper()
	address := "ws" + strings.TrimPrefix(serverURL, "http") +
		"/api/v1/projects/" + projectSlug + "/collaboration/ws"
	if documentSlug != "" {
		address += "?document=" + documentSlug
	}
	headers := http.Header{}
	headers.Set("Cookie", cookie.String())
	connection, _, err := websocket.Dial(context.Background(), address,
		&websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial %s: %v", address, err)
	}
	t.Cleanup(func() { connection.CloseNow() })

	var initial collaborationWireMessage
	if err := wsjson.Read(context.Background(), connection, &initial); err != nil {
		t.Fatalf("read init from %s: %v", address, err)
	}
	if initial.Type != "init" {
		t.Fatalf("init message from %s = %#v", address, initial)
	}
	return connection, initial
}

// seedCollaborationDocuments 在测试项目下建两篇文档。
func seedCollaborationDocuments(t *testing.T, projectID, authorID string) (projectDocument, projectDocument) {
	t.Helper()
	originalDocuments := projectDocumentRepositoryStore
	projectDocumentRepositoryStore = newMemoryProjectDocumentRepository()
	t.Cleanup(func() { projectDocumentRepositoryStore = originalDocuments })

	ctx := context.Background()
	guide, err := projectDocumentRepositoryStore.Create(ctx, projectID, authorID, projectDocumentInput{
		Slug: "guide", Title: "使用指南", Markdown: "# 使用指南\n\n指南初始内容。",
	})
	if err != nil {
		t.Fatal(err)
	}
	api, err := projectDocumentRepositoryStore.Create(ctx, projectID, authorID, projectDocumentInput{
		Slug: "api", Title: "接口文档", Markdown: "# 接口文档\n\n接口初始内容。",
	})
	if err != nil {
		t.Fatal(err)
	}
	return guide, api
}

// TestCollaborationIsolatesDocuments 验证两篇文档的协作互不串内容。
func TestCollaborationIsolatesDocuments(t *testing.T) {
	ownerCookie, owner, _, _, project := setupCollaborationTest(t)
	guide, api := seedCollaborationDocuments(t, project.ID, owner.ID)
	server := newCollaborationTestServer(t)

	guideConnection, guideInit := dialCollaboration(t, server, ownerCookie, project.Slug, "guide")
	apiConnection, apiInit := dialCollaboration(t, server, ownerCookie, project.Slug, "api")

	// 每篇文档的初始正文必须是自己的内容，而不是项目正文。
	if guideInit.Markdown != guide.Markdown {
		t.Fatalf("guide init markdown = %q, want %q", guideInit.Markdown, guide.Markdown)
	}
	if apiInit.Markdown != api.Markdown {
		t.Fatalf("api init markdown = %q, want %q", apiInit.Markdown, api.Markdown)
	}

	// 在使用指南里保存内容。
	guideMarkdown := "# 使用指南\n\n只属于使用指南的新内容。"
	if err := wsjson.Write(context.Background(), guideConnection, collaborationWireMessage{
		Type: "snapshot", Snapshot: base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
		Markdown: guideMarkdown,
	}); err != nil {
		t.Fatal(err)
	}
	var guideSaved collaborationWireMessage
	if err := wsjson.Read(context.Background(), guideConnection, &guideSaved); err != nil {
		t.Fatal(err)
	}
	if guideSaved.Type != "saved" {
		t.Fatalf("guide saved = %#v", guideSaved)
	}

	ctx := context.Background()
	// 只有使用指南被改写，接口文档与项目正文都不受影响。
	storedGuide, found, err := projectDocumentRepositoryStore.FindBySlug(ctx, project.ID, "guide")
	if err != nil || !found || storedGuide.Markdown != guideMarkdown {
		t.Fatalf("guide document = %#v found=%v err=%v", storedGuide, found, err)
	}
	storedAPI, found, err := projectDocumentRepositoryStore.FindBySlug(ctx, project.ID, "api")
	if err != nil || !found {
		t.Fatalf("api lookup failed found=%v err=%v", found, err)
	}
	if storedAPI.Markdown != api.Markdown {
		t.Fatalf("api document was overwritten: %q", storedAPI.Markdown)
	}
	published, found, err := managedProjectRepositoryStore.FindPublishedBySlug(ctx, project.Slug)
	if err != nil || !found {
		t.Fatalf("project lookup failed found=%v err=%v", found, err)
	}
	if published.Description != project.Description {
		t.Fatalf("project description was overwritten by document save: %q", published.Description)
	}

	// 快照按文档分别存放，互不覆盖。
	guideSnapshot, found, err := collaborationRepositoryStore.LoadSnapshot(ctx, project.ID, guide.ID)
	if err != nil || !found || guideSnapshot.Revision != 1 {
		t.Fatalf("guide snapshot = %#v found=%v err=%v", guideSnapshot, found, err)
	}
	if _, found, _ := collaborationRepositoryStore.LoadSnapshot(ctx, project.ID, api.ID); found {
		t.Fatal("api snapshot should not exist after saving only the guide")
	}
	if _, found, _ := collaborationRepositoryStore.LoadSnapshot(ctx, project.ID, ""); found {
		t.Fatal("project snapshot should not exist after saving only the guide")
	}

	// 接口文档的连接不应收到使用指南的广播。
	readContext, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	var leaked collaborationWireMessage
	if err := wsjson.Read(readContext, apiConnection, &leaked); err == nil {
		t.Fatalf("api room received guide broadcast: %#v", leaked)
	}
}

// TestCollaborationProjectBodyStaysSeparateFromDocuments 验证项目正文与文档互不干扰。
func TestCollaborationProjectBodyStaysSeparateFromDocuments(t *testing.T) {
	ownerCookie, owner, _, _, project := setupCollaborationTest(t)
	guide, _ := seedCollaborationDocuments(t, project.ID, owner.ID)
	server := newCollaborationTestServer(t)

	// 不带 document 参数时协作项目正文，保证旧客户端仍可用。
	bodyConnection, bodyInit := dialCollaboration(t, server, ownerCookie, project.Slug, "")
	if bodyInit.Markdown != project.Description {
		t.Fatalf("project body init = %q, want %q", bodyInit.Markdown, project.Description)
	}

	bodyMarkdown := "# 项目正文\n\n这是通过协作会话改写的项目正文内容。"
	if err := wsjson.Write(context.Background(), bodyConnection, collaborationWireMessage{
		Type: "snapshot", Snapshot: base64.StdEncoding.EncodeToString([]byte{9, 9}),
		Markdown: bodyMarkdown,
	}); err != nil {
		t.Fatal(err)
	}
	var saved collaborationWireMessage
	if err := wsjson.Read(context.Background(), bodyConnection, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Type != "saved" {
		t.Fatalf("project body saved = %#v", saved)
	}

	ctx := context.Background()
	published, _, _ := managedProjectRepositoryStore.FindPublishedBySlug(ctx, project.Slug)
	if published.Description != bodyMarkdown {
		t.Fatalf("project description = %q, want %q", published.Description, bodyMarkdown)
	}
	// 文档正文不能被项目正文的保存动作污染。
	storedGuide, _, _ := projectDocumentRepositoryStore.FindBySlug(ctx, project.ID, "guide")
	if storedGuide.Markdown != guide.Markdown {
		t.Fatalf("guide was overwritten by project body save: %q", storedGuide.Markdown)
	}
}

// TestCollaborationRejectsUnknownDocument 未知文档不应建立协作会话。
func TestCollaborationRejectsUnknownDocument(t *testing.T) {
	ownerCookie, owner, _, _, project := setupCollaborationTest(t)
	seedCollaborationDocuments(t, project.ID, owner.ID)
	server := newCollaborationTestServer(t)

	headers := http.Header{}
	headers.Set("Cookie", ownerCookie.String())
	connection, response, err := websocket.Dial(context.Background(),
		"ws"+strings.TrimPrefix(server, "http")+
			"/api/v1/projects/"+project.Slug+"/collaboration/ws?document=does-not-exist",
		&websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		connection.CloseNow()
		t.Fatal("dial with unknown document should fail")
	}
	if response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown document response = %#v", response)
	}
}

func TestCollaborationRoomKeySeparatesDocuments(t *testing.T) {
	if collaborationRoomKey("project-1", "") == collaborationRoomKey("project-1", "document-1") {
		t.Fatal("project body and document must not share a room")
	}
	if collaborationRoomKey("project-1", "document-1") == collaborationRoomKey("project-1", "document-2") {
		t.Fatal("two documents must not share a room")
	}
	// 防止拼接歧义：不同项目与文档的组合不能产生相同房间键。
	if collaborationRoomKey("a", "b-c") == collaborationRoomKey("a-b", "c") {
		t.Fatal("room key concatenation is ambiguous")
	}
}
