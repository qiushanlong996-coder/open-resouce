package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupContentReportTest 重置举报相关的内存仓库，复用管理控制台的其余重置。
func setupContentReportTest(t *testing.T) {
	t.Helper()
	setupAdminConsoleTest(t)
	original := contentReportRepositoryStore
	contentReportRepositoryStore = newMemoryContentReportRepository()
	t.Cleanup(func() { contentReportRepositoryStore = original })
}

func TestReportValidation(t *testing.T) {
	setupContentReportTest(t)
	t.Setenv("ADMIN_EMAILS", "report-admin@example.com")
	adminCookie, _ := registerTestUser(t, "report-admin@example.com", "举报管理员")
	userCookie, _ := registerTestUser(t, "report-user@example.com", "举报用户")

	// 未登录：401。
	anon := httptest.NewRequest(http.MethodPost, "/api/v1/reports",
		strings.NewReader(`{"target_type":"comment","target_id":"comment-1","reason":"垃圾广告"}`))
	anon.Header.Set("Content-Type", "application/json")
	anonResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(anonResponse, anon)
	if anonResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anon report status = %d, want 401", anonResponse.Code)
	}

	// 非法 target_type：422。
	badType := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"user","target_id":"u-1","reason":"垃圾广告"}`, userCookie)
	if badType.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad target_type status = %d, want 422: %s", badType.Code, badType.Body)
	}

	// 缺少原因：422。
	noReason := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"comment","target_id":"comment-1","reason":"  "}`, userCookie)
	if noReason.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty reason status = %d, want 422: %s", noReason.Code, noReason.Body)
	}

	// 被封禁用户：403。
	banned := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"comment","target_id":"comment-1","reason":"垃圾广告"}`, userCookie)
	if banned.Code != http.StatusCreated {
		t.Fatalf("pre-ban report status = %d, want 201: %s", banned.Code, banned.Body)
	}
	targetID := adminUserID(t, userCookie)
	if ban := doAdminRequest(t, http.MethodPost, "/api/v1/admin/users/"+targetID+"/ban",
		`{"reason":"spam"}`, adminCookie); ban.Code != http.StatusNoContent {
		t.Fatalf("ban status = %d: %s", ban.Code, ban.Body)
	}
	blocked := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"comment","target_id":"comment-2","reason":"违规内容"}`, userCookie)
	if blocked.Code != http.StatusForbidden || !strings.Contains(blocked.Body.String(), "user_banned") {
		t.Fatalf("banned report status = %d, want 403 user_banned: %s", blocked.Code, blocked.Body)
	}
}

func TestReportLifecycleAndAdminHandling(t *testing.T) {
	setupContentReportTest(t)
	t.Setenv("ADMIN_EMAILS", "report-admin@example.com")
	adminCookie, _ := registerTestUser(t, "report-admin@example.com", "举报管理员")
	userCookie, reporter := registerTestUser(t, "report-user@example.com", "举报用户")

	create := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"comment","target_id":"comment-abc","reason":"垃圾广告","detail":"含推广链接"}`, userCookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create report status = %d: %s", create.Code, create.Body)
	}
	var created struct {
		Data contentReport `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.Status != reportStatusOpen {
		t.Fatalf("unexpected created report: %s", create.Body)
	}

	// 重复举报同一目标：幂等返回 200。
	dup := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"comment","target_id":"comment-abc","reason":"垃圾广告"}`, userCookie)
	if dup.Code != http.StatusOK || !strings.Contains(dup.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate report status = %d, want 200 duplicate: %s", dup.Code, dup.Body)
	}

	// 非管理员访问管理端列表：403。
	denied := doAdminRequest(t, http.MethodGet, "/api/v1/admin/reports", "", userCookie)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-admin reports list status = %d, want 403", denied.Code)
	}

	// 管理员列出未处理举报，应看到举报人邮箱与目标。
	list := doAdminRequest(t, http.MethodGet, "/api/v1/admin/reports?status=open", "", adminCookie)
	if list.Code != http.StatusOK {
		t.Fatalf("admin reports list status = %d: %s", list.Code, list.Body)
	}
	if !strings.Contains(list.Body.String(), reporter.Email) ||
		!strings.Contains(list.Body.String(), "comment-abc") {
		t.Fatalf("reports list missing reporter/target: %s", list.Body)
	}

	// 处理该举报：状态转为 resolved。
	resolve := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/reports/"+created.Data.ID+"/resolve", "", adminCookie)
	if resolve.Code != http.StatusOK || !strings.Contains(resolve.Body.String(), `"status":"resolved"`) {
		t.Fatalf("resolve status = %d: %s", resolve.Code, resolve.Body)
	}

	// 处理后未处理队列应为空。
	openList := doAdminRequest(t, http.MethodGet, "/api/v1/admin/reports?status=open", "", adminCookie)
	if strings.Contains(openList.Body.String(), created.Data.ID) {
		t.Fatalf("resolved report still in open list: %s", openList.Body)
	}

	// 处理不存在的举报：404。
	missing := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/reports/report-missing/dismiss", "", adminCookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("dismiss missing status = %d, want 404: %s", missing.Code, missing.Body)
	}

	// 处理动作应写入审计日志。
	audit := doAdminRequest(t, http.MethodGet, "/api/v1/admin/audit?action=report_resolved", "", adminCookie)
	if !strings.Contains(audit.Body.String(), "report_resolved") {
		t.Fatalf("audit missing report_resolved: %s", audit.Body)
	}
}

func TestReportProjectSlugResolvesToID(t *testing.T) {
	setupContentReportTest(t)
	t.Setenv("ADMIN_EMAILS", "report-admin@example.com")
	adminCookie, _ := registerTestUser(t, "report-admin@example.com", "举报管理员")
	ownerCookie, _ := registerTestUser(t, "report-owner@example.com", "项目作者")
	userCookie, _ := registerTestUser(t, "report-reporter@example.com", "举报用户")

	// 作者创建、提交并由管理员发布，使 slug 可解析为内部 ID。
	create := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects", adminTestProjectBody, ownerCookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create project status = %d: %s", create.Code, create.Body)
	}
	var project struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if submit := doAdminRequest(t, http.MethodPost,
		"/api/v1/author/projects/"+project.Data.ID+"/submit", "", ownerCookie); submit.Code != http.StatusOK {
		t.Fatalf("submit status = %d: %s", submit.Code, submit.Body)
	}
	if approve := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/reviews/"+project.Data.ID+"/approve", "", adminCookie); approve.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approve.Code, approve.Body)
	}

	// 用 slug 举报项目，存储的应是内部 ID。
	report := doAdminRequest(t, http.MethodPost, "/api/v1/reports",
		`{"target_type":"project","target_id":"admin-demo","reason":"侵权"}`, userCookie)
	if report.Code != http.StatusCreated {
		t.Fatalf("report project status = %d: %s", report.Code, report.Body)
	}
	var created struct {
		Data contentReport `json:"data"`
	}
	if err := json.Unmarshal(report.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.TargetID != project.Data.ID {
		t.Fatalf("target_id = %q, want internal id %q", created.Data.TargetID, project.Data.ID)
	}
}
