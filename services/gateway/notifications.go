package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxNotificationPageSize = 50

type notification struct {
	ID           string     `json:"id"`
	RecipientID  string     `json:"-"`
	ActorID      string     `json:"actor_id,omitempty"`
	ActorName    string     `json:"actor_name,omitempty"`
	Type         string     `json:"type"`
	Title        string     `json:"title"`
	Body         string     `json:"body,omitempty"`
	ProjectSlug  string     `json:"project_slug,omitempty"`
	DocumentSlug string     `json:"document_slug,omitempty"`
	CommentID    string     `json:"comment_id,omitempty"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type notificationListResponse struct {
	Data        []notification `json:"data"`
	UnreadCount int            `json:"unread_count"`
	RequestID   string         `json:"request_id"`
}

type notificationRepository interface {
	Create(context.Context, notification) error
	ListByRecipient(ctx context.Context, recipientID string, limit int) ([]notification, error)
	UnreadCount(ctx context.Context, recipientID string) (int, error)
	MarkRead(ctx context.Context, recipientID, notificationID string, readAt time.Time) (bool, error)
	MarkAllRead(ctx context.Context, recipientID string, readAt time.Time) error
}

type memoryNotificationRepository struct {
	sync.RWMutex
	byRecipient map[string][]notification
}

func newMemoryNotificationRepository() *memoryNotificationRepository {
	return &memoryNotificationRepository{byRecipient: make(map[string][]notification)}
}

func (repository *memoryNotificationRepository) Create(_ context.Context, entry notification) error {
	repository.Lock()
	defer repository.Unlock()
	repository.byRecipient[entry.RecipientID] = append(repository.byRecipient[entry.RecipientID], entry)
	return nil
}

func (repository *memoryNotificationRepository) ListByRecipient(
	_ context.Context, recipientID string, limit int,
) ([]notification, error) {
	repository.RLock()
	defer repository.RUnlock()
	entries := append([]notification(nil), repository.byRecipient[recipientID]...)
	sort.SliceStable(entries, func(left, right int) bool {
		return entries[left].CreatedAt.After(entries[right].CreatedAt)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	if entries == nil {
		return []notification{}, nil
	}
	return entries, nil
}

func (repository *memoryNotificationRepository) UnreadCount(
	_ context.Context, recipientID string,
) (int, error) {
	repository.RLock()
	defer repository.RUnlock()
	count := 0
	for _, entry := range repository.byRecipient[recipientID] {
		if entry.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (repository *memoryNotificationRepository) MarkRead(
	_ context.Context, recipientID, notificationID string, readAt time.Time,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byRecipient[recipientID] {
		entry := &repository.byRecipient[recipientID][index]
		if entry.ID != notificationID {
			continue
		}
		if entry.ReadAt == nil {
			entry.ReadAt = &readAt
		}
		return true, nil
	}
	return false, nil
}

func (repository *memoryNotificationRepository) MarkAllRead(
	_ context.Context, recipientID string, readAt time.Time,
) error {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.byRecipient[recipientID] {
		entry := &repository.byRecipient[recipientID][index]
		if entry.ReadAt == nil {
			entry.ReadAt = &readAt
		}
	}
	return nil
}

type mysqlNotificationRepository struct{ db *sql.DB }

func newMySQLNotificationRepository(db *sql.DB) *mysqlNotificationRepository {
	return &mysqlNotificationRepository{db: db}
}

func (repository *mysqlNotificationRepository) Create(ctx context.Context, entry notification) error {
	_, err := repository.db.ExecContext(ctx, `INSERT INTO notifications
		(id, recipient_id, actor_id, actor_name, type, title, body,
		 project_slug, document_slug, comment_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.RecipientID, entry.ActorID, entry.ActorName, entry.Type,
		entry.Title, entry.Body, entry.ProjectSlug, entry.DocumentSlug,
		entry.CommentID, entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func (repository *mysqlNotificationRepository) ListByRecipient(
	ctx context.Context, recipientID string, limit int,
) ([]notification, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT
		id, recipient_id, actor_id, actor_name, type, title, body,
		project_slug, document_slug, comment_id, read_at, created_at
		FROM notifications WHERE recipient_id = ?
		ORDER BY created_at DESC LIMIT ?`, recipientID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	entries := make([]notification, 0)
	for rows.Next() {
		var entry notification
		if err := rows.Scan(&entry.ID, &entry.RecipientID, &entry.ActorID, &entry.ActorName,
			&entry.Type, &entry.Title, &entry.Body, &entry.ProjectSlug, &entry.DocumentSlug,
			&entry.CommentID, &entry.ReadAt, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (repository *mysqlNotificationRepository) UnreadCount(
	ctx context.Context, recipientID string,
) (int, error) {
	var count int
	err := repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications
		WHERE recipient_id = ? AND read_at IS NULL`, recipientID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (repository *mysqlNotificationRepository) MarkRead(
	ctx context.Context, recipientID, notificationID string, readAt time.Time,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `UPDATE notifications
		SET read_at = COALESCE(read_at, ?) WHERE id = ? AND recipient_id = ?`,
		readAt, notificationID, recipientID)
	if err != nil {
		return false, fmt.Errorf("mark notification read: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlNotificationRepository) MarkAllRead(
	ctx context.Context, recipientID string, readAt time.Time,
) error {
	_, err := repository.db.ExecContext(ctx, `UPDATE notifications
		SET read_at = ? WHERE recipient_id = ? AND read_at IS NULL`, readAt, recipientID)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

var notificationRepositoryStore notificationRepository = newMemoryNotificationRepository()

var _ notificationRepository = (*memoryNotificationRepository)(nil)
var _ notificationRepository = (*mysqlNotificationRepository)(nil)

type notificationEventHub struct {
	sync.RWMutex
	subscribers map[string]map[chan notification]struct{}
}

func newNotificationEventHub() *notificationEventHub {
	return &notificationEventHub{subscribers: make(map[string]map[chan notification]struct{})}
}

func (hub *notificationEventHub) Subscribe(recipientID string) (<-chan notification, func()) {
	channel := make(chan notification, 16)
	hub.Lock()
	if hub.subscribers[recipientID] == nil {
		hub.subscribers[recipientID] = make(map[chan notification]struct{})
	}
	hub.subscribers[recipientID][channel] = struct{}{}
	hub.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			hub.Lock()
			delete(hub.subscribers[recipientID], channel)
			if len(hub.subscribers[recipientID]) == 0 {
				delete(hub.subscribers, recipientID)
			}
			hub.Unlock()
			close(channel)
		})
	}
	return channel, cancel
}

func (hub *notificationEventHub) Publish(entry notification) {
	hub.RLock()
	defer hub.RUnlock()
	for channel := range hub.subscribers[entry.RecipientID] {
		select {
		case channel <- entry:
		default:
		}
	}
}

var notificationEvents = newNotificationEventHub()

// dispatchNotification 持久化站内通知并实时推送给在线接收者。
// 接收者为空或与操作者相同的通知会被跳过；发送失败只记录日志，不阻塞业务主流程。
func dispatchNotification(ctx context.Context, entry notification) {
	if entry.RecipientID == "" || entry.RecipientID == entry.ActorID {
		return
	}
	entry.ID = "notification-" + newRequestID()
	entry.CreatedAt = time.Now().UTC()
	if err := notificationRepositoryStore.Create(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "notification create failed",
			"request_id", requestIDFromContext(ctx), "error", err)
		return
	}
	notificationEvents.Publish(entry)
}

func notificationsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	entries, err := notificationRepositoryStore.ListByRecipient(request.Context(), user.ID, maxNotificationPageSize)
	if err != nil {
		writeNotificationError(writer, request, err)
		return
	}
	unread, err := notificationRepositoryStore.UnreadCount(request.Context(), user.ID)
	if err != nil {
		writeNotificationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, notificationListResponse{
		Data: entries, UnreadCount: unread, RequestID: requestIDFromContext(request.Context()),
	})
}

func notificationReadHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	found, err := notificationRepositoryStore.MarkRead(
		request.Context(), user.ID, request.PathValue("notificationID"), time.Now().UTC())
	if err != nil {
		writeNotificationError(writer, request, err)
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "notification_not_found", "通知不存在")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func notificationsReadAllHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if err := notificationRepositoryStore.MarkAllRead(request.Context(), user.ID, time.Now().UTC()); err != nil {
		writeNotificationError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func notificationEventsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeAPIError(writer, request, http.StatusInternalServerError, "streaming_unsupported", "当前连接不支持实时事件")
		return
	}
	events, unsubscribe := notificationEvents.Subscribe(user.ID)
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
		case entry, open := <-events:
			if !open {
				return
			}
			payload, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(writer, "event: notification\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func writeNotificationError(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "notification repository operation failed",
		"request_id", requestIDFromContext(request.Context()),
		"error", err,
	)
	writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "通知服务暂时不可用")
}

// notifyCommentReply 在回复创建成功后通知根评论作者。
func notifyCommentReply(ctx context.Context, projectSlug, documentSlug, documentID, parentID string, reply documentComment) {
	comments, err := commentRepositoryStore.List(ctx, documentID)
	if err != nil {
		slog.ErrorContext(ctx, "notification parent lookup failed",
			"request_id", requestIDFromContext(ctx), "error", err)
		return
	}
	for _, comment := range comments {
		if comment.ID != parentID {
			continue
		}
		body := reply.Body
		if len([]rune(body)) > 120 {
			body = string([]rune(body)[:120]) + "…"
		}
		dispatchNotification(ctx, notification{
			RecipientID: comment.AuthorID, ActorID: reply.AuthorID, ActorName: reply.Author,
			Type:  "comment.replied",
			Title: reply.Author + " 回复了你的评论",
			Body:  body, ProjectSlug: projectSlug, DocumentSlug: documentSlug, CommentID: parentID,
		})
		return
	}
}

// notifyProjectReview 在管理员审核后通知项目作者。
func notifyProjectReview(ctx context.Context, project managedProject, actor authUser, action, reason string) {
	entry := notification{
		RecipientID: project.OwnerID, ActorID: actor.ID, ActorName: actor.DisplayName,
		ProjectSlug: project.Slug,
	}
	if action == "approve" {
		entry.Type = "project.approved"
		entry.Title = "项目「" + project.Name + "」已通过审核并发布"
	} else {
		entry.Type = "project.rejected"
		entry.Title = "项目「" + project.Name + "」未通过审核"
		entry.Body = strings.TrimSpace(reason)
	}
	dispatchNotification(ctx, entry)
}
