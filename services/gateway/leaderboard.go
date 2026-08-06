package main

import (
	"net/http"
	"strconv"
	"strings"
)

// 公开贡献者排行榜：按经验降序，排除被封禁用户。匿名可访问。

const (
	leaderboardDefaultLimit = 20
	leaderboardMaxLimit     = 50
)

type leaderboardResponse struct {
	Data      []leaderboardUser `json:"data"`
	RequestID string            `json:"request_id"`
}

func leaderboardHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	limit := leaderboardDefaultLimit
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > leaderboardMaxLimit {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_query",
				"limit 必须是 1 到 50 的整数")
			return
		}
		limit = parsed
	}
	var users []leaderboardUser
	if authRepositoryStore != nil {
		var err error
		users, err = authRepositoryStore.Leaderboard(request.Context(), limit)
		if err != nil {
			writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "排行榜暂时不可用")
			return
		}
	}
	if users == nil {
		users = []leaderboardUser{}
	}
	writeJSON(writer, http.StatusOK, leaderboardResponse{
		Data: users, RequestID: requestIDFromContext(request.Context()),
	})
}
