package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupNotificationTest(t *testing.T) {
	t.Helper()
	originalAuth, originalComments, originalNotifications, originalLimiter :=
		authRepositoryStore, commentRepositoryStore, notificationRepositoryStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	notificationRepositoryStore = newMemoryNotificationRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, commentRepositoryStore, notificationRepositoryStore, authRateLimiter =
			originalAuth, originalComments, originalNotifications, originalLimiter
	})
}

func listNotifications(t *testing.T, cookie *http.Cookie) notificationListResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list notifications status = %d: %s", response.Code, response.Body)
	}
	var body notificationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestNotificationsRequireAuthentication(t *testing.T) {
	setupNotificationTest(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous notifications status = %d, want 401", response.Code)
	}
}

func TestReplyCreatesNotificationForCommentAuthor(t *testing.T) {
	setupNotificationTest(t)
	authorCookie, author := registerTestUser(t, "notify-author@example.com", "评论作者")
	replierCookie, replier := registerTestUser(t, "notify-replier@example.com", "回复用户")

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"忽略","body":"等待回复的评论"}`),
	)
	createRequest.AddCookie(authorCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create comment status = %d: %s", createResponse.Code, createResponse.Body)
	}
	var created commentResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	replyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/"+created.Data.ID+"/replies",
		strings.NewReader(`{"author":"忽略","body":"这是一条回复"}`),
	)
	replyRequest.AddCookie(replierCookie)
	replyResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(replyResponse, replyRequest)
	if replyResponse.Code != http.StatusCreated {
		t.Fatalf("create reply status = %d: %s", replyResponse.Code, replyResponse.Body)
	}

	notifications := listNotifications(t, authorCookie)
	if len(notifications.Data) != 1 || notifications.UnreadCount != 1 {
		t.Fatalf("author notifications = %#v", notifications)
	}
	entry := notifications.Data[0]
	if entry.Type != "comment.replied" || entry.ActorID != replier.ID ||
		entry.ProjectSlug != "atlas-agent" || entry.DocumentSlug != "quick-start" ||
		entry.CommentID != created.Data.ID || entry.ReadAt != nil {
		t.Fatalf("notification entry = %#v", entry)
	}
	if !strings.Contains(entry.Title, "回复用户") {
		t.Fatalf("notification title = %q", entry.Title)
	}
	_ = author

	// 回复者本人不应收到自己的回复通知。
	replierNotifications := listNotifications(t, replierCookie)
	if len(replierNotifications.Data) != 0 {
		t.Fatalf("replier notifications = %#v", replierNotifications.Data)
	}
}

func TestSelfReplyDoesNotCreateNotification(t *testing.T) {
	setupNotificationTest(t)
	authorCookie, _ := registerTestUser(t, "self-reply@example.com", "自回复用户")

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"忽略","body":"我的评论"}`),
	)
	createRequest.AddCookie(authorCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	var created commentResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	replyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/"+created.Data.ID+"/replies",
		strings.NewReader(`{"author":"忽略","body":"自己回复自己"}`),
	)
	replyRequest.AddCookie(authorCookie)
	replyResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(replyResponse, replyRequest)
	if replyResponse.Code != http.StatusCreated {
		t.Fatalf("self reply status = %d: %s", replyResponse.Code, replyResponse.Body)
	}

	notifications := listNotifications(t, authorCookie)
	if len(notifications.Data) != 0 {
		t.Fatalf("self reply notifications = %#v", notifications.Data)
	}
}

