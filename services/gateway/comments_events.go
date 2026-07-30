package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type commentEvent struct {
	Type       string `json:"type"`
	DocumentID string `json:"document_id"`
	CommentID  string `json:"comment_id"`
	ReplyID    string `json:"reply_id,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type commentEventHub struct {
	sync.RWMutex
	subscribers map[string]map[chan commentEvent]struct{}
}

func newCommentEventHub() *commentEventHub {
	return &commentEventHub{subscribers: make(map[string]map[chan commentEvent]struct{})}
}

func (hub *commentEventHub) Subscribe(documentID string) (<-chan commentEvent, func()) {
	channel := make(chan commentEvent, 16)
	hub.Lock()
	if hub.subscribers[documentID] == nil {
		hub.subscribers[documentID] = make(map[chan commentEvent]struct{})
	}
	hub.subscribers[documentID][channel] = struct{}{}
	hub.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			hub.Lock()
			delete(hub.subscribers[documentID], channel)
			if len(hub.subscribers[documentID]) == 0 {
				delete(hub.subscribers, documentID)
			}
			hub.Unlock()
			close(channel)
		})
	}
	return channel, cancel
}

func (hub *commentEventHub) Publish(event commentEvent) {
	hub.RLock()
	defer hub.RUnlock()
	for channel := range hub.subscribers[event.DocumentID] {
		select {
		case channel <- event:
		default:
		}
	}
}

var commentEvents = newCommentEventHub()

func publishCommentEvent(eventType, documentID, commentID, replyID string) {
	commentEvents.Publish(commentEvent{
		Type:       eventType,
		DocumentID: documentID,
		CommentID:  commentID,
		ReplyID:    replyID,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func documentCommentEventsHandler(writer http.ResponseWriter, request *http.Request) {
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeAPIError(writer, request, http.StatusInternalServerError, "streaming_unsupported", "当前连接不支持实时事件")
		return
	}
	events, unsubscribe := commentEvents.Subscribe(document.ID)
	defer unsubscribe()

	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(writer, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-request.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(writer, "event: comment\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
