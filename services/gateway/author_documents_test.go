package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// setupAuthorDocumentTest 准备一个已发布项目与登录作者，返回作者 Cookie 和项目。
func setupAuthorDocumentTest(t *testing.T) (*http.Cookie, managedProject) {
	t.Helper()
	originalAuth, originalProjects, originalDocuments, originalLimiter :=
		authRepositoryStore, managedProjectRepositoryStore, projectDocumentRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	projects := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = projects
	projectDocumentRepositoryStore = newMemoryProjectDocumentRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, managedProjectRepositoryStore, projectDocumentRepositoryStore, authRateLimiter =
			originalAuth, originalProjects, originalDocuments, originalLimiter
	})

	cookie, owner := registerTestUser(t, "doc-owner@example.com", "文档作者")
	ctx := context.Background()
	project, err := projects.Create(ctx, owner.ID, managedProjectInput{
		Slug: "doc-tree-demo", Name: "文档树示例", Summary: "用于验证多层文档树的示例项目",
		Description: "# 项目正文\n\n这是项目自身的正文，用作没有文档时的兜底。",
		Category:    "Coding Agent", Tags: []string{"Agent"}, TechStack: []string{"Go"},
		License: "MIT", CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := projects.Submit(ctx, owner.ID, project.ID, now); err != nil {
		t.Fatal(err)
	}
	published, err := projects.Review(ctx, project.ID, "admin", "approve", "", now)
	if err != nil {
		t.Fatal(err)
	}
	return cookie, published
}

func createDocument(t *testing.T, cookie *http.Cookie, projectID, body string) projectDocument {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+projectID+"/documents", strings.NewReader(body))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create document status = %d: %s", response.Code, response.Body)
	}
	var payload projectDocumentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

func TestAuthorDocumentTreeLifecycle(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	base := "/api/v1/author/projects/" + project.ID + "/documents"

	guide := createDocument(t, cookie, project.ID,
		`{"slug":"guide","title":"使用指南","markdown":"# 使用指南\n\n入门说明。"}`)
	api := createDocument(t, cookie, project.ID,
		`{"slug":"api","title":"接口文档","markdown":"# 接口文档\n\n接口说明。"}`)

	// 建立父子关系：接口文档挂到使用指南下。
	child := createDocument(t, cookie, project.ID,
		`{"slug":"guide-install","title":"安装","parent_id":"`+guide.ID+`","markdown":"# 安装\n\n安装步骤。"}`)
	if child.ParentID == nil || *child.ParentID != guide.ID {
		t.Fatalf("child parent = %#v", child.ParentID)
	}

	listRequest := httptest.NewRequest(http.MethodGet, base, nil)
	listRequest.AddCookie(cookie)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listResponse.Code, listResponse.Body)
	}
	var list projectDocumentListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 3 {
		t.Fatalf("document count = %d, want 3", len(list.Data))
	}
	// 树形结构应有两个根节点，其中使用指南带一个子节点。
	if len(list.Tree) != 2 {
		t.Fatalf("tree roots = %d, want 2: %#v", len(list.Tree), list.Tree)
	}
	var guideNode documentNode
	for _, node := range list.Tree {
		if node.Slug == "guide" {
			guideNode = node
		}
	}
	if len(guideNode.Children) != 1 || guideNode.Children[0].Slug != "guide-install" {
		t.Fatalf("guide children = %#v", guideNode.Children)
	}

	// 重命名并改写正文。
	update := httptest.NewRequest(http.MethodPut, base+"/"+api.ID,
		strings.NewReader(`{"slug":"api-reference","title":"接口参考","markdown":"# 接口参考\n\n更新后的说明。"}`))
	update.AddCookie(cookie)
	updateResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updateResponse.Code, updateResponse.Body)
	}

	// 移动：接口参考挂到使用指南下并排在第一位。
	move := httptest.NewRequest(http.MethodPost, base+"/"+api.ID+"/move",
		strings.NewReader(`{"parent_id":"`+guide.ID+`","sort_order":0}`))
	move.AddCookie(cookie)
	moveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(moveResponse, move)
	if moveResponse.Code != http.StatusOK {
		t.Fatalf("move status = %d: %s", moveResponse.Code, moveResponse.Body)
	}

	// 阅读侧应看到调整后的多层目录。
	publicTree := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents", nil)
	publicTreeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(publicTreeResponse, publicTree)
	if publicTreeResponse.Code != http.StatusOK {
		t.Fatalf("public tree status = %d: %s", publicTreeResponse.Code, publicTreeResponse.Body)
	}
	var publicList documentListResponse
	if err := json.Unmarshal(publicTreeResponse.Body.Bytes(), &publicList); err != nil {
		t.Fatal(err)
	}
	if len(publicList.Data) != 1 || publicList.Data[0].Slug != "guide" {
		t.Fatalf("public tree roots = %#v", publicList.Data)
	}
	if len(publicList.Data[0].Children) != 2 {
		t.Fatalf("public tree children = %#v", publicList.Data[0].Children)
	}
	if publicList.Data[0].Children[0].Slug != "api-reference" {
		t.Fatalf("sort order not applied: %#v", publicList.Data[0].Children)
	}

	// 阅读侧按 slug 取到真实正文。
	detail := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents/api-reference", nil)
	detailResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(detailResponse, detail)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("public detail status = %d: %s", detailResponse.Code, detailResponse.Body)
	}
	var detailPayload documentDetailResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detailPayload); err != nil {
		t.Fatal(err)
	}
	if detailPayload.Data.Title != "接口参考" ||
		!strings.Contains(detailPayload.Data.Markdown, "更新后的说明") {
		t.Fatalf("public detail = %#v", detailPayload.Data)
	}
	if detailPayload.Data.Version != "1.0.0" || len(detailPayload.Data.Outline) == 0 {
		t.Fatalf("detail metadata = %#v", detailPayload.Data)
	}

	// 删除父文档时子文档一并移除。
	remove := httptest.NewRequest(http.MethodDelete, base+"/"+guide.ID, nil)
	remove.AddCookie(cookie)
	removeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(removeResponse, remove)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", removeResponse.Code, removeResponse.Body)
	}
	remaining, err := projectDocumentRepositoryStore.ListByProject(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("documents should cascade on delete, remaining=%#v", remaining)
	}
}