func TestNotificationMarkReadAndReadAll(t *testing.T) {
	setupNotificationTest(t)
	authorCookie, author := registerTestUser(t, "mark-read@example.com", "已读用户")
	replierCookie, _ := registerTestUser(t, "mark-read-actor@example.com", "触发用户")

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"block-atlas-intro","author":"忽略","body":"评论一"}`),
	)
	createRequest.AddCookie(authorCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, createRequest)
	var created commentResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		replyRequest := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/projects/atlas-agent/documents/quick-start/comments/"+created.Data.ID+"/replies",
			strings.NewReader(`{"author":"忽略","body":"一条回复"}`),
		)
		replyRequest.AddCookie(replierCookie)
		replyResponse := httptest.NewRecorder()
		newHandler().ServeHTTP(replyResponse, replyRequest)
		if replyResponse.Code != http.StatusCreated {
			t.Fatalf("reply status = %d: %s", replyResponse.Code, replyResponse.Body)
		}
	}

	notifications := listNotifications(t, authorCookie)
	if len(notifications.Data) != 2 || notifications.UnreadCount != 2 {
		t.Fatalf("notifications before read = %#v", notifications)
	}

	readRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/notifications/"+notifications.Data[0].ID+"/read", nil)
	readRequest.AddCookie(authorCookie)
	readResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusNoContent {
		t.Fatalf("mark read status = %d: %s", readResponse.Code, readResponse.Body)
	}
	if after := listNotifications(t, authorCookie); after.UnreadCount != 1 {
		t.Fatalf("unread after single read = %d", after.UnreadCount)
	}

	// 其他用户不能读别人的通知。
	crossRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/notifications/"+notifications.Data[1].ID+"/read", nil)
	crossRequest.AddCookie(replierCookie)
	crossResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(crossResponse, crossRequest)
	if crossResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user mark read status = %d", crossResponse.Code)
	}

	readAllRequest := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	readAllRequest.AddCookie(authorCookie)
	readAllResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(readAllResponse, readAllRequest)
	if readAllResponse.Code != http.StatusNoContent {
		t.Fatalf("read all status = %d: %s", readAllResponse.Code, readAllResponse.Body)
	}
	final := listNotifications(t, authorCookie)
	if final.UnreadCount != 0 || len(final.Data) != 2 {
		t.Fatalf("notifications after read all = %#v", final)
	}
	_ = author
}

func TestProjectReviewCreatesOwnerNotification(t *testing.T) {
	setupNotificationTest(t)
	originalProjects := managedProjectRepositoryStore
	managedProjectRepositoryStore = newMemoryManagedProjectRepository()
	t.Cleanup(func() { managedProjectRepositoryStore = originalProjects })

	ownerCookie, _ := registerTestUser(t, "review-owner@example.com", "被审核作者")
	adminCookie, _ := registerTestUser(t, "review-admin@example.com", "审核管理员")
	t.Setenv("ADMIN_EMAILS", "review-admin@example.com")

	body := `{"slug":"notify-demo","name":"Notify Demo","summary":"用于验证审核通知的示例项目摘要",
		"description":"这是足够长的项目详细介绍，用来验证审核通过和驳回的站内通知流程。",
		"category":"Coding Agent","tags":["Agent"],"tech_stack":["Go"],
		"license":"MIT","repository_url":"","current_version":"0.1.0"}`
	create := httptest.NewRequest(http.MethodPost, "/api/v1/author/projects", strings.NewReader(body))
	create.AddCookie(ownerCookie)
	createResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create project status = %d: %s", createResponse.Code, createResponse.Body)
	}
	var created struct {
		Data managedProject `json:"data"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	submit := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", nil)
	submit.AddCookie(ownerCookie)
	submitResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusOK {
		t.Fatalf("submit status = %d: %s", submitResponse.Code, submitResponse.Body)
	}

	reject := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/reject", strings.NewReader(`{"reason":"资料需要补充"}`))
	reject.AddCookie(adminCookie)
	rejectResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(rejectResponse, reject)
	if rejectResponse.Code != http.StatusOK {
		t.Fatalf("reject status = %d: %s", rejectResponse.Code, rejectResponse.Body)
	}

	notifications := listNotifications(t, ownerCookie)
	if len(notifications.Data) != 1 {
		t.Fatalf("owner notifications = %#v", notifications.Data)
	}
	rejected := notifications.Data[0]
	if rejected.Type != "project.rejected" || rejected.ProjectSlug != "notify-demo" ||
		rejected.Body != "资料需要补充" || !strings.Contains(rejected.Title, "Notify Demo") {
		t.Fatalf("rejected notification = %#v", rejected)
	}

	resubmit := httptest.NewRequest(http.MethodPost,
		"/api/v1/author/projects/"+created.Data.ID+"/submit", nil)
	resubmit.AddCookie(ownerCookie)
	resubmitResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(resubmitResponse, resubmit)
	if resubmitResponse.Code != http.StatusOK {
		t.Fatalf("resubmit status = %d: %s", resubmitResponse.Code, resubmitResponse.Body)
	}
	approve := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/reviews/"+created.Data.ID+"/approve", nil)
	approve.AddCookie(adminCookie)
	approveResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(approveResponse, approve)
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("approve status = %d: %s", approveResponse.Code, approveResponse.Body)
	}

	final := listNotifications(t, ownerCookie)
	if len(final.Data) != 2 || final.UnreadCount != 2 {
		t.Fatalf("owner notifications after approve = %#v", final)
	}
	if final.Data[0].Type != "project.approved" && final.Data[1].Type != "project.approved" {
		t.Fatalf("missing approved notification: %#v", final.Data)
	}
}

func TestNotificationHubPublishesToSubscriber(t *testing.T) {
	hub := newNotificationEventHub()
	events, cancel := hub.Subscribe("user-1")
	defer cancel()
	hub.Publish(notification{ID: "n-1", RecipientID: "user-1", Type: "comment.replied"})
	select {
	case entry := <-events:
		if entry.ID != "n-1" {
			t.Fatalf("received entry = %#v", entry)
		}
	default:
		t.Fatal("subscriber did not receive notification")
	}
	// 其他用户的订阅不应收到该事件。
	otherEvents, otherCancel := hub.Subscribe("user-2")
	defer otherCancel()
	hub.Publish(notification{ID: "n-2", RecipientID: "user-1"})
	select {
	case entry := <-otherEvents:
		t.Fatalf("unexpected cross-user event: %#v", entry)
	default:
	}
}
