package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMySQLNotificationRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("MYSQL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MYSQL_TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := openMySQLDatabase(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	// 关闭必须通过 t.Cleanup 注册：defer 在函数 return 时执行，会早于 t.Cleanup 里的清理语句，
	// 导致连接已关闭、临时数据无法删除。t.Cleanup 按后进先出执行，删除先于关闭。
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	// 通知表以 recipient_id 外键指向 users，集成测试需要先创建临时接收者。
	recipientID := "user-notify-" + newRequestID()
	actorID := "user-actor-" + newRequestID()
	firstID := "notification-" + newRequestID()
	secondID := "notification-" + newRequestID()
	t.Cleanup(func() {
		if _, err := database.Exec(`DELETE FROM notifications WHERE recipient_id IN (?, ?)`, recipientID, actorID); err != nil {
			t.Errorf("clean up integration notifications: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM users WHERE id IN (?, ?)`, recipientID, actorID); err != nil {
			t.Errorf("clean up integration users: %v", err)
		}
	})

	authRepository := newMySQLAuthRepository(database)
	passwordHash, err := hashPassword("integration-password")
	if err != nil {
		t.Fatal(err)
	}
	for id, name := range map[string]string{recipientID: "通知接收者", actorID: "通知触发者"} {
		if err := authRepository.CreateUser(ctx, authUser{
			ID: id, Email: id + "@example.com", DisplayName: name, PasswordHash: passwordHash,
		}); err != nil {
			t.Fatalf("create user %s: %v", id, err)
		}
	}

	repository := newMySQLNotificationRepository(database)
	createdAt := time.Now().UTC().Truncate(time.Second)
	if err := repository.Create(ctx, notification{
		ID: firstID, RecipientID: recipientID, ActorID: actorID, ActorName: "通知触发者",
		Type: "comment.replied", Title: "通知触发者 回复了你的评论", Body: "集成测试回复正文",
		ProjectSlug: "atlas-agent", DocumentSlug: "quick-start", CommentID: "comment-integration",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("create first notification: %v", err)
	}
	if err := repository.Create(ctx, notification{
		ID: secondID, RecipientID: recipientID, ActorID: actorID, ActorName: "审核管理员",
		Type: "project.rejected", Title: "项目未通过审核", Body: "资料需要补充",
		ProjectSlug: "notify-demo", CreatedAt: createdAt.Add(time.Second),
	}); err != nil {
		t.Fatalf("create second notification: %v", err)
	}

	entries, err := repository.ListByRecipient(ctx, recipientID, maxNotificationPageSize)
	if err != nil || len(entries) != 2 {
		t.Fatalf("list notifications: count=%d err=%v", len(entries), err)
	}
	// 列表按创建时间倒序，最新的驳回通知应排在首位。
	if entries[0].ID != secondID || entries[1].ID != firstID {
		t.Fatalf("unexpected notification order: %#v", entries)
	}
	if entries[1].ProjectSlug != "atlas-agent" || entries[1].DocumentSlug != "quick-start" ||
		entries[1].CommentID != "comment-integration" || entries[1].ReadAt != nil {
		t.Fatalf("unexpected notification payload: %#v", entries[1])
	}

	if count, err := repository.UnreadCount(ctx, recipientID); err != nil || count != 2 {
		t.Fatalf("unread count = %d err=%v", count, err)
	}

	readAt := time.Now().UTC().Truncate(time.Second)
	found, err := repository.MarkRead(ctx, recipientID, firstID, readAt)
	if err != nil || !found {
		t.Fatalf("mark read: found=%v err=%v", found, err)
	}
	if count, err := repository.UnreadCount(ctx, recipientID); err != nil || count != 1 {
		t.Fatalf("unread count after single read = %d err=%v", count, err)
	}
	// 重复标记必须幂等，且不覆盖首次已读时间。
	if found, err := repository.MarkRead(ctx, recipientID, firstID, readAt.Add(time.Hour)); err != nil || !found {
		t.Fatalf("mark read idempotently: found=%v err=%v", found, err)
	}
	entries, err = repository.ListByRecipient(ctx, recipientID, maxNotificationPageSize)
	if err != nil {
		t.Fatalf("list after read: %v", err)
	}
	if entries[1].ReadAt == nil || !entries[1].ReadAt.Equal(readAt) {
		t.Fatalf("read_at was overwritten: %#v", entries[1].ReadAt)
	}

	// 跨用户标记不得命中他人通知。
	if found, err := repository.MarkRead(ctx, actorID, secondID, readAt); err != nil || found {
		t.Fatalf("cross-user mark read: found=%v err=%v", found, err)
	}

	if err := repository.MarkAllRead(ctx, recipientID, readAt); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	if count, err := repository.UnreadCount(ctx, recipientID); err != nil || count != 0 {
		t.Fatalf("unread count after read all = %d err=%v", count, err)
	}

	// 删除用户时通知应随外键级联清理。
	if _, err := database.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, recipientID); err != nil {
		t.Fatalf("delete recipient: %v", err)
	}
	if entries, err := repository.ListByRecipient(ctx, recipientID, maxNotificationPageSize); err != nil || len(entries) != 0 {
		t.Fatalf("notifications survived user deletion: count=%d err=%v", len(entries), err)
	}
}
