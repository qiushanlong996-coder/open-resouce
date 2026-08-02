package main

import (
	"context"
	"log/slog"
	"time"
)

// 用户等级系统。
//
// 累计经验决定 1..6 级，管理员（ADMIN_EMAILS 白名单）恒为最高级。
// 经验写入是 best-effort：失败只记日志，绝不阻塞评论、收藏、发布等主动作
//（与 notifications.go、search.go 的降级策略一致）。
// 幂等由账本表唯一键 (user_id, action, source_key) 保证：收藏→取消→再收藏、
// 重复分享都只加一次经验。

const maxUserLevel = 6

// levelThresholds[i] 是达到第 (i+1) 级所需的累计经验（温和曲线）。
var levelThresholds = [maxUserLevel]int{0, 100, 300, 700, 1500, 3000}

// 各动作经验值。
const (
	xpPost     = 50 // 项目通过审核（发帖）
	xpComment  = 10 // 发表评论
	xpReply    = 5  // 回复评论
	xpShare    = 3  // 分享项目（每项目每人一次）
	xpFavorite = 2  // 收藏项目（每项目每人一次）
	xpLike     = 1  // 点赞评论（每条评论每人一次）
)

// 经验动作标识，同时作为账本 action 列的取值。
const (
	xpActionPost     = "post"
	xpActionComment  = "comment"
	xpActionReply    = "reply"
	xpActionShare    = "share"
	xpActionFavorite = "favorite"
	xpActionLike     = "like"
)

// levelForExperience 把累计经验映射到 1..6 级。
func levelForExperience(experience int) int {
	level := 1
	for index := 1; index < maxUserLevel; index++ {
		if experience >= levelThresholds[index] {
			level = index + 1
		}
	}
	return level
}

// levelForUser 计算展示等级：管理员恒为最高级，其余由经验决定。
func levelForUser(email string, experience int) int {
	if isAdminEmail(email) {
		return maxUserLevel
	}
	return levelForExperience(experience)
}

// awardExperienceBestEffort 异步给用户加经验。匿名（userID 为空）或未接仓库时跳过。
func awardExperienceBestEffort(userID, action, sourceKey string, points int) {
	if userID == "" || authRepositoryStore == nil {
		return
	}
	runBestEffort(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := authRepositoryStore.AddExperience(ctx, userID, action, sourceKey, points); err != nil {
			slog.Warn("award experience failed",
				"user_id", userID, "action", action, "error", err)
		}
	})
}

// enrichCommentLevels 为评论及其回复批量补作者等级，避免逐条查询（N+1）。
// 查询失败时保持原样返回，不阻塞评论展示。
func enrichCommentLevels(ctx context.Context, comments []documentComment) []documentComment {
	if authRepositoryStore == nil || len(comments) == 0 {
		return comments
	}
	ids := make([]string, 0, len(comments))
	seen := make(map[string]struct{})
	collect := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, comment := range comments {
		collect(comment.AuthorID)
		for _, reply := range comment.Replies {
			collect(reply.AuthorID)
		}
	}
	levels, err := authRepositoryStore.LevelsByUserIDs(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "load comment author levels failed", "error", err)
		return comments
	}
	// 头像框在同一次补全里批量加载；失败只记日志，不阻塞评论展示（保持等级与内容可见）。
	frames, err := authRepositoryStore.FramesByUserIDs(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "load comment author frames failed", "error", err)
		frames = map[string]string{}
	}
	for index := range comments {
		comments[index].AuthorLevel = levels[comments[index].AuthorID]
		comments[index].AuthorFrame = frames[comments[index].AuthorID]
		for replyIndex := range comments[index].Replies {
			comments[index].Replies[replyIndex].AuthorLevel = levels[comments[index].Replies[replyIndex].AuthorID]
			comments[index].Replies[replyIndex].AuthorFrame = frames[comments[index].Replies[replyIndex].AuthorID]
		}
	}
	return comments
}
