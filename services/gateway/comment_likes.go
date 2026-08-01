package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// 文档评论点赞。
//
// 与收藏类似：以 (comment_id, user_id) 联合主键保证幂等，重复点赞/取消都安全。
// 点赞成功（首次）给点赞者加经验（xpActionLike）。评论列表按当前查看者补上
// like_count 与 liked，避免逐条查询。

type commentLikeRepository interface {
	// SetLike 设置点赞状态，返回是否发生变化（首次点赞或首次取消为 true）。
	SetLike(ctx context.Context, commentID, userID string, like bool) (bool, error)
	CountsByComments(ctx context.Context, commentIDs []string) (map[string]int, error)
	LikedByUser(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error)
}

type memoryCommentLikeRepository struct {
	sync.RWMutex
	byComment map[string]map[string]struct{}
}

func newMemoryCommentLikeRepository() *memoryCommentLikeRepository {
	return &memoryCommentLikeRepository{byComment: make(map[string]map[string]struct{})}
}

func (repository *memoryCommentLikeRepository) SetLike(
	_ context.Context, commentID, userID string, like bool,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	users := repository.byComment[commentID]
	_, liked := users[userID]
	if like {
		if liked {
			return false, nil
		}
		if users == nil {
			users = make(map[string]struct{})
			repository.byComment[commentID] = users
		}
		users[userID] = struct{}{}
		return true, nil
	}
	if !liked {
		return false, nil
	}
	delete(users, userID)
	return true, nil
}

func (repository *memoryCommentLikeRepository) CountsByComments(
	_ context.Context, commentIDs []string,
) (map[string]int, error) {
	repository.RLock()
	defer repository.RUnlock()
	counts := make(map[string]int, len(commentIDs))
	for _, id := range commentIDs {
		if users := repository.byComment[id]; len(users) > 0 {
			counts[id] = len(users)
		}
	}
	return counts, nil
}

func (repository *memoryCommentLikeRepository) LikedByUser(
	_ context.Context, userID string, commentIDs []string,
) (map[string]bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	liked := make(map[string]bool, len(commentIDs))
	for _, id := range commentIDs {
		if _, ok := repository.byComment[id][userID]; ok {
			liked[id] = true
		}
	}
	return liked, nil
}

type mysqlCommentLikeRepository struct{ db *sql.DB }

func newMySQLCommentLikeRepository(db *sql.DB) *mysqlCommentLikeRepository {
	return &mysqlCommentLikeRepository{db: db}
}

