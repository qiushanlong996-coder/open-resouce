package main

import (
	"context"
	"net/http"
)

// projectShareHandler 记录一次项目分享并给分享者加经验（每项目每人一次，幂等）。
//
//	POST /api/v1/projects/{slug}/share
//
// 前端分享按钮在复制链接的同时调用本接口。分享无需项目已发布——种子项目与
// 已发布项目都可分享；未知 slug 返回 404。
func projectShareHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	projectID, found, err := resolveProjectID(request.Context(), request.PathValue("slug"))
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	awardExperienceBestEffort(user.ID, xpActionShare, projectID, xpShare)
	writer.WriteHeader(http.StatusNoContent)
}

// resolveProjectID 解析项目 slug 到内部 ID，覆盖种子项目与已发布项目。
func resolveProjectID(ctx context.Context, slug string) (string, bool, error) {
	if project, ok := seedProjectDetails[slug]; ok {
		return project.ID, true, nil
	}
	managed, found, err := managedProjectRepositoryStore.FindPublishedBySlug(ctx, slug)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}
	return managed.ID, true, nil
}
