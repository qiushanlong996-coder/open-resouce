package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// 内容举报。
//
// 登录用户可举报项目或评论，选择原因并可附带说明。举报进入 open 队列，
// 管理员在控制台处理（resolved 已处理 / dismissed 已驳回）。处理动作写入
// admin_audit，与封禁、下架等管理操作一致。
//
// 目标标识策略：项目按 slug 解析成稳定的内部 ID 后存储（未知 slug 原样保留，
// 视作已经是 ID）；评论直接存储评论 ID。

const (
	reportTargetProject = "project"
	reportTargetComment = "comment"

	reportStatusOpen      = "open"
	reportStatusResolved  = "resolved"
	reportStatusDismissed = "dismissed"

	reportReasonMaxRunes = 64
	reportDetailMaxRunes = 1000
	reportTargetIDMax    = 191
	reportListDefault    = 100
	reportListMax        = 500
)

type contentReport struct {
	ID            string     `json:"id"`
	ReporterID    string     `json:"reporter_id"`
	ReporterEmail string     `json:"reporter_email,omitempty"`
	ReporterName  string     `json:"reporter_name,omitempty"`
	TargetType    string     `json:"target_type"`
	TargetID      string     `json:"target_id"`
	Reason        string     `json:"reason"`
	Detail        string     `json:"detail"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ResolverID    *string    `json:"resolver_id,omitempty"`
}

type contentReportRepository interface {
	Create(ctx context.Context, report contentReport) error
	// List 按创建时间倒序返回举报；status 非空时仅返回该状态。
	List(ctx context.Context, status string, limit int) ([]contentReport, error)
	// UpdateStatus 更新举报状态并记录处理人与处理时间；举报不存在返回 false。
	UpdateStatus(ctx context.Context, id, status, resolverID string, resolvedAt time.Time) (bool, error)
	// HasOpenReport 判断某用户是否已对同一目标存在未处理举报，用于去重。
	HasOpenReport(ctx context.Context, reporterID, targetType, targetID string) (bool, error)
}

type memoryContentReportRepository struct {
	sync.RWMutex
	reports []contentReport
}

func newMemoryContentReportRepository() *memoryContentReportRepository {
	return &memoryContentReportRepository{reports: make([]contentReport, 0)}
}

func (repository *memoryContentReportRepository) Create(_ context.Context, report contentReport) error {
	repository.Lock()
	defer repository.Unlock()
	repository.reports = append(repository.reports, report)
	return nil
}

func (repository *memoryContentReportRepository) List(
	_ context.Context, status string, limit int,
) ([]contentReport, error) {
	repository.RLock()
	defer repository.RUnlock()
	filtered := make([]contentReport, 0, len(repository.reports))
	for _, report := range repository.reports {
		if status != "" && report.Status != status {
			continue
		}
		filtered = append(filtered, report)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if limit <= 0 || limit > len(filtered) {
		limit = len(filtered)
	}
	return filtered[:limit], nil
}

func (repository *memoryContentReportRepository) UpdateStatus(
	_ context.Context, id, status, resolverID string, resolvedAt time.Time,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for index := range repository.reports {
		if repository.reports[index].ID != id {
			continue
		}
		resolver := resolverID
		resolvedTime := resolvedAt
		repository.reports[index].Status = status
		repository.reports[index].ResolverID = &resolver
		repository.reports[index].ResolvedAt = &resolvedTime
		return true, nil
	}
	return false, nil
}

func (repository *memoryContentReportRepository) HasOpenReport(
	_ context.Context, reporterID, targetType, targetID string,
) (bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	for _, report := range repository.reports {
		if report.Status == reportStatusOpen && report.ReporterID == reporterID &&
			report.TargetType == targetType && report.TargetID == targetID {
			return true, nil
		}
	}
	return false, nil
}

type mysqlContentReportRepository struct{ db *sql.DB }

func newMySQLContentReportRepository(db *sql.DB) *mysqlContentReportRepository {
	return &mysqlContentReportRepository{db: db}
}

func (repository *mysqlContentReportRepository) Create(ctx context.Context, report contentReport) error {
	_, err := repository.db.ExecContext(ctx,
		`INSERT INTO content_reports (id, reporter_id, target_type, target_id, reason, detail, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.ReporterID, report.TargetType, report.TargetID,
		report.Reason, report.Detail, report.Status, report.CreatedAt)
	if err != nil {
		return fmt.Errorf("create content report: %w", err)
	}
	return nil
}

