package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	errProjectSlugExists = errors.New("project slug exists")
	errProjectNotFound   = errors.New("managed project not found")
	errProjectImmutable  = errors.New("managed project is not editable")
	projectSlugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type managedProject struct {
	ID                string     `json:"id"`
	OwnerID           string     `json:"owner_id"`
	Slug              string     `json:"slug"`
	Name              string     `json:"name"`
	Summary           string     `json:"summary"`
	Description       string     `json:"description"`
	Category          string     `json:"category"`
	Tags              []string   `json:"tags"`
	TechStack         []string   `json:"tech_stack"`
	License           string     `json:"license"`
	RepositoryURL     string     `json:"repository_url"`
	CoverObjectKey    string     `json:"cover_object_key"`
	DocumentObjectKey string     `json:"document_object_key"`
	CodeObjectKey     string     `json:"code_object_key"`
	CurrentVersion    string     `json:"current_version"`
	Status            string     `json:"status"`
	ReviewReason      string     `json:"review_reason"`
	SubmittedAt       *time.Time `json:"submitted_at,omitempty"`
	PublishedAt       *time.Time `json:"published_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type managedProjectInput struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Summary           string   `json:"summary"`
	Description       string   `json:"description"`
	Category          string   `json:"category"`
	Tags              []string `json:"tags"`
	TechStack         []string `json:"tech_stack"`
	License           string   `json:"license"`
	RepositoryURL     string   `json:"repository_url"`
	CoverObjectKey    string   `json:"cover_object_key"`
	DocumentObjectKey string   `json:"document_object_key"`
	CodeObjectKey     string   `json:"code_object_key"`
	CurrentVersion    string   `json:"current_version"`
}

type managedProjectRepository interface {
	Create(context.Context, string, managedProjectInput) (managedProject, error)
	ListByOwner(context.Context, string) ([]managedProject, error)
	Update(context.Context, string, string, managedProjectInput) (managedProject, error)
	Submit(context.Context, string, string, time.Time) (managedProject, error)
	DeleteDraft(context.Context, string, string) error
	ListPending(context.Context) ([]managedProject, error)
	Review(context.Context, string, string, string, string, time.Time) (managedProject, error)
	ListPublished(context.Context) ([]managedProject, error)
	FindPublishedBySlug(context.Context, string) (managedProject, bool, error)
	// FindByID 不限定审核状态，供作者端编辑草稿与已发布项目使用。
	FindByID(context.Context, string) (managedProject, bool, error)
	UpdatePublishedDescription(context.Context, string, string, time.Time) (managedProject, error)
	// CountByStatus 返回各状态项目数量，供管理概览统计。
	CountByStatus(context.Context) (map[string]int, error)
	// ListAll 跨状态分页列出所有项目；status 非空时按该状态过滤，返回匹配总数。
	ListAll(ctx context.Context, status string, limit, offset int) ([]managedProject, int, error)
	// Takedown 将非下架项目置为 archived（下架），返回更新后的项目。
	Takedown(ctx context.Context, projectID string, now time.Time) (managedProject, error)
}

// projectDownedStatus 是下架项目的状态：移出公开目录与搜索索引。
const projectDownedStatus = "archived"

type memoryManagedProjectRepository struct {
	sync.RWMutex
	projects map[string]managedProject
}

func newMemoryManagedProjectRepository() *memoryManagedProjectRepository {
	return &memoryManagedProjectRepository{projects: make(map[string]managedProject)}
}

func (repository *memoryManagedProjectRepository) Create(
	_ context.Context, ownerID string, input managedProjectInput,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	for _, project := range repository.projects {
		if project.Slug == input.Slug {
			return managedProject{}, errProjectSlugExists
		}
	}
	now := time.Now().UTC()
	project := projectFromInput(input)
	project.ID, project.OwnerID, project.Status = "project-"+newRequestID(), ownerID, "draft"
	project.CreatedAt, project.UpdatedAt = now, now
	repository.projects[project.ID] = project
	return project, nil
}

func (repository *memoryManagedProjectRepository) ListByOwner(
	_ context.Context, ownerID string,
) ([]managedProject, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]managedProject, 0)
	for _, project := range repository.projects {
		if project.OwnerID == ownerID {
			result = append(result, project)
		}
	}
	return result, nil
}

func (repository *memoryManagedProjectRepository) Update(
	_ context.Context, ownerID, projectID string, input managedProjectInput,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found || project.OwnerID != ownerID {
		return managedProject{}, errProjectNotFound
	}
	if project.Status != "draft" && project.Status != "rejected" {
		return managedProject{}, errProjectImmutable
	}
	for id, existing := range repository.projects {
		if id != projectID && existing.Slug == input.Slug {
			return managedProject{}, errProjectSlugExists
		}
	}
	updated := projectFromInput(input)
	updated.ID, updated.OwnerID, updated.Status = project.ID, project.OwnerID, "draft"
	updated.CreatedAt, updated.UpdatedAt = project.CreatedAt, time.Now().UTC()
	repository.projects[projectID] = updated
	return updated, nil
}

func (repository *memoryManagedProjectRepository) Submit(
	_ context.Context, ownerID, projectID string, now time.Time,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found || project.OwnerID != ownerID {
		return managedProject{}, errProjectNotFound
	}
	if project.Status != "draft" && project.Status != "rejected" {
		return managedProject{}, errProjectImmutable
	}
	project.Status, project.ReviewReason, project.SubmittedAt, project.UpdatedAt = "pending_review", "", &now, now
	repository.projects[projectID] = project
	return project, nil
}

func (repository *memoryManagedProjectRepository) DeleteDraft(
	_ context.Context, ownerID, projectID string,
) error {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found || project.OwnerID != ownerID {
		return errProjectNotFound
	}
	if project.Status != "draft" && project.Status != "rejected" {
		return errProjectImmutable
	}
	delete(repository.projects, projectID)
	return nil
}

func (repository *memoryManagedProjectRepository) ListPending(_ context.Context) ([]managedProject, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]managedProject, 0)
	for _, project := range repository.projects {
		if project.Status == "pending_review" {
			result = append(result, project)
		}
	}
	return result, nil
}

func (repository *memoryManagedProjectRepository) ListPublished(_ context.Context) ([]managedProject, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]managedProject, 0)
	for _, project := range repository.projects {
		if project.Status == "published" {
			result = append(result, project)
		}
	}
	return result, nil
}

func (repository *memoryManagedProjectRepository) FindPublishedBySlug(
	_ context.Context, slug string,
) (managedProject, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	for _, project := range repository.projects {
		if project.Slug == slug && project.Status == "published" {
			return project, true, nil
		}
	}
	return managedProject{}, false, nil
}

func (repository *memoryManagedProjectRepository) FindByID(
	_ context.Context, projectID string,
) (managedProject, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	project, found := repository.projects[projectID]
	return project, found, nil
}

func (repository *memoryManagedProjectRepository) UpdatePublishedDescription(
	_ context.Context, projectID, description string, now time.Time,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found {
		return managedProject{}, errProjectNotFound
	}
	if project.Status != "published" {
		return managedProject{}, errProjectImmutable
	}
	project.Description, project.UpdatedAt = description, now
	repository.projects[projectID] = project
	return project, nil
}

func (repository *memoryManagedProjectRepository) Review(
	_ context.Context, projectID, _ string, action, reason string, now time.Time,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found {
		return managedProject{}, errProjectNotFound
	}
	if project.Status != "pending_review" {
		return managedProject{}, errProjectImmutable
	}
	project.ReviewReason, project.UpdatedAt = reason, now
	if action == "approve" {
		project.Status, project.PublishedAt = "published", &now
	} else {
		project.Status = "rejected"
	}
	repository.projects[projectID] = project
	return project, nil
}

func (repository *memoryManagedProjectRepository) CountByStatus(_ context.Context) (map[string]int, error) {
	repository.RLock()
	defer repository.RUnlock()
	counts := make(map[string]int)
	for _, project := range repository.projects {
		counts[project.Status]++
	}
	return counts, nil
}

func (repository *memoryManagedProjectRepository) ListAll(
	_ context.Context, status string, limit, offset int,
) ([]managedProject, int, error) {
	repository.RLock()
	defer repository.RUnlock()
	matched := make([]managedProject, 0, len(repository.projects))
	for _, project := range repository.projects {
		if status != "" && project.Status != status {
			continue
		}
		matched = append(matched, project)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].UpdatedAt.Equal(matched[j].UpdatedAt) {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
	})
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []managedProject{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	page := make([]managedProject, end-offset)
	copy(page, matched[offset:end])
	return page, total, nil
}

func (repository *memoryManagedProjectRepository) Takedown(
	_ context.Context, projectID string, now time.Time,
) (managedProject, error) {
	repository.Lock()
	defer repository.Unlock()
	project, found := repository.projects[projectID]
	if !found {
		return managedProject{}, errProjectNotFound
	}
	if project.Status == projectDownedStatus {
		return managedProject{}, errProjectImmutable
	}
	project.Status, project.UpdatedAt = projectDownedStatus, now
	repository.projects[projectID] = project
	return project, nil
}

type mysqlManagedProjectRepository struct{ db *sql.DB }

func newMySQLManagedProjectRepository(db *sql.DB) *mysqlManagedProjectRepository {
	return &mysqlManagedProjectRepository{db: db}
}

func (repository *mysqlManagedProjectRepository) Create(
	ctx context.Context, ownerID string, input managedProjectInput,
) (managedProject, error) {
	project := projectFromInput(input)
	project.ID, project.OwnerID, project.Status = "project-"+newRequestID(), ownerID, "draft"
	tags, _ := json.Marshal(project.Tags)
	stack, _ := json.Marshal(project.TechStack)
	_, err := repository.db.ExecContext(ctx, `INSERT INTO managed_projects
		(id, owner_id, slug, name, summary, description, category, tags, tech_stack,
		 license, repository_url, cover_object_key, document_object_key, code_object_key,
		 current_version, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'draft')`,
		project.ID, ownerID, project.Slug, project.Name, project.Summary, project.Description,
		project.Category, tags, stack, project.License, project.RepositoryURL,
		project.CoverObjectKey, project.DocumentObjectKey, project.CodeObjectKey, project.CurrentVersion)
	if isDuplicateProjectSlug(err) {
		return managedProject{}, errProjectSlugExists
	}
	if err != nil {
		return managedProject{}, fmt.Errorf("create managed project: %w", err)
	}
	return repository.find(ctx, ownerID, project.ID)
}

func (repository *mysqlManagedProjectRepository) ListByOwner(
	ctx context.Context, ownerID string,
) ([]managedProject, error) {
	rows, err := repository.db.QueryContext(ctx, managedProjectSelect+
		` WHERE owner_id = ? ORDER BY updated_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list managed projects: %w", err)
	}
	defer rows.Close()
	result := make([]managedProject, 0)
	for rows.Next() {
		project, err := scanManagedProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, project)
	}
	return result, rows.Err()
}

