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

// setupAdminConsoleTest 重置管理控制台涉及的全部内存仓库，返回清理函数。
func setupAdminConsoleTest(t *testing.T) {
	t.Helper()
	originalAuth := authRepositoryStore
	originalProjects := managedProjectRepositoryStore
	originalBans := banRepositoryStore
	originalKeys := apiKeyRepositoryStore
	originalAudit := adminAuditRepositoryStore
	originalComments := commentRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	banRepositoryStore = newMemoryBanRepository()
	apiKeyRepositoryStore = newMemoryAPIKeyRepository()
	adminAuditRepositoryStore = newMemoryAdminAuditRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		managedProjectRepositoryStore = originalProjects
		banRepositoryStore = originalBans
		apiKeyRepositoryStore = originalKeys
		adminAuditRepositoryStore = originalAudit
		commentRepositoryStore = originalComments
		authRateLimiter = originalLimiter
	})
}

const adminTestProjectBody = `{"slug":"admin-demo","name":"Admin Demo","summary":"用于验证管理控制台的示例项目",
	"description":"这是足够长的项目详细介绍，用于验证管理端列表与下架流程的完整过程。",
	"category":"Coding Agent","tags":["Agent"],"tech_stack":["Go"],
	"license":"MIT","repository_url":"https://github.com/example/demo","current_version":"0.1.0"}`

func TestAdminConsoleRequiresAdmin(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	userCookie, _ := registerTestUser(t, "console-user@example.com", "普通用户")

	for _, path := range []string{
		"/api/v1/admin/stats", "/api/v1/admin/users",
		"/api/v1/admin/projects", "/api/v1/admin/api-keys", "/api/v1/admin/audit",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(userCookie)
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("non-admin GET %s status = %d, want 403", path, response.Code)
		}
	}

	// 未登录同样不可访问（401）。
	anon := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	anonResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(anonResponse, anon)
	if anonResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous stats status = %d, want 401", anonResponse.Code)
	}
}