func TestAuthorDocumentValidation(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	base := "/api/v1/author/projects/" + project.ID + "/documents"
	first := createDocument(t, cookie, project.ID,
		`{"slug":"overview","title":"总览","markdown":"# 总览"}`)

	for name, testCase := range map[string]struct {
		body   string
		status int
		code   string
	}{
		"重复 slug": {`{"slug":"overview","title":"重复","markdown":""}`, http.StatusConflict, "document_slug_exists"},
		"非法 slug": {`{"slug":"Bad Slug","title":"标题","markdown":""}`, http.StatusUnprocessableEntity, "invalid_document"},
		"空标题":     {`{"slug":"empty-title","title":"","markdown":""}`, http.StatusUnprocessableEntity, "invalid_document"},
		"父级不存在":   {`{"slug":"orphan","title":"孤儿","parent_id":"document-missing","markdown":""}`, http.StatusNotFound, "document_not_found"},
	} {
		request := httptest.NewRequest(http.MethodPost, base, strings.NewReader(testCase.body))
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != testCase.status {
			t.Fatalf("%s status = %d, want %d: %s", name, response.Code, testCase.status, response.Body)
		}
		if !strings.Contains(response.Body.String(), testCase.code) {
			t.Fatalf("%s body = %s, want code %s", name, response.Body, testCase.code)
		}
	}

	// 不能把文档挂到自己下面。
	selfMove := httptest.NewRequest(http.MethodPost, base+"/"+first.ID+"/move",
		strings.NewReader(`{"parent_id":"`+first.ID+`","sort_order":0}`))
	selfMove.AddCookie(cookie)
	selfMoveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(selfMoveResponse, selfMove)
	if selfMoveResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("self move status = %d: %s", selfMoveResponse.Code, selfMoveResponse.Body)
	}

	// 不能把父文档移动到自己的子文档下，避免形成环。
	child := createDocument(t, cookie, project.ID,
		`{"slug":"child","title":"子文档","parent_id":"`+first.ID+`","markdown":""}`)
	cycleMove := httptest.NewRequest(http.MethodPost, base+"/"+first.ID+"/move",
		strings.NewReader(`{"parent_id":"`+child.ID+`","sort_order":0}`))
	cycleMove.AddCookie(cookie)
	cycleResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(cycleResponse, cycleMove)
	if cycleResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cycle move status = %d: %s", cycleResponse.Code, cycleResponse.Body)
	}
	if !strings.Contains(cycleResponse.Body.String(), "invalid_document_parent") {
		t.Fatalf("cycle move body = %s", cycleResponse.Body)
	}
}