func (repository *mysqlContentReportRepository) List(
	ctx context.Context, status string, limit int,
) ([]contentReport, error) {
	if limit <= 0 {
		limit = reportListDefault
	}
	query := `SELECT id, reporter_id, target_type, target_id, reason, detail, status,
		 created_at, resolved_at, resolver_id
		 FROM content_reports`
	arguments := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		arguments = append(arguments, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	arguments = append(arguments, limit)
	rows, err := repository.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list content reports: %w", err)
	}
	defer rows.Close()
	result := make([]contentReport, 0)
	for rows.Next() {
		var report contentReport
		if err := rows.Scan(&report.ID, &report.ReporterID, &report.TargetType, &report.TargetID,
			&report.Reason, &report.Detail, &report.Status, &report.CreatedAt,
			&report.ResolvedAt, &report.ResolverID); err != nil {
			return nil, fmt.Errorf("scan content report: %w", err)
		}
		result = append(result, report)
	}
	return result, rows.Err()
}

func (repository *mysqlContentReportRepository) UpdateStatus(
	ctx context.Context, id, status, resolverID string, resolvedAt time.Time,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx,
		`UPDATE content_reports SET status = ?, resolver_id = ?, resolved_at = ? WHERE id = ?`,
		status, resolverID, resolvedAt, id)
	if err != nil {
		return false, fmt.Errorf("update content report: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (repository *mysqlContentReportRepository) HasOpenReport(
	ctx context.Context, reporterID, targetType, targetID string,
) (bool, error) {
	var count int
	err := repository.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_reports
		 WHERE reporter_id = ? AND target_type = ? AND target_id = ? AND status = ?`,
		reporterID, targetType, targetID, reportStatusOpen).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count open content reports: %w", err)
	}
	return count > 0, nil
}

var contentReportRepositoryStore contentReportRepository = newMemoryContentReportRepository()

var _ contentReportRepository = (*memoryContentReportRepository)(nil)
var _ contentReportRepository = (*mysqlContentReportRepository)(nil)

// reportsHandler 让登录用户提交内容举报。
//
//	POST /api/v1/reports  body: {target_type, target_id, reason, detail?}
func reportsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if !ensureNotBanned(writer, request, user.ID) {
		return
	}
	var input struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Detail     string `json:"detail"`
	}
	if decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "举报数据格式不正确")
		return
	}
	targetType := strings.TrimSpace(input.TargetType)
	if targetType != reportTargetProject && targetType != reportTargetComment {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_target_type", "举报对象类型不支持")
		return
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || len([]rune(reason)) > reportReasonMaxRunes {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_reason",
			fmt.Sprintf("请填写举报原因，且不超过 %d 字", reportReasonMaxRunes))
		return
	}
	detail := strings.TrimSpace(input.Detail)
	if len([]rune(detail)) > reportDetailMaxRunes {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_detail",
			fmt.Sprintf("补充说明不超过 %d 字", reportDetailMaxRunes))
		return
	}
	rawTarget := strings.TrimSpace(input.TargetID)
	if rawTarget == "" {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_target", "缺少举报对象")
		return
	}
	targetID, err := resolveReportTargetID(request.Context(), targetType, rawTarget)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "举报服务暂时不可用")
		return
	}
	if len([]rune(targetID)) > reportTargetIDMax {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_target", "举报对象标识过长")
		return
	}

	// 去重：同一用户对同一目标已有未处理举报时，直接幂等返回 200，避免刷量。
	duplicate, err := contentReportRepositoryStore.HasOpenReport(request.Context(), user.ID, targetType, targetID)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "举报服务暂时不可用")
		return
	}
	if duplicate {
		writeJSON(writer, http.StatusOK, map[string]any{
			"data":       map[string]any{"status": reportStatusOpen, "duplicate": true},
			"request_id": requestIDFromContext(request.Context()),
		})
		return
	}

	report := contentReport{
		ID: "report-" + newRequestID(), ReporterID: user.ID,
		TargetType: targetType, TargetID: targetID, Reason: reason, Detail: detail,
		Status: reportStatusOpen, CreatedAt: time.Now().UTC(),
	}
	if err := contentReportRepositoryStore.Create(request.Context(), report); err != nil {
		slog.ErrorContext(request.Context(), "create content report failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "举报服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"data": report, "request_id": requestIDFromContext(request.Context()),
	})
}

// resolveReportTargetID 把举报目标标准化为稳定标识。项目优先按 slug 解析成内部
// ID；无法解析时视作调用方已经传入 ID，原样保留。评论直接使用其 ID。
func resolveReportTargetID(ctx context.Context, targetType, raw string) (string, error) {
	if targetType != reportTargetProject {
		return raw, nil
	}
	projectID, found, err := resolveProjectID(ctx, raw)
	if err != nil {
		return "", err
	}
	if found {
		return projectID, nil
	}
	return raw, nil
}

// adminReportsHandler 列出内容举报，默认仅未处理（open）。
//
//	GET /api/v1/admin/reports?status=open
func adminReportsHandler(writer http.ResponseWriter, request *http.Request) {
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
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	switch status {
	case "", reportStatusOpen, reportStatusResolved, reportStatusDismissed:
	default:
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_status", "举报状态筛选无效")
		return
	}
	reports, err := contentReportRepositoryStore.List(request.Context(), status, reportListMax)
	if err != nil {
		slog.ErrorContext(request.Context(), "list content reports failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "举报服务暂时不可用")
		return
	}
	enrichReportReporters(request.Context(), reports)
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": reports, "request_id": requestIDFromContext(request.Context()),
	})
}

// enrichReportReporters 用举报人邮箱/昵称补全列表，best-effort：查不到只留 ID。
func enrichReportReporters(ctx context.Context, reports []contentReport) {
	if authRepositoryStore == nil || len(reports) == 0 {
		return
	}
	ids := make([]string, 0, len(reports))
	seen := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		if _, ok := seen[report.ReporterID]; ok {
			continue
		}
		seen[report.ReporterID] = struct{}{}
		ids = append(ids, report.ReporterID)
	}
	users, err := authRepositoryStore.UsersByIDs(ctx, ids)
	if err != nil {
		return
	}
	for index := range reports {
		if user, ok := users[reports[index].ReporterID]; ok {
			reports[index].ReporterEmail = user.Email
			reports[index].ReporterName = user.DisplayName
		}
	}
}

// adminReportActionHandler 处理（resolve）或驳回（dismiss）一条举报。
//
//	POST /api/v1/admin/reports/{reportID}/resolve
//	POST /api/v1/admin/reports/{reportID}/dismiss
func adminReportActionHandler(writer http.ResponseWriter, request *http.Request) {
	admin, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	reportID := request.PathValue("reportID")
	action := request.PathValue("action")
	var status, auditAction string
	switch action {
	case "resolve":
		status, auditAction = reportStatusResolved, "report_resolved"
	case "dismiss":
		status, auditAction = reportStatusDismissed, "report_dismissed"
	default:
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "请求的接口不存在")
		return
	}
	updated, err := contentReportRepositoryStore.UpdateStatus(
		request.Context(), reportID, status, admin.ID, time.Now().UTC())
	if err != nil {
		slog.ErrorContext(request.Context(), "update content report failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "举报服务暂时不可用")
		return
	}
	if !updated {
		writeAPIError(writer, request, http.StatusNotFound, "report_not_found", "举报不存在")
		return
	}
	recordAdminAudit(request, admin, auditAction, reportID, status)
	writeJSON(writer, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": reportID, "status": status},
		"request_id": requestIDFromContext(request.Context()),
	})
}
