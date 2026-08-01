package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// 管理端用户统计。
//
// GET /api/v1/admin/user-stats?days=14 汇总注册趋势（按日）、等级分布直方图、
// 用户总数与封禁数，供管理概览的可视化图表使用。仅管理员可访问，复用
// requireAdminUser 与既有的 auth / ban 仓库。

// userRegistrationPoint 是注册趋势里的一天（UTC 日期，零填充）。
type userRegistrationPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// userLevelBucket 是等级分布直方图里的一档（1..maxUserLevel）。
type userLevelBucket struct {
	Level int `json:"level"`
	Count int `json:"count"`
}

// userStatsData 由仓库层填充，处理器再叠加封禁数后返回。
type userStatsData struct {
	TotalUsers     int                     `json:"total_users"`
	Days           int                     `json:"days"`
	Registrations  []userRegistrationPoint `json:"registrations"`
	LevelHistogram []userLevelBucket       `json:"level_histogram"`
}

const (
	userStatsDefaultDays = 14
	userStatsMaxDays     = 90
)

// adminUserStatsHandler 返回用户注册趋势与等级分布，供管理概览渲染图表。
//
//	GET /api/v1/admin/user-stats?days=14
func adminUserStatsHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	_ = admin
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	days := userStatsDefaultDays
	if raw := request.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			days = parsed
		}
	}
	if days > userStatsMaxDays {
		days = userStatsMaxDays
	}
	ctx := request.Context()
	stats, err := authRepositoryStore.UserStats(ctx, days)
	if err != nil {
		slog.ErrorContext(ctx, "load user stats failed",
			"request_id", requestIDFromContext(ctx), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "统计服务暂时不可用")
		return
	}
	// 封禁数是 best-effort：黑名单仓库抖动不应阻断整份统计。
	banned := 0
	if banRepositoryStore != nil {
		if count, err := banRepositoryStore.CountBanned(ctx); err != nil {
			slog.WarnContext(ctx, "count banned failed",
				"request_id", requestIDFromContext(ctx), "error", err)
		} else {
			banned = count
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{
			"total_users":     stats.TotalUsers,
			"banned":          banned,
			"days":            stats.Days,
			"registrations":   stats.Registrations,
			"level_histogram": stats.LevelHistogram,
		},
		"request_id": requestIDFromContext(ctx),
	})
}

// buildRegistrationBuckets 生成最近 days 天（UTC，含今天、最旧在前）的零填充
// 桶，并返回 "日期→下标" 索引供仓库层按注册日累加。
func buildRegistrationBuckets(days int) ([]userRegistrationPoint, map[string]int) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	registrations := make([]userRegistrationPoint, days)
	index := make(map[string]int, days)
	for i := 0; i < days; i++ {
		key := today.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		registrations[i] = userRegistrationPoint{Date: key, Count: 0}
		index[key] = i
	}
	return registrations, index
}

// levelHistogramFromCounts 把 level→count 映射整理成 1..maxUserLevel 的稳定序列。
func levelHistogramFromCounts(counts map[int]int) []userLevelBucket {
	histogram := make([]userLevelBucket, maxUserLevel)
	for level := 1; level <= maxUserLevel; level++ {
		histogram[level-1] = userLevelBucket{Level: level, Count: counts[level]}
	}
	return histogram
}