func TestAuthorDocumentPermissions(t *testing.T) {
	cookie, project := setupAuthorDocumentTest(t)
	createDocument(t, cookie, project.ID, `{"slug":"only-doc","title":"唯一文档","markdown":"# 唯一文档"}`)
	base := "/api/v1/author/projects/" + project.ID + "/documents"

	// 匿名访问必须被拒绝。
	anonymous := httptest.NewRequest(http.MethodGet, base, nil)
	anonymousResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(anonymousResponse, anonymous)
	if anonymousResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymousResponse.Code)
	}

	// 其他登录用户既不是所有者也不是协作者，应看到 404 而不是 403，避免泄露项目归属。
	otherCookie, _ := registerTestUser(t, "doc-outsider@example.com", "外部用户")
	outsider := httptest.NewRequest(http.MethodGet, base, nil)
	outsider.AddCookie(otherCookie)
	outsiderResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(outsiderResponse, outsider)
	if outsiderResponse.Code != http.StatusNotFound {
		t.Fatalf("outsider status = %d, want 404: %s", outsiderResponse.Code, outsiderResponse.Body)
	}
}

// TestProjectWithoutDocumentsFallsBackToDescription 保证旧项目升级后阅读页不会空白。
func TestProjectWithoutDocumentsFallsBackToDescription(t *testing.T) {
	_, project := setupAuthorDocumentTest(t)

	treeRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents", nil)
	treeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(treeResponse, treeRequest)
	var tree documentListResponse
	if err := json.Unmarshal(treeResponse.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	if len(tree.Data) != 1 || tree.Data[0].Slug != publishedDocumentSlug {
		t.Fatalf("fallback tree = %#v", tree.Data)
	}

	detailRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.Slug+"/documents/"+publishedDocumentSlug, nil)
	detailResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(detailResponse, detailRequest)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("fallback detail status = %d: %s", detailResponse.Code, detailResponse.Body)
	}
	var detail documentDetailResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.Data.Markdown, "这是项目自身的正文") {
		t.Fatalf("fallback markdown = %q", detail.Data.Markdown)
	}
}

func TestBuildDocumentTreeSortsAndNests(t *testing.T) {
	root := "document-root"
	documents := []projectDocument{
		{ID: "document-b", Slug: "b", Title: "B", SortOrder: 2},
		{ID: root, Slug: "root", Title: "Root", SortOrder: 0},
		{ID: "document-a", Slug: "a", Title: "A", SortOrder: 1},
		{ID: "document-child2", Slug: "c2", Title: "C2", SortOrder: 1, ParentID: &root},
		{ID: "document-child1", Slug: "c1", Title: "C1", SortOrder: 0, ParentID: &root},
	}
	tree := buildDocumentTree(documents)
	if len(tree) != 3 {
		t.Fatalf("roots = %d, want 3: %#v", len(tree), tree)
	}
	// 根节点按 sort_order 升序。
	if tree[0].Slug != "root" || tree[1].Slug != "a" || tree[2].Slug != "b" {
		t.Fatalf("root order = %v %v %v", tree[0].Slug, tree[1].Slug, tree[2].Slug)
	}
	if len(tree[0].Children) != 2 ||
		tree[0].Children[0].Slug != "c1" || tree[0].Children[1].Slug != "c2" {
		t.Fatalf("children order = %#v", tree[0].Children)
	}
}
