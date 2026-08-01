package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 项目文档树。一个项目可以有多篇文档并按父子关系组成多层目录，
// 对齐在线文档工具的知识库结构。
//
// 兼容策略：项目还没有任何文档时，阅读页仍把项目正文（managed_projects.description）
// 作为单篇文档提供，因此老项目升级后不会出现空白文档页。

var (
	errDocumentNotFound   = errors.New("project document not found")
	errDocumentSlugExists = errors.New("project document slug exists")
	errDocumentCycle      = errors.New("project document parent would create a cycle")
	documentSlugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

const (
	maxDocumentsPerProject = 500
	maxDocumentDepth       = 5
	maxDocumentMarkdown    = 200_000
)

type projectDocument struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	ParentID  *string   `json:"parent_id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Markdown  string    `json:"markdown"`
	SortOrder int       `json:"sort_order"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type projectDocumentInput struct {
	ParentID *string `json:"parent_id"`
	Slug     string  `json:"slug"`
	Title    string  `json:"title"`
	Markdown string  `json:"markdown"`
}

// documentMove 描述一次目录调整：改变父级并落到指定位置。
type documentMove struct {
	ParentID  *string `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

type projectDocumentRepository interface {
	ListByProject(ctx context.Context, projectID string) ([]projectDocument, error)
	FindBySlug(ctx context.Context, projectID, slug string) (projectDocument, bool, error)
	Create(ctx context.Context, projectID, authorID string, input projectDocumentInput) (projectDocument, error)
	Update(ctx context.Context, projectID, documentID string, input projectDocumentInput) (projectDocument, error)
	UpdateMarkdown(ctx context.Context, projectID, documentID, markdown string, now time.Time) (projectDocument, error)
	Move(ctx context.Context, projectID, documentID string, move documentMove) (projectDocument, error)
	Delete(ctx context.Context, projectID, documentID string, now time.Time) error
}

// normalizeDocumentInput 统一裁剪空白并小写 slug。
func normalizeDocumentInput(input projectDocumentInput) projectDocumentInput {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Title = strings.TrimSpace(input.Title)
	if input.ParentID != nil {
		parent := strings.TrimSpace(*input.ParentID)
		if parent == "" {
			input.ParentID = nil
		} else {
			input.ParentID = &parent
		}
	}
	return input
}

func validDocumentInput(input projectDocumentInput) bool {
	normalized := normalizeDocumentInput(input)
	return documentSlugPattern.MatchString(normalized.Slug) && len(normalized.Slug) <= 160 &&
		len([]rune(normalized.Title)) >= 1 && len([]rune(normalized.Title)) <= 200 &&
		len(normalized.Markdown) <= maxDocumentMarkdown
}

// buildDocumentTree 把平铺文档按父子关系组装成多层目录，同级按 sort_order 再按标题排序。
func buildDocumentTree(flat []projectDocument) []documentNode {
	children := make(map[string][]projectDocument)
	for _, document := range flat {
		parent := ""
		if document.ParentID != nil {
			parent = *document.ParentID
		}
		children[parent] = append(children[parent], document)
	}
	for key := range children {
		siblings := children[key]
		sort.SliceStable(siblings, func(left, right int) bool {
			if siblings[left].SortOrder != siblings[right].SortOrder {
				return siblings[left].SortOrder < siblings[right].SortOrder
			}
			return siblings[left].Title < siblings[right].Title
		})
		children[key] = siblings
	}

	// 递归组装时限制深度，避免异常数据造成无限递归。
	var assemble func(parent string, depth int) []documentNode
	assemble = func(parent string, depth int) []documentNode {
		nodes := make([]documentNode, 0, len(children[parent]))
		if depth > maxDocumentDepth {
			return nodes
		}
		for _, document := range children[parent] {
			nodes = append(nodes, documentNode{
				ID: document.ID, Slug: document.Slug, Title: document.Title,
				Order: document.SortOrder, Children: assemble(document.ID, depth+1),
			})
		}
		return nodes
	}
	return assemble("", 1)
}

// documentDetailFrom 把存储的文档转成阅读页需要的结构，大纲和稳定块由正文解析得出。
func documentDetailFrom(document projectDocument, version string) documentDetail {
	parsed := parseMarkdownDocument(document.Markdown)
	return documentDetail{
		ID:        document.ID,
		ProjectID: document.ProjectID,
		Slug:      document.Slug,
		Title:     document.Title,
		Version:   version,
		UpdatedAt: document.UpdatedAt.UTC().Format(time.RFC3339),
		Markdown:  document.Markdown,
		Outline:   parsed.Outline,
		Blocks:    parsed.Blocks,
	}
}

type memoryProjectDocumentRepository struct {
	sync.RWMutex
	documents map[string]projectDocument
}

func newMemoryProjectDocumentRepository() *memoryProjectDocumentRepository {
	return &memoryProjectDocumentRepository{documents: make(map[string]projectDocument)}
}

func (repository *memoryProjectDocumentRepository) ListByProject(
	_ context.Context, projectID string,
) ([]projectDocument, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]projectDocument, 0)
	for _, document := range repository.documents {
		if document.ProjectID == projectID {
			result = append(result, document)
		}
	}
	return result, nil
}

func (repository *memoryProjectDocumentRepository) FindBySlug(
	_ context.Context, projectID, slug string,
) (projectDocument, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	for _, document := range repository.documents {
		if document.ProjectID == projectID && document.Slug == slug {
			return document, true, nil
		}
	}
	return projectDocument{}, false, nil
}

func (repository *memoryProjectDocumentRepository) Create(
	_ context.Context, projectID, authorID string, input projectDocumentInput,
) (projectDocument, error) {
	repository.Lock()
	defer repository.Unlock()
	count := 0
	for _, document := range repository.documents {
		if document.ProjectID != projectID {
			continue
		}
		count++
		if document.Slug == input.Slug {
			return projectDocument{}, errDocumentSlugExists
		}
	}
	if count >= maxDocumentsPerProject {
		return projectDocument{}, fmt.Errorf("project document limit reached")
	}
	if err := repository.validateParentLocked(projectID, "", input.ParentID); err != nil {
		return projectDocument{}, err
	}
	now := time.Now().UTC()
	document := projectDocument{
		ID: "document-" + newRequestID(), ProjectID: projectID, ParentID: input.ParentID,
		Slug: input.Slug, Title: input.Title, Markdown: input.Markdown,
		SortOrder: count, CreatedBy: authorID, CreatedAt: now, UpdatedAt: now,
	}
	repository.documents[document.ID] = document
	return document, nil
}

func (repository *memoryProjectDocumentRepository) Update(
	_ context.Context, projectID, documentID string, input projectDocumentInput,
) (projectDocument, error) {
	repository.Lock()
	defer repository.Unlock()
	document, found := repository.documents[documentID]
	if !found || document.ProjectID != projectID {
		return projectDocument{}, errDocumentNotFound
	}
	for id, existing := range repository.documents {
		if id != documentID && existing.ProjectID == projectID && existing.Slug == input.Slug {
			return projectDocument{}, errDocumentSlugExists
		}
	}
	if err := repository.validateParentLocked(projectID, documentID, input.ParentID); err != nil {
		return projectDocument{}, err
	}
	document.ParentID, document.Slug, document.Title = input.ParentID, input.Slug, input.Title
	document.Markdown, document.UpdatedAt = input.Markdown, time.Now().UTC()
	repository.documents[documentID] = document
	return document, nil
}

func (repository *memoryProjectDocumentRepository) UpdateMarkdown(
	_ context.Context, projectID, documentID, markdown string, now time.Time,
) (projectDocument, error) {
	repository.Lock()
	defer repository.Unlock()
	document, found := repository.documents[documentID]
	if !found || document.ProjectID != projectID {
		return projectDocument{}, errDocumentNotFound
	}
	document.Markdown, document.UpdatedAt = markdown, now
	repository.documents[documentID] = document
	return document, nil
}

func (repository *memoryProjectDocumentRepository) Move(
	_ context.Context, projectID, documentID string, move documentMove,
) (projectDocument, error) {
	repository.Lock()
	defer repository.Unlock()
	document, found := repository.documents[documentID]
	if !found || document.ProjectID != projectID {
		return projectDocument{}, errDocumentNotFound
	}
	if err := repository.validateParentLocked(projectID, documentID, move.ParentID); err != nil {
		return projectDocument{}, err
	}
	document.ParentID, document.SortOrder = move.ParentID, move.SortOrder
	document.UpdatedAt = time.Now().UTC()
	repository.documents[documentID] = document
	return document, nil
}

func (repository *memoryProjectDocumentRepository) Delete(
	_ context.Context, projectID, documentID string, _ time.Time,
) error {
	repository.Lock()
	defer repository.Unlock()
	document, found := repository.documents[documentID]
	if !found || document.ProjectID != projectID {
		return errDocumentNotFound
	}
	// 递归删除子文档，与 MySQL 的级联行为保持一致。
	remove := []string{documentID}
	for len(remove) > 0 {
		current := remove[len(remove)-1]
		remove = remove[:len(remove)-1]
		for id, candidate := range repository.documents {
			if candidate.ParentID != nil && *candidate.ParentID == current {
				remove = append(remove, id)
			}
		}
		delete(repository.documents, current)
	}
	return nil
}

// validateParentLocked 校验父级属于同一项目、不指向自身、不形成环且不超过深度上限。
func (repository *memoryProjectDocumentRepository) validateParentLocked(
	projectID, documentID string, parentID *string,
) error {
	if parentID == nil {
		return nil
	}
	if *parentID == documentID {
		return errDocumentCycle
	}
	depth := 1
	cursor := *parentID
	for cursor != "" {
		parent, found := repository.documents[cursor]
		if !found || parent.ProjectID != projectID {
			return errDocumentNotFound
		}
		if documentID != "" && parent.ID == documentID {
			return errDocumentCycle
		}
		depth++
		if depth > maxDocumentDepth {
			return errDocumentCycle
		}
		if parent.ParentID == nil {
			break
		}
		cursor = *parent.ParentID
	}
	return nil
}

type mysqlProjectDocumentRepository struct{ db *sql.DB }

func newMySQLProjectDocumentRepository(db *sql.DB) *mysqlProjectDocumentRepository {
	return &mysqlProjectDocumentRepository{db: db}
}

const projectDocumentSelect = `SELECT id, project_id, parent_id, slug, title,
	content_markdown, sort_order, created_by, created_at, updated_at
	FROM project_documents`

func scanProjectDocument(scanner rowScanner) (projectDocument, error) {
	var document projectDocument
	err := scanner.Scan(&document.ID, &document.ProjectID, &document.ParentID, &document.Slug,
		&document.Title, &document.Markdown, &document.SortOrder, &document.CreatedBy,
		&document.CreatedAt, &document.UpdatedAt)
	if err != nil {
		return document, fmt.Errorf("scan project document: %w", err)
	}
	return document, nil
}

func (repository *mysqlProjectDocumentRepository) ListByProject(
	ctx context.Context, projectID string,
) ([]projectDocument, error) {
	rows, err := repository.db.QueryContext(ctx, projectDocumentSelect+
		` WHERE project_id = ? AND deleted_at IS NULL ORDER BY sort_order, title`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project documents: %w", err)
	}
	defer rows.Close()
	result := make([]projectDocument, 0)
	for rows.Next() {
		document, err := scanProjectDocument(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, document)
	}
	return result, rows.Err()
}

func (repository *mysqlProjectDocumentRepository) FindBySlug(
	ctx context.Context, projectID, slug string,
) (projectDocument, bool, error) {
	document, err := scanProjectDocument(repository.db.QueryRowContext(ctx,
		projectDocumentSelect+` WHERE project_id = ? AND slug = ? AND deleted_at IS NULL`,
		projectID, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return projectDocument{}, false, nil
	}
	return document, err == nil, err
}

func (repository *mysqlProjectDocumentRepository) Create(
	ctx context.Context, projectID, authorID string, input projectDocumentInput,
) (projectDocument, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return projectDocument{}, fmt.Errorf("begin create project document: %w", err)
	}
	defer transaction.Rollback()

	var count int
	if err := transaction.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_documents WHERE project_id = ? AND deleted_at IS NULL`,
		projectID).Scan(&count); err != nil {
		return projectDocument{}, fmt.Errorf("count project documents: %w", err)
	}
	if count >= maxDocumentsPerProject {
		return projectDocument{}, fmt.Errorf("project document limit reached")
	}
	if err := validateDocumentParent(ctx, transaction, projectID, "", input.ParentID); err != nil {
		return projectDocument{}, err
	}

	documentID := "document-" + newRequestID()
	_, err = transaction.ExecContext(ctx, `INSERT INTO project_documents
		(id, project_id, parent_id, slug, title, content_markdown, sort_order, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		documentID, projectID, input.ParentID, input.Slug, input.Title, input.Markdown, count, authorID)
	if isDuplicateDocumentSlug(err) {
		return projectDocument{}, errDocumentSlugExists
	}
	if err != nil {
		return projectDocument{}, fmt.Errorf("create project document: %w", err)
	}
	document, err := scanProjectDocument(transaction.QueryRowContext(ctx,
		projectDocumentSelect+` WHERE id = ?`, documentID))
	if err != nil {
		return projectDocument{}, err
	}
	if err := transaction.Commit(); err != nil {
		return projectDocument{}, fmt.Errorf("commit create project document: %w", err)
	}
	return document, nil
}

func (repository *mysqlProjectDocumentRepository) Update(
	ctx context.Context, projectID, documentID string, input projectDocumentInput,
) (projectDocument, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return projectDocument{}, fmt.Errorf("begin update project document: %w", err)
	}
	defer transaction.Rollback()

	if err := validateDocumentParent(ctx, transaction, projectID, documentID, input.ParentID); err != nil {
		return projectDocument{}, err
	}
	result, err := transaction.ExecContext(ctx, `UPDATE project_documents
		SET parent_id = ?, slug = ?, title = ?, content_markdown = ?
		WHERE id = ? AND project_id = ? AND deleted_at IS NULL`,
		input.ParentID, input.Slug, input.Title, input.Markdown, documentID, projectID)
	if isDuplicateDocumentSlug(err) {
		return projectDocument{}, errDocumentSlugExists
	}
	if err != nil {
		return projectDocument{}, fmt.Errorf("update project document: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return projectDocument{}, errDocumentNotFound
	}
	document, err := scanProjectDocument(transaction.QueryRowContext(ctx,
		projectDocumentSelect+` WHERE id = ?`, documentID))
	if err != nil {
		return projectDocument{}, err
	}
	if err := transaction.Commit(); err != nil {
		return projectDocument{}, fmt.Errorf("commit update project document: %w", err)
	}
	return document, nil
}

func (repository *mysqlProjectDocumentRepository) UpdateMarkdown(
	ctx context.Context, projectID, documentID, markdown string, now time.Time,
) (projectDocument, error) {
	result, err := repository.db.ExecContext(ctx, `UPDATE project_documents
		SET content_markdown = ?, updated_at = ?
		WHERE id = ? AND project_id = ? AND deleted_at IS NULL`,
		markdown, now, documentID, projectID)
	if err != nil {
		return projectDocument{}, fmt.Errorf("update project document markdown: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return projectDocument{}, errDocumentNotFound
	}
	document, err := scanProjectDocument(repository.db.QueryRowContext(ctx,
		projectDocumentSelect+` WHERE id = ?`, documentID))
	if errors.Is(err, sql.ErrNoRows) {
		return projectDocument{}, errDocumentNotFound
	}
	return document, err
}

func (repository *mysqlProjectDocumentRepository) Move(
	ctx context.Context, projectID, documentID string, move documentMove,
) (projectDocument, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return projectDocument{}, fmt.Errorf("begin move project document: %w", err)
	}
	defer transaction.Rollback()

	if err := validateDocumentParent(ctx, transaction, projectID, documentID, move.ParentID); err != nil {
		return projectDocument{}, err
	}
	result, err := transaction.ExecContext(ctx, `UPDATE project_documents
		SET parent_id = ?, sort_order = ?
		WHERE id = ? AND project_id = ? AND deleted_at IS NULL`,
		move.ParentID, move.SortOrder, documentID, projectID)
	if err != nil {
		return projectDocument{}, fmt.Errorf("move project document: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return projectDocument{}, errDocumentNotFound
	}
	document, err := scanProjectDocument(transaction.QueryRowContext(ctx,
		projectDocumentSelect+` WHERE id = ?`, documentID))
	if err != nil {
		return projectDocument{}, err
	}
	if err := transaction.Commit(); err != nil {
		return projectDocument{}, fmt.Errorf("commit move project document: %w", err)
	}
	return document, nil
}

func (repository *mysqlProjectDocumentRepository) Delete(
	ctx context.Context, projectID, documentID string, now time.Time,
) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete project document: %w", err)
	}
	defer transaction.Rollback()

	// 软删除自身与全部后代，逐层收集避免依赖数据库递归查询。
	ids := []string{documentID}
	frontier := []string{documentID}
	for depth := 0; depth < maxDocumentDepth && len(frontier) > 0; depth++ {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(frontier)), ",")
		arguments := make([]any, 0, len(frontier)+1)
		arguments = append(arguments, projectID)
		for _, id := range frontier {
			arguments = append(arguments, id)
		}
		rows, err := transaction.QueryContext(ctx,
			`SELECT id FROM project_documents WHERE project_id = ? AND deleted_at IS NULL
			 AND parent_id IN (`+placeholders+`)`, arguments...)
		if err != nil {
			return fmt.Errorf("collect child documents: %w", err)
		}
		next := make([]string, 0)
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("scan child document: %w", err)
			}
			next = append(next, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate child documents: %w", err)
		}
		ids = append(ids, next...)
		frontier = next
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, 0, len(ids)+2)
	arguments = append(arguments, now, projectID)
	for _, id := range ids {
		arguments = append(arguments, id)
	}
	result, err := transaction.ExecContext(ctx, `UPDATE project_documents SET deleted_at = ?
		WHERE project_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return fmt.Errorf("delete project documents: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errDocumentNotFound
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit delete project document: %w", err)
	}
	return nil
}

type documentQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// validateDocumentParent 校验父级存在于同一项目、不指向自身且层级不超过上限。
func validateDocumentParent(
	ctx context.Context, querier documentQuerier, projectID, documentID string, parentID *string,
) error {
	if parentID == nil {
		return nil
	}
	if *parentID == documentID {
		return errDocumentCycle
	}
	cursor := *parentID
	for depth := 1; depth <= maxDocumentDepth; depth++ {
		var owner string
		var next sql.NullString
		err := querier.QueryRowContext(ctx,
			`SELECT project_id, parent_id FROM project_documents
			 WHERE id = ? AND deleted_at IS NULL`, cursor).Scan(&owner, &next)
		if errors.Is(err, sql.ErrNoRows) {
			return errDocumentNotFound
		}
		if err != nil {
			return fmt.Errorf("validate document parent: %w", err)
		}
		if owner != projectID {
			return errDocumentNotFound
		}
		if !next.Valid {
			return nil
		}
		if documentID != "" && next.String == documentID {
			return errDocumentCycle
		}
		cursor = next.String
	}
	return errDocumentCycle
}

func isDuplicateDocumentSlug(err error) bool {
	var mysqlError *mysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}

var projectDocumentRepositoryStore projectDocumentRepository = newMemoryProjectDocumentRepository()

var _ projectDocumentRepository = (*memoryProjectDocumentRepository)(nil)
var _ projectDocumentRepository = (*mysqlProjectDocumentRepository)(nil)
