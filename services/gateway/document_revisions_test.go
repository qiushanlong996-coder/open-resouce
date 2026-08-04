package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// setupRevisionTest 复用作者端文档测试环境，并换上干净的版本历史仓库。
func setupRevisionTest(t *testing.T) (*http.Cookie, managedProject) {
	t.Helper()
	cookie, project := setupAuthorDocumentTest(t)
	original := documentRevisionRepositoryStore
	documentRevisionRepositoryStore = newMemoryDocumentRevisionRepository()
	t.Cleanup(func() { documentRevisionRepositoryStore = original })
	return cookie, project
}

func revisionBase(projectID, documentID string) string {
	return "/api/v1/author/projects/" + projectID + "/documents/" + documentID + "/revisions"
}

// callAuthor 发起一次带作者 Cookie 的请求，返回状态码与响应体。
func callAuthor(t *testing.T, cookie *http.Cookie, method, path, body string) (int, []byte) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	return response.Code, response.Body.Bytes()
}

func updateDocumentMarkdown(t *testing.T, cookie *http.Cookie, project managedProject,
	document projectDocument, markdown string) projectDocument {
	t.Helper()
	payload, err := json.Marshal(projectDocumentInput{
		ParentID: document.ParentID, Slug: document.Slug, Title: document.Title, Markdown: markdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body := callAuthor(t, cookie, http.MethodPut,
		"/api/v1/author/projects/"+project.ID+"/documents/"+document.ID, string(payload))
	if status != http.StatusOK {
		t.Fatalf("update status = %d: %s", status, body)
	}
	var response projectDocumentResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

// TestDocumentRevisionCoalescesRapidAutosaves 验证同一作者的连续自动保存合并为同一版本。
func TestDocumentRevisionCoalescesRapidAutosaves(t *testing.T) {
	cookie, project := setupRevisionTest(t)
	document := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n第一稿。"}`)
	if document.Version != 1 {
		t.Fatalf("created document version = %d, want 1", document.Version)
	}

	// 编辑器每 1.2s 自动保存一次：这三次都应该合并进 v2，而不是变成 v2/v3/v4。
	var latest projectDocument
	for _, markdown := range []string{"# 使用指南\n\n第二稿。", "# 使用指南\n\n第三稿。", "# 使用指南\n\n第四稿。"} {
		latest = updateDocumentMarkdown(t, cookie, project, document, markdown)
	}
	if latest.Version != 2 {
		t.Fatalf("version after autosaves = %d, want 2", latest.Version)
	}

	status, body := callAuthor(t, cookie, http.MethodGet, revisionBase(project.ID, document.ID), "")
	if status != http.StatusOK {
		t.Fatalf("list revisions status = %d: %s", status, body)
	}
	var list documentRevisionListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 2 {
		t.Fatalf("revision count = %d, want 2: %s", len(list.Data), body)
	}
	if list.CurrentVersion != 2 {
		t.Fatalf("current version = %d, want 2", list.CurrentVersion)
	}
	// 列表按版本号倒序，最新的合并版本在最前。
	if list.Data[0].Version != 2 || !list.Data[0].Current {
		t.Fatalf("newest revision = %#v", list.Data[0])
	}
	if list.Data[0].Source != revisionSourceEdit || list.Data[1].Source != revisionSourceCreate {
		t.Fatalf("revision sources = %q, %q", list.Data[0].Source, list.Data[1].Source)
	}
	if list.Data[0].AuthorName != "文档作者" {
		t.Fatalf("revision author name = %q, want 文档作者", list.Data[0].AuthorName)
	}
	if list.Data[0].CharCount == 0 {
		t.Fatal("revision char count should be reported")
	}
	// 列表不应回传正文，避免把 50 篇文章一次全拉回前端。
	if strings.Contains(string(body), "第四稿") {
		t.Fatalf("revision list leaked markdown: %s", body)
	}
}

// TestDocumentRevisionSplitsAfterWindow 验证超出合并窗口后会开新版本。
func TestDocumentRevisionSplitsAfterWindow(t *testing.T) {
	original := documentRevisionRepositoryStore
	repository := newMemoryDocumentRevisionRepository()
	documentRevisionRepositoryStore = repository
	t.Cleanup(func() { documentRevisionRepositoryStore = original })

	ctx := context.Background()
	document := projectDocument{ID: "document-1", ProjectID: "project-1", Slug: "guide", Title: "指南"}
	base := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)

	document.Markdown = "第一稿"
	first, err := recordDocumentRevision(ctx, document, "user-1", revisionSourceEdit, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	document.Markdown = "第二稿"
	inWindow, err := recordDocumentRevision(ctx, document, "user-1", revisionSourceEdit, nil,
		base.Add(documentRevisionWindow-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if inWindow.Version != first.Version {
		t.Fatalf("in-window version = %d, want %d", inWindow.Version, first.Version)
	}
	document.Markdown = "第三稿"
	afterWindow, err := recordDocumentRevision(ctx, document, "user-1", revisionSourceEdit, nil,
		base.Add(2*documentRevisionWindow))
	if err != nil {
		t.Fatal(err)
	}
	if afterWindow.Version != first.Version+1 {
		t.Fatalf("after-window version = %d, want %d", afterWindow.Version, first.Version+1)
	}
	// 换人编辑必须立即开新版本，否则历史会把两个人的改动混成一条。
	document.Markdown = "协作者的稿"
	other, err := recordDocumentRevision(ctx, document, "user-2", revisionSourceEdit, nil,
		base.Add(2*documentRevisionWindow).Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if other.Version != afterWindow.Version+1 {
		t.Fatalf("other author version = %d, want %d", other.Version, afterWindow.Version+1)
	}
}

// TestDocumentRevisionPrunesOldest 验证历史条数超过上限后丢弃最旧的版本。
func TestDocumentRevisionPrunesOldest(t *testing.T) {
	repository := newMemoryDocumentRevisionRepository()
	original := documentRevisionRepositoryStore
	documentRevisionRepositoryStore = repository
	t.Cleanup(func() { documentRevisionRepositoryStore = original })

	ctx := context.Background()
	document := projectDocument{ID: "document-1", ProjectID: "project-1", Slug: "guide", Title: "指南"}
	base := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	for index := 0; index < maxDocumentRevisions+5; index++ {
		document.Markdown = "第 " + strconv.Itoa(index) + " 稿"
		// 每次都换作者以避免合并，专门测试修剪逻辑。
		author := "user-" + strconv.Itoa(index)
		if _, err := recordDocumentRevision(ctx, document, author, revisionSourceEdit, nil,
			base.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	revisions, err := repository.List(ctx, document.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != maxDocumentRevisions {
		t.Fatalf("retained revisions = %d, want %d", len(revisions), maxDocumentRevisions)
	}
	if revisions[0].Version != maxDocumentRevisions+5 {
		t.Fatalf("newest retained version = %d, want %d", revisions[0].Version, maxDocumentRevisions+5)
	}
	oldest := revisions[len(revisions)-1].Version
	if oldest != 6 {
		t.Fatalf("oldest retained version = %d, want 6", oldest)
	}
}

// TestDocumentRevisionRestore 验证回滚只改正文、并把还原内容追加为新版本。
func TestDocumentRevisionRestore(t *testing.T) {
	cookie, project := setupRevisionTest(t)
	document := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n第一稿。"}`)

	// 直接改仓库时间戳绕过合并窗口，制造两个真实版本。
	ageOutRevisions(t, document.ID)
	edited := updateDocumentMarkdown(t, cookie, project, document, "# 使用指南\n\n被误删了大半内容。")
	if edited.Version != 2 {
		t.Fatalf("edited version = %d, want 2", edited.Version)
	}

	status, body := callAuthor(t, cookie, http.MethodGet,
		revisionBase(project.ID, document.ID)+"/1", "")
	if status != http.StatusOK {
		t.Fatalf("show revision status = %d: %s", status, body)
	}
	var detail documentRevisionResponse
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.Data.Markdown, "第一稿") {
		t.Fatalf("revision markdown = %q", detail.Data.Markdown)
	}

	status, body = callAuthor(t, cookie, http.MethodPost,
		revisionBase(project.ID, document.ID)+"/1/restore", "")
	if status != http.StatusOK {
		t.Fatalf("restore status = %d: %s", status, body)
	}
	var restored projectDocumentResponse
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restored.Data.Markdown, "第一稿") {
		t.Fatalf("restored markdown = %q", restored.Data.Markdown)
	}
	// 回滚追加新版本而不是删历史，所以误操作之后还能再回滚回去。
	if restored.Data.Version != 3 {
		t.Fatalf("restored version = %d, want 3", restored.Data.Version)
	}
	if restored.Data.Slug != document.Slug || restored.Data.Title != document.Title {
		t.Fatalf("restore changed slug/title: %#v", restored.Data)
	}

	status, body = callAuthor(t, cookie, http.MethodGet, revisionBase(project.ID, document.ID), "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d: %s", status, body)
	}
	var list documentRevisionListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 3 || list.Data[0].Source != revisionSourceRestore {
		t.Fatalf("revisions after restore = %s", body)
	}
	if list.Data[0].RestoredFrom == nil || *list.Data[0].RestoredFrom != 1 {
		t.Fatalf("restored_from = %#v", list.Data[0].RestoredFrom)
	}
}

// ageOutRevisions 把已有版本的时间戳推到合并窗口之外，让下一次保存开新版本。
func ageOutRevisions(t *testing.T, documentID string) {
	t.Helper()
	repository, ok := documentRevisionRepositoryStore.(*memoryDocumentRevisionRepository)
	if !ok {
		t.Fatal("revision repository is not the in-memory implementation")
	}
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.revisions[documentID] {
		repository.revisions[documentID][index].UpdatedAt =
			time.Now().UTC().Add(-2 * documentRevisionWindow)
	}
}

// TestDocumentRevisionMissingVersion 验证不存在的版本号返回 404。
func TestDocumentRevisionMissingVersion(t *testing.T) {
	cookie, project := setupRevisionTest(t)
	document := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n第一稿。"}`)

	status, body := callAuthor(t, cookie, http.MethodGet,
		revisionBase(project.ID, document.ID)+"/99", "")
	if status != http.StatusNotFound {
		t.Fatalf("missing revision status = %d: %s", status, body)
	}
	status, body = callAuthor(t, cookie, http.MethodPost,
		revisionBase(project.ID, document.ID)+"/99/restore", "")
	if status != http.StatusNotFound {
		t.Fatalf("restore missing revision status = %d: %s", status, body)
	}
	// 版本号写错时必须 404，不能因为解析不出数字就退化成返回整个列表。
	for _, path := range []string{"/abc", "/0", "/-1", "/abc/restore"} {
		status, body := callAuthor(t, cookie, http.MethodGet,
			revisionBase(project.ID, document.ID)+path, "")
		if status != http.StatusNotFound {
			t.Fatalf("GET %s status = %d: %s", path, status, body)
		}
	}
}

// TestDocumentRevisionRequiresEditPermission 验证匿名访问历史被拒绝。
func TestDocumentRevisionRequiresEditPermission(t *testing.T) {
	cookie, project := setupRevisionTest(t)
	document := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n第一稿。"}`)

	request := httptest.NewRequest(http.MethodGet, revisionBase(project.ID, document.ID), nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous revision status = %d: %s", response.Code, response.Body)
	}
}

// TestReaderDocumentExposesRevision 验证阅读页能拿到修订号与最后更新人。
func TestReaderDocumentExposesRevision(t *testing.T) {
	cookie, project := setupRevisionTest(t)
	document := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n第一稿。"}`)
	ageOutRevisions(t, document.ID)
	updateDocumentMarkdown(t, cookie, project, document, "# 使用指南\n\n第二稿。")

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents/guide", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("reader document status = %d: %s", response.Code, response.Body)
	}
	var payload documentDetailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Revision != 2 {
		t.Fatalf("reader revision = %d, want 2", payload.Data.Revision)
	}
	if payload.Data.UpdatedByName != "文档作者" {
		t.Fatalf("reader updated_by_name = %q, want 文档作者", payload.Data.UpdatedByName)
	}
}