func (repository *mysqlCommentLikeRepository) SetLike(
	ctx context.Context, commentID, userID string, like bool,
) (bool, error) {
	if like {
		// INSERT IGNORE：已点赞则 0 行，天然幂等。
		result, err := repository.db.ExecContext(ctx,
			`INSERT IGNORE INTO comment_likes (comment_id, user_id) VALUES (?, ?)`,
			commentID, userID)
		if err != nil {
			return false, fmt.Errorf("insert comment like: %w", err)
		}
		affected, _ := result.RowsAffected()
		return affected > 0, nil
	}
	result, err := repository.db.ExecContext(ctx,
		`DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?`, commentID, userID)
	if err != nil {
		return false, fmt.Errorf("delete comment like: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlCommentLikeRepository) CountsByComments(
	ctx context.Context, commentIDs []string,
) (map[string]int, error) {
	counts := make(map[string]int)
	if len(commentIDs) == 0 {
		return counts, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(commentIDs)), ",")
	arguments := make([]any, len(commentIDs))
	for index, id := range commentIDs {
		arguments[index] = id
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT comment_id, COUNT(*) FROM comment_likes WHERE comment_id IN (`+placeholders+`)
		 GROUP BY comment_id`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("count comment likes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan comment like count: %w", err)
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (repository *mysqlCommentLikeRepository) LikedByUser(
	ctx context.Context, userID string, commentIDs []string,
) (map[string]bool, error) {
	liked := make(map[string]bool)
	if userID == "" || len(commentIDs) == 0 {
		return liked, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(commentIDs)), ",")
	arguments := make([]any, 0, len(commentIDs)+1)
	for _, id := range commentIDs {
		arguments = append(arguments, id)
	}
	arguments = append(arguments, userID)
	rows, err := repository.db.QueryContext(ctx,
		`SELECT comment_id FROM comment_likes WHERE comment_id IN (`+placeholders+`) AND user_id = ?`,
		arguments...)
	if err != nil {
		return nil, fmt.Errorf("query liked comments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan liked comment: %w", err)
		}
		liked[id] = true
	}
	return liked, rows.Err()
}

var commentLikeRepositoryStore commentLikeRepository = newMemoryCommentLikeRepository()

var _ commentLikeRepository = (*memoryCommentLikeRepository)(nil)
var _ commentLikeRepository = (*mysqlCommentLikeRepository)(nil)

// documentCommentLikeHandler 处理评论点赞与取消。
//
//	POST   .../comments/{commentID}/like   点赞（幂等）
//	DELETE .../comments/{commentID}/like   取消点赞（幂等）
func documentCommentLikeHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost && request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	document, ok := findDocument(request, writer)
	if !ok {
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	commentID := request.PathValue("commentID")
	// 校验评论确实属于当前文档，避免对任意 ID 点赞。
	comments, err := commentRepositoryStore.List(request.Context(), document.ID)
	if err != nil {
		writeRepositoryError(writer, request, err)
		return
	}
	if !documentContainsComment(comments, commentID) {
		writeAPIError(writer, request, http.StatusNotFound, "comment_not_found", "评论不存在")
		return
	}

	changed, err := commentLikeRepositoryStore.SetLike(
		request.Context(), commentID, user.ID, request.Method == http.MethodPost)
	if err != nil {
		slog.ErrorContext(request.Context(), "set comment like failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "点赞服务暂时不可用")
		return
	}
	if changed && request.Method == http.MethodPost {
		awardExperienceBestEffort(user.ID, xpActionLike, commentID, xpLike)
	}
	writer.WriteHeader(http.StatusNoContent)
}

// documentContainsComment 判断某评论 ID 是否属于评论树（根或回复）。
func documentContainsComment(comments []documentComment, commentID string) bool {
	for _, comment := range comments {
		if comment.ID == commentID {
			return true
		}
		for _, reply := range comment.Replies {
			if reply.ID == commentID {
				return true
			}
		}
	}
	return false
}

// enrichCommentLikes 为评论及其回复补 like_count 与当前查看者的 liked 状态。
// viewerID 为空（匿名）时只补数量，不查 liked。失败时保持原样，不阻塞展示。
func enrichCommentLikes(ctx context.Context, comments []documentComment, viewerID string) []documentComment {
	if commentLikeRepositoryStore == nil || len(comments) == 0 {
		return comments
	}
	ids := make([]string, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.ID)
		for _, reply := range comment.Replies {
			ids = append(ids, reply.ID)
		}
	}
	counts, err := commentLikeRepositoryStore.CountsByComments(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "load comment like counts failed", "error", err)
		return comments
	}
	liked := map[string]bool{}
	if viewerID != "" {
		if result, err := commentLikeRepositoryStore.LikedByUser(ctx, viewerID, ids); err != nil {
			slog.WarnContext(ctx, "load viewer comment likes failed", "error", err)
		} else {
			liked = result
		}
	}
	for index := range comments {
		comments[index].LikeCount = counts[comments[index].ID]
		comments[index].Liked = liked[comments[index].ID]
		for replyIndex := range comments[index].Replies {
			replyID := comments[index].Replies[replyIndex].ID
			comments[index].Replies[replyIndex].LikeCount = counts[replyID]
			comments[index].Replies[replyIndex].Liked = liked[replyID]
		}
	}
	return comments
}
