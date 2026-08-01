package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemoryCommentLikeIdempotent(t *testing.T) {
	repository := newMemoryCommentLikeRepository()
	ctx := context.Background()

	changed, _ := repository.SetLike(ctx, "c1", "u1", true)
	if !changed {
		t.Fatal("first like should change state")
	}
	changed, _ = repository.SetLike(ctx, "c1", "u1", true)
	if changed {
		t.Fatal("repeat like must be idempotent")
	}
	// 另一用户点赞同一评论。
	if _, _ = repository.SetLike(ctx, "c1", "u2", true); false {
	}

	counts, _ := repository.CountsByComments(ctx, []string{"c1"})
	if counts["c1"] != 2 {
		t.Fatalf("like count = %d, want 2", counts["c1"])
	}
	liked, _ := repository.LikedByUser(ctx, "u1", []string{"c1"})
	if !liked["c1"] {
		t.Fatal("u1 should have liked c1")
	}

	changed, _ = repository.SetLike(ctx, "c1", "u1", false)
	if !changed {
		t.Fatal("unlike should change state")
	}
	changed, _ = repository.SetLike(ctx, "c1", "u1", false)
	if changed {
		t.Fatal("repeat unlike must be idempotent")
	}
	counts, _ = repository.CountsByComments(ctx, []string{"c1"})
	if counts["c1"] != 1 {
		t.Fatalf("like count after unlike = %d, want 1", counts["c1"])
	}
}

func TestCommentLikeEndpointAndListReflectsCount(t *testing.T) {
	originalLikes := commentLikeRepositoryStore
	commentLikeRepositoryStore = newMemoryCommentLikeRepository()
	t.Cleanup(func() { commentLikeRepositoryStore = originalLikes })

	// 点赞种子评论 comment-atlas-001（属于 atlas quick-start 文档）。
	likePath := "/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-atlas-001/like"
	likeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(likeResponse, httptest.NewRequest(http.MethodPost, likePath, nil))
	if likeResponse.Code != http.StatusNoContent {
		t.Fatalf("like status = %d: %s", likeResponse.Code, likeResponse.Body)
	}

	// 列表应反映 like_count=1。
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments", nil))
	var listed commentListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, comment := range listed.Data {
		if comment.ID == "comment-atlas-001" {
			found = true
			if comment.LikeCount != 1 {
				t.Fatalf("like_count = %d, want 1", comment.LikeCount)
			}
		}
	}
	if !found {
		t.Fatal("seed comment not found in list")
	}

	// 取消点赞后计数归零。
	unlikeResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(unlikeResponse, httptest.NewRequest(http.MethodDelete, likePath, nil))
	if unlikeResponse.Code != http.StatusNoContent {
		t.Fatalf("unlike status = %d", unlikeResponse.Code)
	}
}

func TestCommentLikeRejectsUnknownComment(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/comment-does-not-exist/like", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestMySQLCommentLikeIntegration(t *testing.T) {
	database := requireTestDatabase(t)
	ctx := context.Background()

	userID := "user-like-" + newRequestID()
	commentID := "comment-like-" + newRequestID()
	docID := "doc-like-" + newRequestID()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.com", "点赞集成用户", "integration-only"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO document_comments (id, document_id, block_id, author_name, body_text, status, created_at, updated_at)
		 VALUES (?, ?, '', '作者', '正文', 'open', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`,
		commentID, docID); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	t.Cleanup(func() {
		// comment_likes 有 FK 级联，删评论与用户即清点赞。
		if _, err := database.Exec(`DELETE FROM document_comments WHERE id = ?`, commentID); err != nil {
			t.Errorf("clean up comment: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
			t.Errorf("clean up user: %v", err)
		}
	})

	repository := newMySQLCommentLikeRepository(database)
	changed, err := repository.SetLike(ctx, commentID, userID, true)
	if err != nil || !changed {
		t.Fatalf("like: changed=%v err=%v", changed, err)
	}
	changed, err = repository.SetLike(ctx, commentID, userID, true)
	if err != nil || changed {
		t.Fatalf("duplicate like should be idempotent: changed=%v err=%v", changed, err)
	}
	counts, err := repository.CountsByComments(ctx, []string{commentID})
	if err != nil || counts[commentID] != 1 {
		t.Fatalf("counts=%v err=%v", counts, err)
	}
	liked, err := repository.LikedByUser(ctx, userID, []string{commentID})
	if err != nil || !liked[commentID] {
		t.Fatalf("liked=%v err=%v", liked, err)
	}
	if changed, err := repository.SetLike(ctx, commentID, userID, false); err != nil || !changed {
		t.Fatalf("unlike: changed=%v err=%v", changed, err)
	}
}