func (repository *mysqlManagedProjectRepository) Update(
	ctx context.Context, ownerID, projectID string, input managedProjectInput,
) (managedProject, error) {
	tags, _ := json.Marshal(input.Tags)
	stack, _ := json.Marshal(input.TechStack)
	result, err := repository.db.ExecContext(ctx, `UPDATE managed_projects SET
		slug=?, name=?, summary=?, description=?, category=?, tags=?, tech_stack=?,
		license=?, repository_url=?, cover_object_key=?, document_object_key=?, code_object_key=?,
		current_version=?, status='draft', review_reason=''
		WHERE id=? AND owner_id=? AND status IN ('draft','rejected')`,
		input.Slug, input.Name, input.Summary, input.Description, input.Category, tags, stack,
		input.License, input.RepositoryURL, input.CoverObjectKey, input.DocumentObjectKey,
		input.CodeObjectKey, input.CurrentVersion, projectID, ownerID)
	if isDuplicateProjectSlug(err) {
		return managedProject{}, errProjectSlugExists
	}
	if err != nil {
		return managedProject{}, fmt.Errorf("update managed project: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return managedProject{}, repository.classifyMissing(ctx, ownerID, projectID)
	}
	return repository.find(ctx, ownerID, projectID)
}

func (repository *mysqlManagedProjectRepository) Submit(
	ctx context.Context, ownerID, projectID string, now time.Time,
) (managedProject, error) {
	result, err := repository.db.ExecContext(ctx, `UPDATE managed_projects
		SET status='pending_review', review_reason='', submitted_at=?
		WHERE id=? AND owner_id=? AND status IN ('draft','rejected')`, now, projectID, ownerID)
	if err != nil {
		return managedProject{}, fmt.Errorf("submit managed project: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return managedProject{}, repository.classifyMissing(ctx, ownerID, projectID)
	}
	return repository.find(ctx, ownerID, projectID)
}

func (repository *mysqlManagedProjectRepository) DeleteDraft(
	ctx context.Context, ownerID, projectID string,
) error {
	result, err := repository.db.ExecContext(ctx, `DELETE FROM managed_projects
		WHERE id=? AND owner_id=? AND status IN ('draft','rejected')`, projectID, ownerID)
	if err != nil {
		return fmt.Errorf("delete managed project: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return repository.classifyMissing(ctx, ownerID, projectID)
	}
	return nil
}

func (repository *mysqlManagedProjectRepository) ListPending(ctx context.Context) ([]managedProject, error) {
	return repository.listByStatus(ctx, "pending_review")
}

func (repository *mysqlManagedProjectRepository) ListPublished(ctx context.Context) ([]managedProject, error) {
	return repository.listByStatus(ctx, "published")
}

func (repository *mysqlManagedProjectRepository) FindPublishedBySlug(
	ctx context.Context, slug string,
) (managedProject, bool, error) {
	project, err := scanManagedProject(repository.db.QueryRowContext(
		ctx, managedProjectSelect+` WHERE slug=? AND status='published'`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return managedProject{}, false, nil
	}
	return project, err == nil, err
}

func (repository *mysqlManagedProjectRepository) FindByID(
	ctx context.Context, projectID string,
) (managedProject, bool, error) {
	project, err := scanManagedProject(repository.db.QueryRowContext(
		ctx, managedProjectSelect+` WHERE id=?`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return managedProject{}, false, nil
	}
	return project, err == nil, err
}

func (repository *mysqlManagedProjectRepository) UpdatePublishedDescription(
	ctx context.Context, projectID, description string, now time.Time,
) (managedProject, error) {
	result, err := repository.db.ExecContext(ctx, `UPDATE managed_projects
		SET description=?, updated_at=? WHERE id=? AND status='published'`,
		description, now, projectID)
	if err != nil {
		return managedProject{}, fmt.Errorf("update published project description: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return managedProject{}, errProjectNotFound
	}
	project, err := scanManagedProject(repository.db.QueryRowContext(
		ctx, managedProjectSelect+` WHERE id=? AND status='published'`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return managedProject{}, errProjectNotFound
	}
	return project, err
}

func (repository *mysqlManagedProjectRepository) listByStatus(
	ctx context.Context, status string,
) ([]managedProject, error) {
	rows, err := repository.db.QueryContext(ctx, managedProjectSelect+
		` WHERE status=? ORDER BY updated_at DESC`, status)
	if err != nil {
		return nil, fmt.Errorf("list managed projects by status: %w", err)
	}
	defer rows.Close()
	result := make([]managedProject, 0)
	for rows.Next() {
		project, err := scanManagedProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, project)
	}
	return result, rows.Err()
}

func (repository *mysqlManagedProjectRepository) Review(
	ctx context.Context, projectID, actorID, action, reason string, now time.Time,
) (managedProject, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return managedProject{}, fmt.Errorf("begin project review: %w", err)
	}
	defer transaction.Rollback()
	status := "rejected"
	var publishedAt any
	if action == "approve" {
		status, publishedAt = "published", now
	}
	result, err := transaction.ExecContext(ctx, `UPDATE managed_projects
		SET status=?, review_reason=?, published_at=?
		WHERE id=? AND status='pending_review'`, status, reason, publishedAt, projectID)
	if err != nil {
		return managedProject{}, fmt.Errorf("update project review: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var existing string
		err := transaction.QueryRowContext(ctx,
			`SELECT status FROM managed_projects WHERE id=?`, projectID).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return managedProject{}, errProjectNotFound
		}
		if err != nil {
			return managedProject{}, fmt.Errorf("find project for review: %w", err)
		}
		return managedProject{}, errProjectImmutable
	}
	_, err = transaction.ExecContext(ctx, `INSERT INTO project_review_events
		(id, project_id, actor_id, action, reason, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"review-"+newRequestID(), projectID, actorID, action, reason, now)
	if err != nil {
		return managedProject{}, fmt.Errorf("create project review event: %w", err)
	}
	project, err := scanManagedProject(transaction.QueryRowContext(
		ctx, managedProjectSelect+` WHERE id=?`, projectID))
	if err != nil {
		return managedProject{}, err
	}
	if err := transaction.Commit(); err != nil {
		return managedProject{}, fmt.Errorf("commit project review: %w", err)
	}
	return project, nil
}

func (repository *mysqlManagedProjectRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := repository.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM managed_projects GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count managed projects by status: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan project status count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func (repository *mysqlManagedProjectRepository) ListAll(
	ctx context.Context, status string, limit, offset int,
) ([]managedProject, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where, arguments := "", []any{}
	if status != "" {
		where = ` WHERE status = ?`
		arguments = append(arguments, status)
	}
	var total int
	if err := repository.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM managed_projects`+where, arguments...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count all managed projects: %w", err)
	}
	pageArguments := append(append([]any{}, arguments...), limit, offset)
	rows, err := repository.db.QueryContext(ctx, managedProjectSelect+where+
		` ORDER BY updated_at DESC, id ASC LIMIT ? OFFSET ?`, pageArguments...)
	if err != nil {
		return nil, 0, fmt.Errorf("list all managed projects: %w", err)
	}
	defer rows.Close()
	result := make([]managedProject, 0)
	for rows.Next() {
		project, err := scanManagedProject(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, project)
	}
	return result, total, rows.Err()
}

func (repository *mysqlManagedProjectRepository) Takedown(
	ctx context.Context, projectID string, now time.Time,
) (managedProject, error) {
	result, err := repository.db.ExecContext(ctx,
		`UPDATE managed_projects SET status=?, updated_at=? WHERE id=? AND status<>?`,
		projectDownedStatus, now, projectID, projectDownedStatus)
	if err != nil {
		return managedProject{}, fmt.Errorf("takedown managed project: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var existing string
		err := repository.db.QueryRowContext(ctx,
			`SELECT status FROM managed_projects WHERE id=?`, projectID).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return managedProject{}, errProjectNotFound
		}
		if err != nil {
			return managedProject{}, fmt.Errorf("classify takedown target: %w", err)
		}
		return managedProject{}, errProjectImmutable
	}
	project, err := scanManagedProject(repository.db.QueryRowContext(
		ctx, managedProjectSelect+` WHERE id=?`, projectID))
	if err != nil {
		return managedProject{}, err
	}
	return project, nil
}

const managedProjectSelect = `SELECT id, owner_id, slug, name, summary, description,
	category, tags, tech_stack, license, repository_url, cover_object_key,
	document_object_key, code_object_key, current_version, status,
	review_reason, submitted_at, published_at, created_at, updated_at FROM managed_projects`

type rowScanner interface{ Scan(...any) error }

func scanManagedProject(scanner rowScanner) (managedProject, error) {
	var project managedProject
	var tags, stack []byte
	err := scanner.Scan(&project.ID, &project.OwnerID, &project.Slug, &project.Name,
		&project.Summary, &project.Description, &project.Category, &tags, &stack,
		&project.License, &project.RepositoryURL, &project.CoverObjectKey,
		&project.DocumentObjectKey, &project.CodeObjectKey, &project.CurrentVersion, &project.Status,
		&project.ReviewReason, &project.SubmittedAt, &project.PublishedAt,
		&project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		return project, fmt.Errorf("scan managed project: %w", err)
	}
	if err := json.Unmarshal(tags, &project.Tags); err != nil {
		return project, fmt.Errorf("decode project tags: %w", err)
	}
	if err := json.Unmarshal(stack, &project.TechStack); err != nil {
		return project, fmt.Errorf("decode project stack: %w", err)
	}
	return project, nil
}

func (repository *mysqlManagedProjectRepository) find(
	ctx context.Context, ownerID, projectID string,
) (managedProject, error) {
	project, err := scanManagedProject(repository.db.QueryRowContext(
		ctx, managedProjectSelect+` WHERE id=? AND owner_id=?`, projectID, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return managedProject{}, errProjectNotFound
	}
	return project, err
}

func (repository *mysqlManagedProjectRepository) classifyMissing(
	ctx context.Context, ownerID, projectID string,
) error {
	var status string
	err := repository.db.QueryRowContext(ctx,
		`SELECT status FROM managed_projects WHERE id=? AND owner_id=?`, projectID, ownerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return errProjectNotFound
	}
	if err != nil {
		return fmt.Errorf("classify managed project: %w", err)
	}
	return errProjectImmutable
}

func isDuplicateProjectSlug(err error) bool {
	var mysqlError *mysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}

var managedProjectRepositoryStore managedProjectRepository = newMemoryManagedProjectRepository()

func authorProjectsHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		projects, err := managedProjectRepositoryStore.ListByOwner(request.Context(), user.ID)
		if err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": projects, "request_id": requestIDFromContext(request.Context())})
	case http.MethodPost:
		if !ensureNotBanned(writer, request, user.ID) {
			return
		}
		var input managedProjectInput
		if decodeJSONBody(request, &input) != nil || !validateManagedProjectInput(input) {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project", "项目资料不完整或格式不正确")
			return
		}
		input = normalizeManagedProjectInput(input)
		if !validProjectObjectOwnership(input, user.ID) {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project_file", "项目文件不属于当前账号")
			return
		}
		project, err := managedProjectRepositoryStore.Create(request.Context(), user.ID, input)
		if err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		auditAuth(request, "project_draft_created", user.Email, user.ID)
		writeJSON(writer, http.StatusCreated, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
	default:
		writer.Header().Set("Allow", "GET, POST")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

func authorProjectHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/author/projects/")
	submit := strings.HasSuffix(path, "/submit")
	projectID := strings.TrimSuffix(path, "/submit")
	if projectID == "" || strings.Contains(projectID, "/") {
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
		return
	}
	if submit {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
			return
		}
		project, err := managedProjectRepositoryStore.Submit(request.Context(), user.ID, projectID, time.Now().UTC())
		if err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		auditAuth(request, "project_submitted", user.Email, user.ID)
		writeJSON(writer, http.StatusOK, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
		return
	}
	switch request.Method {
	case http.MethodPut:
		var input managedProjectInput
		if decodeJSONBody(request, &input) != nil || !validateManagedProjectInput(input) {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project", "项目资料不完整或格式不正确")
			return
		}
		input = normalizeManagedProjectInput(input)
		if !validProjectObjectOwnership(input, user.ID) {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_project_file", "项目文件不属于当前账号")
			return
		}
		project, err := managedProjectRepositoryStore.Update(request.Context(), user.ID, projectID, input)
		if err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
	case http.MethodDelete:
		if err := managedProjectRepositoryStore.DeleteDraft(request.Context(), user.ID, projectID); err != nil {
			writeManagedProjectError(writer, request, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.Header().Set("Allow", "PUT, DELETE")
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
	}
}

func adminReviewsHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	_ = user
	projects, err := managedProjectRepositoryStore.ListPending(request.Context())
	if err != nil {
		writeManagedProjectError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projects, "request_id": requestIDFromContext(request.Context())})
}

func adminReviewActionHandler(writer http.ResponseWriter, request *http.Request) {
	user, ok := requireAdminUser(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/reviews/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || (parts[1] != "approve" && parts[1] != "reject") {
		writeAPIError(writer, request, http.StatusNotFound, "review_not_found", "审核任务不存在")
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if request.Body != nil && request.ContentLength != 0 && decodeJSONBody(request, &input) != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "审核数据格式不正确")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if parts[1] == "reject" && input.Reason == "" {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "review_reason_required", "驳回时必须填写原因")
		return
	}
	if len([]rune(input.Reason)) > 500 {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_review_reason", "审核意见不能超过 500 个字符")
		return
	}
	project, err := managedProjectRepositoryStore.Review(
		request.Context(), parts[0], user.ID, parts[1], input.Reason, time.Now().UTC())
	if err != nil {
		writeManagedProjectError(writer, request, err)
		return
	}
	recordAdminAudit(request, user, "project_review_"+parts[1], parts[0], input.Reason)
	if parts[1] == "approve" {
		// 项目通过审核＝发帖成功，给作者加经验（每项目一次，幂等）。
		awardExperienceBestEffort(project.OwnerID, xpActionPost, project.ID, xpPost)
	}
	notifyProjectReview(request.Context(), project, user, parts[1], input.Reason)
	if parts[1] == "approve" {
		// 项目（重新）发布后通知关注者，跳过发起审核的管理员本人。
		notifyProjectFollowers(request.Context(), project, user)
	}
	// 项目只有发布后才应进入搜索；驳回则从索引中清除。
	syncProjectSearchIndex(project, parts[1])
	writeJSON(writer, http.StatusOK, map[string]any{"data": project, "request_id": requestIDFromContext(request.Context())})
}

func requireAdminUser(writer http.ResponseWriter, request *http.Request) (authUser, bool) {
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return authUser{}, false
	}
	if isAdminEmail(user.Email) {
		return user, true
	}
	auditAuth(request, "admin_access_denied", user.Email, user.ID)
	writeAPIError(writer, request, http.StatusForbidden, "admin_required", "需要管理员权限")
	return authUser{}, false
}

func isAdminEmail(email string) bool {
	for _, configured := range strings.Split(os.Getenv("ADMIN_EMAILS"), ",") {
		if strings.EqualFold(strings.TrimSpace(configured), email) {
			return true
		}
	}
	return false
}

func projectFromInput(input managedProjectInput) managedProject {
	return managedProject{Slug: input.Slug, Name: input.Name, Summary: input.Summary,
		Description: input.Description, Category: input.Category, Tags: input.Tags,
		TechStack: input.TechStack, License: input.License,
		RepositoryURL: input.RepositoryURL, CoverObjectKey: input.CoverObjectKey,
		DocumentObjectKey: input.DocumentObjectKey, CodeObjectKey: input.CodeObjectKey,
		CurrentVersion: input.CurrentVersion}
}

func normalizeManagedProjectInput(input managedProjectInput) managedProjectInput {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name, input.Summary = strings.TrimSpace(input.Name), strings.TrimSpace(input.Summary)
	input.Description, input.Category = strings.TrimSpace(input.Description), strings.TrimSpace(input.Category)
	input.License, input.RepositoryURL = strings.TrimSpace(input.License), strings.TrimSpace(input.RepositoryURL)
	input.CoverObjectKey, input.DocumentObjectKey = strings.TrimSpace(input.CoverObjectKey), strings.TrimSpace(input.DocumentObjectKey)
	input.CodeObjectKey = strings.TrimSpace(input.CodeObjectKey)
	input.CurrentVersion = strings.TrimSpace(input.CurrentVersion)
	input.Tags, input.TechStack = cleanStringList(input.Tags), cleanStringList(input.TechStack)
	return input
}

func cleanStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func validateManagedProjectInput(raw managedProjectInput) bool {
	input := normalizeManagedProjectInput(raw)
	return projectSlugPattern.MatchString(input.Slug) && len(input.Slug) <= 80 &&
		len([]rune(input.Name)) >= 2 && len([]rune(input.Name)) <= 120 &&
		len([]rune(input.Summary)) >= 10 && len([]rune(input.Summary)) <= 300 &&
		len([]rune(input.Description)) >= 20 && len([]rune(input.Description)) <= 50000 &&
		input.Category != "" && len([]rune(input.Category)) <= 80 &&
		len(input.Tags) <= 10 && len(input.TechStack) <= 10 &&
		input.License != "" && len(input.License) <= 40 &&
		input.CurrentVersion != "" && len(input.CurrentVersion) <= 40 &&
		len(input.RepositoryURL) <= 500 && validOwnedObjectKey(input.CoverObjectKey) &&
		validOwnedObjectKey(input.DocumentObjectKey) && validOwnedObjectKey(input.CodeObjectKey)
}

func validOwnedObjectKey(key string) bool {
	return key == "" || (strings.HasPrefix(key, "uploads/user-") && len(key) <= 500 && !strings.Contains(key, ".."))
}

func validProjectObjectOwnership(input managedProjectInput, userID string) bool {
	prefix := "uploads/" + userID + "/"
	for _, key := range []string{input.CoverObjectKey, input.DocumentObjectKey, input.CodeObjectKey} {
		if key != "" && !strings.HasPrefix(key, prefix) {
			return false
		}
	}
	return true
}

func writeManagedProjectError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, errProjectSlugExists):
		writeAPIError(writer, request, http.StatusConflict, "project_slug_exists", "项目标识已被使用")
	case errors.Is(err, errProjectNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "project_not_found", "项目不存在")
	case errors.Is(err, errProjectImmutable):
		writeAPIError(writer, request, http.StatusConflict, "project_not_editable", "当前审核状态不允许修改")
	default:
		slog.ErrorContext(request.Context(), "managed project repository failed",
			"request_id", requestIDFromContext(request.Context()), "error", err)
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
	}
}
