package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCommentEventHubPublishesAndUnsubscribes(t *testing.T) {
	hub := newCommentEventHub()
	events, unsubscribe := hub.Subscribe("doc-test")
	event := commentEvent{Type: "reply.created", DocumentID: "doc-test", CommentID: "comment-test"}
	hub.Publish(event)

	select {
	case received := <-events:
		if received.Type != event.Type || received.CommentID != event.CommentID {
			t.Fatalf("unexpected event: %#v", received)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for comment event")
	}

	unsubscribe()
	unsubscribe()
	if len(hub.subscribers) != 0 {
		t.Fatalf("subscribers were not removed: %#v", hub.subscribers)
	}
}

func TestDocumentCommentEventsStreamHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments/events",
		nil,
	).WithContext(ctx)
	request.SetPathValue("slug", "atlas-agent")
	request.SetPathValue("documentSlug", "quick-start")
	response := httptest.NewRecorder()

	documentCommentEventsHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/event-stream; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if buffering := response.Header().Get("X-Accel-Buffering"); buffering != "no" {
		t.Fatalf("X-Accel-Buffering = %q", buffering)
	}
	if body := response.Body.String(); body != ": connected\n\n" {
		t.Fatalf("unexpected stream prelude: %q", body)
	}
}
