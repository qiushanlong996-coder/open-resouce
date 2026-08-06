package main

import (
	"net/http"
	"sort"
	"strings"
)

// 热门标签：从已发布项目聚合标签并按出现次数排序，供首页标签云与筛选使用。

type tagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type tagListResponse struct {
	Data      []tagCount `json:"data"`
	RequestID string     `json:"request_id"`
}

const maxHotTags = 30

func hotTagsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projects, err := managedProjectRepositoryStore.ListPublished(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "标签聚合暂时不可用")
		return
	}
	counts := make(map[string]int)
	for _, project := range projects {
		for _, raw := range project.Tags {
			name := strings.TrimSpace(raw)
			if name != "" {
				counts[name]++
			}
		}
	}
	tags := make([]tagCount, 0, len(counts))
	for name, count := range counts {
		tags = append(tags, tagCount{Name: name, Count: count})
	}
	sort.Slice(tags, func(left, right int) bool {
		if tags[left].Count == tags[right].Count {
			return tags[left].Name < tags[right].Name
		}
		return tags[left].Count > tags[right].Count
	})
	if len(tags) > maxHotTags {
		tags = tags[:maxHotTags]
	}
	writeJSON(writer, http.StatusOK, tagListResponse{
		Data: tags, RequestID: requestIDFromContext(request.Context()),
	})
}
