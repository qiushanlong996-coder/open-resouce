package main

import (
	"context"
	"testing"
)

func TestMemoryCommentRepository(t *testing.T) {
	repository := newMemoryCommentRepository()
	ctx := context.Background()

	initial, err := repository.List(ctx, "doc-atlas-quick-start")
	if err != nil {
		t.Fatalf("list seed comments: %v", err)
	}
	if len(initial) != 1 || initial[0].ID != "comment-atlas-001" {
		t.Fatalf("unexpected seed comments: %#v", initial)
	}

	created, err := repository.Create(ctx, documentComment{
		ID: "comment-test", DocumentID: "doc-atlas-quick-start",
		BlockID: "block-atlas-intro", Author: "测试", Body: "仓库测试",
		Status: "open", CreatedAt: "2026-07-27T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	stored, err := repository.List(ctx, "doc-atlas-quick-start")
	if err != nil {
		t.Fatalf("list stored comments: %v", err)
	}
	if created.ID != "comment-test" || len(stored) != 2 {
		t.Fatalf("comment was not stored: %#v", created)
	}
	updatedComment, found, err := repository.UpdateComment(
		ctx, "doc-atlas-quick-start", "comment-test", "", "更新后的评论", "2026-07-27T00:00:10Z",
	)
	if err != nil || !found || updatedComment.Body != "更新后的评论" || updatedComment.UpdatedAt == "" {
		t.Fatalf("comment was not updated: found=%v err=%v comment=%#v", found, err, updatedComment)
	}

	reply, found, err := repository.CreateReply(ctx, "comment-test", documentComment{
		ID: "reply-test", DocumentID: "doc-atlas-quick-start",
		Author: "回复者", Body: "仓库回复测试", Status: "open",
		CreatedAt: "2026-07-27T00:00:30Z",
	})
	if err != nil || !found || reply.ParentID == nil || reply.BlockID != "block-atlas-intro" {
		t.Fatalf("reply was not stored: found=%v err=%v reply=%#v", found, err, reply)
	}
	updatedReply, found, err := repository.UpdateReply(
		ctx, "doc-atlas-quick-start", "comment-test", "reply-test", "", "更新后的回复", "2026-07-27T00:00:40Z",
	)
	if err != nil || !found || updatedReply.Body != "更新后的回复" || updatedReply.UpdatedAt == "" {
		t.Fatalf("reply was not updated: found=%v err=%v reply=%#v", found, err, updatedReply)
	}
	threaded, err := repository.List(ctx, "doc-atlas-quick-start")
	if err != nil || len(threaded) != 2 || len(threaded[1].Replies) != 1 || threaded[1].Replies[0].ID != "reply-test" {
		t.Fatalf("reply was not nested: err=%v comments=%#v", err, threaded)
	}
	if threaded[1].ReplyCount != 1 {
		t.Fatalf("reply count = %d, want 1", threaded[1].ReplyCount)
	}
	deleted, err := repository.DeleteReply(ctx, "doc-atlas-quick-start", "comment-test", "reply-test", "", "2026-07-27T00:00:45Z")
	if err != nil || !deleted {
		t.Fatalf("delete reply: deleted=%v err=%v", deleted, err)
	}
	threaded, err = repository.List(ctx, "doc-atlas-quick-start")
	if err != nil || len(threaded[1].Replies) != 0 || threaded[1].ReplyCount != 0 {
		t.Fatalf("reply was not removed: err=%v comments=%#v", err, threaded)
	}
	if deleted, err := repository.DeleteReply(ctx, "doc-atlas-quick-start", "comment-test", "reply-test", "", "2026-07-27T00:00:46Z"); err != nil || deleted {
		t.Fatalf("delete missing reply: deleted=%v err=%v", deleted, err)
	}
	if _, found, err := repository.CreateReply(ctx, "missing", documentComment{
		ID: "reply-missing", DocumentID: "doc-atlas-quick-start",
	}); err != nil || found {
		t.Fatalf("reply to missing parent: found=%v err=%v", found, err)
	}
	if _, found, err := repository.UpdateComment(ctx, "doc-atlas-quick-start", "missing", "", "正文", "2026-07-27T00:00:50Z"); err != nil || found {
		t.Fatalf("update missing comment: found=%v err=%v", found, err)
	}
	if _, found, err := repository.UpdateReply(ctx, "doc-atlas-quick-start", "comment-test", "missing", "", "正文", "2026-07-27T00:00:50Z"); err != nil || found {
		t.Fatalf("update missing reply: found=%v err=%v", found, err)
	}

	resolved, found, err := repository.Resolve(ctx, "doc-atlas-quick-start", "comment-test", "", "2026-07-27T00:01:00Z")
	if err != nil {
		t.Fatalf("resolve comment: %v", err)
	}
	if !found || resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("comment was not resolved: found=%v comment=%#v", found, resolved)
	}

	if _, found, err := repository.Resolve(ctx, "doc-atlas-quick-start", "missing", "", "2026-07-27T00:02:00Z"); err != nil || found {
		t.Fatal("missing comment resolved unexpectedly")
	}
	if deleted, err := repository.DeleteComment(ctx, "doc-atlas-quick-start", "comment-test", "", "2026-07-27T00:03:00Z"); err != nil || !deleted {
		t.Fatalf("delete comment: deleted=%v err=%v", deleted, err)
	}
	remaining, err := repository.List(ctx, "doc-atlas-quick-start")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("comment thread was not removed: err=%v comments=%#v", err, remaining)
	}

	if empty, err := repository.List(ctx, "doc-missing"); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v, want non-nil empty slice", empty)
	}
}
