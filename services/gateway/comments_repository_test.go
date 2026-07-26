package main

import "testing"

func TestMemoryCommentRepository(t *testing.T) {
	repository := newMemoryCommentRepository()

	initial := repository.List("doc-atlas-quick-start")
	if len(initial) != 1 || initial[0].ID != "comment-atlas-001" {
		t.Fatalf("unexpected seed comments: %#v", initial)
	}

	created := repository.Create(documentComment{
		ID: "comment-test", DocumentID: "doc-atlas-quick-start",
		BlockID: "block-atlas-intro", Author: "测试", Body: "仓库测试",
		Status: "open", CreatedAt: "2026-07-27T00:00:00Z",
	})
	if created.ID != "comment-test" || len(repository.List("doc-atlas-quick-start")) != 2 {
		t.Fatalf("comment was not stored: %#v", created)
	}

	resolved, found := repository.Resolve("doc-atlas-quick-start", "comment-test", "2026-07-27T00:01:00Z")
	if !found || resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("comment was not resolved: found=%v comment=%#v", found, resolved)
	}

	if _, found := repository.Resolve("doc-atlas-quick-start", "missing", "2026-07-27T00:02:00Z"); found {
		t.Fatal("missing comment resolved unexpectedly")
	}

	if empty := repository.List("doc-missing"); empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v, want non-nil empty slice", empty)
	}
}