func TestAdminStatsAndUsers(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	_, target := registerTestUser(t, "console-target@example.com", "目标用户")

	stats := doAdminRequest(t, http.MethodGet, "/api/v1/admin/stats", "", adminCookie)
	if stats.Code != http.StatusOK {
		t.Fatalf("stats status = %d: %s", stats.Code, stats.Body)
	}
	var statsBody struct {
		Data struct {
			Users          int `json:"users"`
			PendingReviews int `json:"pending_reviews"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stats.Body.Bytes(), &statsBody); err != nil {
		t.Fatal(err)
	}
	if statsBody.Data.Users < 2 {
		t.Fatalf("stats users = %d, want >= 2", statsBody.Data.Users)
	}

	users := doAdminRequest(t, http.MethodGet, "/api/v1/admin/users?search=target", "", adminCookie)
	if users.Code != http.StatusOK || !strings.Contains(users.Body.String(), target.Email) {
		t.Fatalf("users list status = %d: %s", users.Code, users.Body)
	}
	if strings.Contains(users.Body.String(), "console-admin@example.com") {
		t.Fatalf("search=target must not return admin: %s", users.Body)
	}
}

func TestAdminBanBlocksWrites(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	userCookie, target := registerTestUser(t, "console-banme@example.com", "待封禁用户")

	// 封禁前可创建项目。
	before := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects", adminTestProjectBody, userCookie)
	if before.Code != http.StatusCreated {
		t.Fatalf("pre-ban create status = %d: %s", before.Code, before.Body)
	}

	// 封禁自己应被拒绝。
	banSelf := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/users/"+adminUserID(t, adminCookie)+"/ban", "", adminCookie)
	if banSelf.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ban self status = %d, want 422", banSelf.Code)
	}

	// 封禁目标用户。
	ban := doAdminRequest(t, http.MethodPost, "/api/v1/admin/users/"+target.ID+"/ban",
		`{"reason":"垃圾内容"}`, adminCookie)
	if ban.Code != http.StatusNoContent {
		t.Fatalf("ban status = %d: %s", ban.Code, ban.Body)
	}

	// 被封禁后写操作（创建项目）应 403。
	blocked := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects", adminTestProjectBody, userCookie)
	if blocked.Code != http.StatusForbidden || !strings.Contains(blocked.Body.String(), "user_banned") {
		t.Fatalf("banned create status = %d: %s", blocked.Code, blocked.Body)
	}

	// 被封禁用户仍可浏览（读）项目列表。
	browse := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	browse.AddCookie(userCookie)
	browseResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(browseResponse, browse)
	if browseResponse.Code != http.StatusOK {
		t.Fatalf("banned browse status = %d, want 200", browseResponse.Code)
	}

	// 用户列表应显示 banned=true。
	list := doAdminRequest(t, http.MethodGet, "/api/v1/admin/users?search=banme", "", adminCookie)
	if !strings.Contains(list.Body.String(), `"banned":true`) {
		t.Fatalf("banned flag missing: %s", list.Body)
	}

	// 解封后恢复写权限。
	unban := doAdminRequest(t, http.MethodDelete, "/api/v1/admin/users/"+target.ID+"/ban", "", adminCookie)
	if unban.Code != http.StatusNoContent {
		t.Fatalf("unban status = %d: %s", unban.Code, unban.Body)
	}
	after := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects",
		strings.Replace(adminTestProjectBody, "admin-demo", "admin-demo-2", 1), userCookie)
	if after.Code != http.StatusCreated {
		t.Fatalf("post-unban create status = %d: %s", after.Code, after.Body)
	}

	// 解封不存在的封禁应 404。
	missing := doAdminRequest(t, http.MethodDelete, "/api/v1/admin/users/"+target.ID+"/ban", "", adminCookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unban missing status = %d, want 404", missing.Code)
	}
}

func TestAdminProjectsListAndTakedown(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	ownerCookie, _ := registerTestUser(t, "console-owner@example.com", "项目作者")

	create := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects", adminTestProjectBody, ownerCookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body)
	}
	var created struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// 管理端跨状态列表应能看到 draft 项目（作者端列表之外）。
	list := doAdminRequest(t, http.MethodGet, "/api/v1/admin/projects", "", adminCookie)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), created.Data.ID) {
		t.Fatalf("admin projects list status = %d: %s", list.Code, list.Body)
	}

	takedown := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/projects/"+created.Data.ID+"/takedown", `{"reason":"违规"}`, adminCookie)
	if takedown.Code != http.StatusOK || !strings.Contains(takedown.Body.String(), `"status":"archived"`) {
		t.Fatalf("takedown status = %d: %s", takedown.Code, takedown.Body)
	}

	// 重复下架应 409。
	again := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/projects/"+created.Data.ID+"/takedown", "", adminCookie)
	if again.Code != http.StatusConflict {
		t.Fatalf("repeat takedown status = %d, want 409", again.Code)
	}
}

func TestAdminAPIKeysAndOpenEndpoint(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")

	// 缺少密钥时开放端点 401。
	noKey := httptest.NewRequest(http.MethodGet, "/api/v1/open/projects", nil)
	noKeyResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(noKeyResponse, noKey)
	if noKeyResponse.Code != http.StatusUnauthorized {
		t.Fatalf("open without key status = %d, want 401", noKeyResponse.Code)
	}

	issue := doAdminRequest(t, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"demo key"}`, adminCookie)
	if issue.Code != http.StatusCreated {
		t.Fatalf("issue key status = %d: %s", issue.Code, issue.Body)
	}
	var issued struct {
		Data struct {
			Key       apiKey `json:"key"`
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Data.Plaintext == "" || issued.Data.Key.ID == "" {
		t.Fatalf("issued key missing plaintext/id: %s", issue.Body)
	}

	// 使用明文密钥访问开放端点。
	authed := httptest.NewRequest(http.MethodGet, "/api/v1/open/projects", nil)
	authed.Header.Set("Authorization", "Bearer "+issued.Data.Plaintext)
	authedResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(authedResponse, authed)
	if authedResponse.Code != http.StatusOK {
		t.Fatalf("open with key status = %d: %s", authedResponse.Code, authedResponse.Body)
	}

	// 列表不应回传明文。
	list := doAdminRequest(t, http.MethodGet, "/api/v1/admin/api-keys", "", adminCookie)
	if strings.Contains(list.Body.String(), issued.Data.Plaintext) {
		t.Fatalf("api key list leaked plaintext: %s", list.Body)
	}

	// 撤销后开放端点 401。
	revoke := doAdminRequest(t, http.MethodDelete, "/api/v1/admin/api-keys/"+issued.Data.Key.ID, "", adminCookie)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d: %s", revoke.Code, revoke.Body)
	}
	revoked := httptest.NewRequest(http.MethodGet, "/api/v1/open/projects", nil)
	revoked.Header.Set("Authorization", "Bearer "+issued.Data.Plaintext)
	revokedResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(revokedResponse, revoked)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("open with revoked key status = %d, want 401", revokedResponse.Code)
	}

	// 审计日志应记录密钥签发与撤销。
	audit := doAdminRequest(t, http.MethodGet, "/api/v1/admin/audit", "", adminCookie)
	if !strings.Contains(audit.Body.String(), "api_key_issued") ||
		!strings.Contains(audit.Body.String(), "api_key_revoked") {
		t.Fatalf("audit log missing key events: %s", audit.Body)
	}
}

func TestAdminUserStats(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	_, target := registerTestUser(t, "console-target@example.com", "目标用户")

	// 非管理员应被拒绝（403）。
	userCookie, _ := registerTestUser(t, "stats-user@example.com", "普通用户")
	denied := doAdminRequest(t, http.MethodGet, "/api/v1/admin/user-stats", "", userCookie)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-admin user-stats status = %d, want 403", denied.Code)
	}

	// 封禁一名用户，验证 banned 计数。
	ban := doAdminRequest(t, http.MethodPost, "/api/v1/admin/users/"+target.ID+"/ban", `{"reason":"spam"}`, adminCookie)
	if ban.Code != http.StatusNoContent {
		t.Fatalf("ban status = %d: %s", ban.Code, ban.Body)
	}

	response := doAdminRequest(t, http.MethodGet, "/api/v1/admin/user-stats?days=7", "", adminCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("user-stats status = %d: %s", response.Code, response.Body)
	}
	var body struct {
		Data struct {
			TotalUsers    int `json:"total_users"`
			Banned        int `json:"banned"`
			Days          int `json:"days"`
			Registrations []struct {
				Date  string `json:"date"`
				Count int    `json:"count"`
			} `json:"registrations"`
			LevelHistogram []struct {
				Level int `json:"level"`
				Count int `json:"count"`
			} `json:"level_histogram"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Days != 7 {
		t.Fatalf("days = %d, want 7", body.Data.Days)
	}
	if len(body.Data.Registrations) != 7 {
		t.Fatalf("registrations len = %d, want 7 (zero-filled)", len(body.Data.Registrations))
	}
	if body.Data.TotalUsers < 3 {
		t.Fatalf("total_users = %d, want >= 3", body.Data.TotalUsers)
	}
	if body.Data.Banned != 1 {
		t.Fatalf("banned = %d, want 1", body.Data.Banned)
	}
	// 等级直方图应覆盖 1..maxUserLevel，且今天注册的用户被计入最后一个桶。
	if len(body.Data.LevelHistogram) != maxUserLevel {
		t.Fatalf("level_histogram len = %d, want %d", len(body.Data.LevelHistogram), maxUserLevel)
	}
	trendSum := 0
	for _, point := range body.Data.Registrations {
		trendSum += point.Count
	}
	if trendSum < 3 {
		t.Fatalf("registration trend sum = %d, want >= 3 (all registered today)", trendSum)
	}
}

func TestAdminAuditActionFilter(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	_, target := registerTestUser(t, "console-target@example.com", "目标用户")

	// 触发两类审计动作：签发密钥 + 封禁用户。
	if issue := doAdminRequest(t, http.MethodPost, "/api/v1/admin/api-keys", `{"name":"k"}`, adminCookie); issue.Code != http.StatusCreated {
		t.Fatalf("issue key status = %d: %s", issue.Code, issue.Body)
	}
	if ban := doAdminRequest(t, http.MethodPost, "/api/v1/admin/users/"+target.ID+"/ban", `{"reason":"spam"}`, adminCookie); ban.Code != http.StatusNoContent {
		t.Fatalf("ban status = %d: %s", ban.Code, ban.Body)
	}

	// 按 action 过滤：仅返回封禁事件，不含密钥签发。
	filtered := doAdminRequest(t, http.MethodGet, "/api/v1/admin/audit?action=user_banned", "", adminCookie)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered audit status = %d: %s", filtered.Code, filtered.Body)
	}
	if !strings.Contains(filtered.Body.String(), "user_banned") {
		t.Fatalf("filtered audit missing user_banned: %s", filtered.Body)
	}
	if strings.Contains(filtered.Body.String(), "api_key_issued") {
		t.Fatalf("action filter leaked other actions: %s", filtered.Body)
	}
}

func TestReviewThroughConsoleEndpoints(t *testing.T) {
	setupAdminConsoleTest(t)
	t.Setenv("ADMIN_EMAILS", "console-admin@example.com")
	adminCookie, _ := registerTestUser(t, "console-admin@example.com", "控制台管理员")
	ownerCookie, _ := registerTestUser(t, "console-owner@example.com", "项目作者")

	// 作者创建并提交项目进入待审核。
	create := doAdminRequest(t, http.MethodPost, "/api/v1/author/projects", adminTestProjectBody, ownerCookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body)
	}
	var created struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	submit := doAdminRequest(t, http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", "", ownerCookie)
	if submit.Code != http.StatusOK {
		t.Fatalf("submit status = %d: %s", submit.Code, submit.Body)
	}

	// 控制台内容审核复用的列表端点应能看到该项目。
	pending := doAdminRequest(t, http.MethodGet, "/api/v1/admin/reviews", "", adminCookie)
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), created.Data.ID) {
		t.Fatalf("pending reviews status = %d: %s", pending.Code, pending.Body)
	}

	// 通过审核端点批准，项目应转为已发布。
	approve := doAdminRequest(t, http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/approve", "", adminCookie)
	if approve.Code != http.StatusOK || !strings.Contains(approve.Body.String(), `"status":"published"`) {
		t.Fatalf("approve status = %d: %s", approve.Code, approve.Body)
	}

	// 审核动作应写入审计日志，且可按 action 过滤到。
	audit := doAdminRequest(t, http.MethodGet, "/api/v1/admin/audit?action=project_review_approve", "", adminCookie)
	if !strings.Contains(audit.Body.String(), "project_review_approve") {
		t.Fatalf("audit missing review approve: %s", audit.Body)
	}
}

// doAdminRequest 发起带 cookie 的请求并返回响应记录器。
func doAdminRequest(t *testing.T, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	return response
}

// adminUserID 通过 /api/v1/auth/me 取得当前登录用户 ID。
func adminUserID(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	var body struct {
		Data authUser `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("auth/me decode: %v", err)
	}
	return body.Data.ID
}

func TestMySQLAdminRepositoriesIntegration(t *testing.T) {
	database := requireTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID := "user-admin-" + newRequestID()
	adminID := "user-admin-actor-" + newRequestID()
	for _, seed := range []struct{ id, email, name string }{
		{userID, userID + "@example.com", "封禁目标"},
		{adminID, adminID + "@example.com", "管理员"},
	} {
		if _, err := database.ExecContext(ctx,
			`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			seed.id, seed.email, seed.name, "integration-only"); err != nil {
			t.Fatalf("create user %s: %v", seed.id, err)
		}
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM users WHERE id IN (?, ?)`, userID, adminID)
	})

	bans := newMySQLBanRepository(database)
	if err := bans.Ban(ctx, userID, adminID, "spam", time.Now().UTC()); err != nil {
		t.Fatalf("ban: %v", err)
	}
	banned, err := bans.IsBanned(ctx, userID)
	if err != nil || !banned {
		t.Fatalf("IsBanned = %v err=%v, want true", banned, err)
	}
	set, err := bans.BannedSet(ctx, []string{userID, adminID})
	if err != nil || !set[userID] || set[adminID] {
		t.Fatalf("BannedSet = %v err=%v", set, err)
	}
	if unbanned, err := bans.Unban(ctx, userID); err != nil || !unbanned {
		t.Fatalf("Unban = %v err=%v", unbanned, err)
	}

	keys := newMySQLAPIKeyRepository(database)
	plaintext, keyHash, prefix, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	key := apiKey{ID: "apikey-" + newRequestID(), OwnerID: adminID, Name: "demo", Prefix: prefix, CreatedAt: time.Now().UTC()}
	if err := keys.Create(ctx, key, keyHash); err != nil {
		t.Fatalf("create key: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DELETE FROM api_keys WHERE id = ?`, key.ID) })
	found, ok, err := keys.FindActiveByHash(ctx, keyHash)
	if err != nil || !ok || found.ID != key.ID {
		t.Fatalf("FindActiveByHash = %v ok=%v err=%v", found, ok, err)
	}
	_ = plaintext
	if revoked, err := keys.Revoke(ctx, key.ID, adminID, time.Now().UTC()); err != nil || !revoked {
		t.Fatalf("Revoke = %v err=%v", revoked, err)
	}
	if _, ok, _ := keys.FindActiveByHash(ctx, keyHash); ok {
		t.Fatal("revoked key must not be active")
	}

	audit := newMySQLAdminAuditRepository(database)
	entryID := "audit-" + newRequestID()
	if err := audit.Record(ctx, adminAuditEntry{
		ID: entryID, ActorID: adminID, ActorEmail: adminID + "@example.com",
		Action: "user_banned", Target: userID, Detail: "spam", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DELETE FROM admin_audit WHERE id = ?`, entryID) })
	entries, err := audit.List(ctx, "", 10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	var seen bool
	for _, entry := range entries {
		if entry.ID == entryID {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("recorded audit entry %s not returned", entryID)
	}
}
