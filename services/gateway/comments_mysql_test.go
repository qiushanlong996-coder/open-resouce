package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMySQLDSNFromDatabaseURL(t *testing.T) {
	dsn, err := mysqlDSN("mysql://app_user:p%40ss@db.example:3306/open_resouce")
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	for _, expected := range []string{
		"app_user:p@ss@tcp(db.example:3306)/open_resouce",
		"parseTime=true",
		"clientFoundRows=true",
		"collation=utf8mb4_unicode_ci",
	} {
		if !strings.Contains(dsn, expected) {
			t.Fatalf("DSN does not contain %q", expected)
		}
	}
}

func TestMySQLDSNRejectsIncompleteURL(t *testing.T) {
	for _, databaseURL := range []string{
		"",
		"postgres://user:password@db.example/open_resouce",
		"mysql://db.example/open_resouce",
		"mysql://user:password@db.example",
	} {
		if _, err := mysqlDSN(databaseURL); err == nil {
			t.Fatalf("expected invalid DATABASE_URL %q to fail", databaseURL)
		}
	}
}

func TestMySQLCommentRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("MYSQL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MYSQL_TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := openMySQLDatabase(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	repository := newMySQLCommentRepository(db)
	suffix := newRequestID()
	documentID := "integration-document-" + suffix
	commentID := "comment-" + suffix
	replyID := "comment-reply-" + suffix
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := db.ExecContext(cleanupContext, "DELETE FROM document_comments WHERE id = ?", replyID); err != nil {
			t.Errorf("clean up integration reply: %v", err)
		}
		if _, err := db.ExecContext(cleanupContext, "DELETE FROM document_comments WHERE id = ?", commentID); err != nil {
			t.Errorf("clean up integration comment: %v", err)
		}
	})

	createdAt := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	created, err := repository.Create(ctx, documentComment{
		ID:         commentID,
		DocumentID: documentID,
		BlockID:    "integration-block",
		Author:     "集成测试",
		Quote:      "测试摘录",
		Body:       "验证 MySQL 评论仓库",
		Status:     "open",
		CreatedAt:  createdAt,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if created.ID != commentID {
		t.Fatalf("created comment ID = %q", created.ID)
	}

	comments, err := repository.List(ctx, documentID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != commentID {
		t.Fatalf("unexpected comments: %#v", comments)
	}
	updated, found, err := repository.UpdateComment(ctx, documentID, commentID, "", "更新后的集成评论", createdAt)
	if err != nil || !found || updated.Body != "更新后的集成评论" {
		t.Fatalf("update comment: found=%v err=%v comment=%#v", found, err, updated)
	}

	reply, found, err := repository.CreateReply(ctx, commentID, documentComment{
		ID: replyID, DocumentID: documentID, Author: "集成回复",
		Body: "验证 MySQL 回复", Status: "open", CreatedAt: createdAt,
	})
	if err != nil || !found || reply.ParentID == nil {
		t.Fatalf("create reply: found=%v err=%v reply=%#v", found, err, reply)
	}
	comments, err = repository.List(ctx, documentID)
	if err != nil || len(comments) != 1 || len(comments[0].Replies) != 1 || comments[0].Replies[0].ID != replyID {
		t.Fatalf("list threaded comments: err=%v comments=%#v", err, comments)
	}
	if comments[0].ReplyCount != 1 {
		t.Fatalf("reply count = %d, want 1", comments[0].ReplyCount)
	}
	updatedReply, found, err := repository.UpdateReply(ctx, documentID, commentID, replyID, "", "更新后的集成回复", createdAt)
	if err != nil || !found || updatedReply.Body != "更新后的集成回复" {
		t.Fatalf("update reply: found=%v err=%v reply=%#v", found, err, updatedReply)
	}

	deletedAt := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	deleted, err := repository.DeleteReply(ctx, documentID, commentID, replyID, "", deletedAt)
	if err != nil || !deleted {
		t.Fatalf("delete reply: deleted=%v err=%v", deleted, err)
	}
	comments, err = repository.List(ctx, documentID)
	if err != nil || len(comments) != 1 || comments[0].ReplyCount != 0 || len(comments[0].Replies) != 0 {
		t.Fatalf("list after reply deletion: err=%v comments=%#v", err, comments)
	}
	if deleted, err := repository.DeleteReply(ctx, documentID, commentID, replyID, "", deletedAt); err != nil || deleted {
		t.Fatalf("delete reply idempotently: deleted=%v err=%v", deleted, err)
	}

	resolvedAt := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	resolved, found, err := repository.Resolve(ctx, documentID, commentID, "", resolvedAt)
	if err != nil || !found {
		t.Fatalf("resolve comment: found=%v err=%v", found, err)
	}
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("unexpected resolved comment: %#v", resolved)
	}

	resolvedAgain, found, err := repository.Resolve(ctx, documentID, commentID, "", resolvedAt)
	if err != nil || !found || resolvedAgain.ResolvedAt == nil {
		t.Fatalf("resolve comment idempotently: found=%v err=%v comment=%#v", found, err, resolvedAgain)
	}
	if deleted, err := repository.DeleteComment(ctx, documentID, commentID, "", resolvedAt); err != nil || !deleted {
		t.Fatalf("delete comment: deleted=%v err=%v", deleted, err)
	}
	if comments, err := repository.List(ctx, documentID); err != nil || len(comments) != 0 {
		t.Fatalf("list after comment deletion: err=%v comments=%#v", err, comments)
	}
}
