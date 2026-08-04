package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// 文章版本历史。
//
// 编辑器每 1.2s 自动保存一次正文。如果每次保存都开一个新版本，历史列表会被
// 自动保存彻底刷爆，作者反而找不到有意义的还原点。因此这里采用「合并窗口」：
// 同一作者在 documentRevisionWindow 内的连续编辑原地更新最新那条版本记录，
// 只有换人编辑、超出窗口，或执行回滚时才递增版本号。
//
// 版本号从 1 开始，回滚也会推进版本号（把历史内容作为新版本追加），
// 因此回滚永远不会丢内容——回滚之后还能再回滚回去。

const (
	// documentRevisionWindow 是同一作者连续编辑的合并窗口。
	documentRevisionWindow = 5 * time.Minute
	// maxDocumentRevisions 是单篇文章保留的历史版本上限，超出后丢弃最旧的。
	maxDocumentRevisions = 50
	// documentRevisionListLimit 是历史列表接口一次返回的条数上限。
	documentRevisionListLimit = 50
)

// 版本来源。
const (
	revisionSourceCreate  = "create"
	revisionSourceEdit    = "edit"
	revisionSourceRestore = "restore"
)

var errRevisionNotFound = errors.New("document revision not found")

type documentRevision struct {
	ID         string `json:"id"`
	DocumentID string `json:"document_id"`
	ProjectID  string `json:"project_id"`
	Version    int    `json:"version"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Markdown   string `json:"markdown,omitempty"`
	AuthorID   string `json:"author_id,omitempty"`
	Source     string `json:"source"`
	// CharCount 是正文字符数。列表接口不回传正文，只带这个体量指标。
	CharCount int `json:"char_count"`
	// RestoredFrom 仅在 source 为 restore 时有值，记录还原自哪个版本号。
	RestoredFrom *int      `json:"restored_from,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// documentRevisionSummary 是历史列表用的轻量结构：不带正文，只带体量与作者信息。
type documentRevisionSummary struct {
	ID           string    `json:"id"`
	Version      int       `json:"version"`
	Title        string    `json:"title"`
	Slug         string    `json:"slug"`
	Source       string    `json:"source"`
	RestoredFrom *int      `json:"restored_from,omitempty"`
	AuthorID     string    `json:"author_id,omitempty"`
	AuthorName   string    `json:"author_name,omitempty"`
	AuthorAvatar string    `json:"author_avatar,omitempty"`
	CharCount    int       `json:"char_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Current 标记该版本是否就是文档当前正文所处的版本。
	Current bool `json:"current"`
}

type documentRevisionRepository interface {
	// Latest 返回文档最新的一条版本记录。
	Latest(ctx context.Context, documentID string) (documentRevision, bool, error)
	// Append 追加一条新版本。
	Append(ctx context.Context, revision documentRevision) (documentRevision, error)
	// Amend 原地更新已有版本的正文（合并窗口内的连续自动保存）。
	Amend(ctx context.Context, revisionID, slug, title, markdown string, now time.Time) (documentRevision, error)
	// List 按版本号倒序返回历史版本，不带正文。
	List(ctx context.Context, documentID string, limit int) ([]documentRevision, error)
	// Find 按版本号取出完整版本（含正文）。
	Find(ctx context.Context, documentID string, version int) (documentRevision, bool, error)
	// Prune 只保留最新的 keep 条版本，丢弃更旧的。
	Prune(ctx context.Context, documentID string, keep int) error
}

// recordDocumentRevision 在文章正文保存后留存历史版本，返回文档应处的版本号。
//
// 合并规则：最新版本由同一作者写入、来源是普通编辑、且距今不超过合并窗口时，
// 原地更新该版本；其余情况追加新版本。source 为 create/restore 时总是追加。
func recordDocumentRevision(
	ctx context.Context, document projectDocument, authorID, source string, restoredFrom *int, now time.Time,
) (documentRevision, error) {
	if documentRevisionRepositoryStore == nil {
		return documentRevision{}, nil
	}
	latest, found, err := documentRevisionRepositoryStore.Latest(ctx, document.ID)
	if err != nil {
		return documentRevision{}, err
	}
	if found && source == revisionSourceEdit && canCoalesceRevision(latest, authorID, now) {
		return documentRevisionRepositoryStore.Amend(
			ctx, latest.ID, document.Slug, document.Title, document.Markdown, now)
	}
	version := 1
	if found {
		version = latest.Version + 1
	}
	revision, err := documentRevisionRepositoryStore.Append(ctx, documentRevision{
		ID:         "revision-" + newRequestID(),
		DocumentID: document.ID,
		ProjectID:  document.ProjectID,
		Version:    version,
		Slug:       document.Slug,
		Title:      document.Title,
		Markdown:   document.Markdown,
		AuthorID:   authorID,
		Source:     source,
		// 只有回滚版本需要记录来源版本号，其余来源保持为空。
		RestoredFrom: restoredFrom,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return documentRevision{}, err
	}
	// 修剪是清理动作，失败不影响本次保存，交给日志和下一次保存重试。
	if err := documentRevisionRepositoryStore.Prune(ctx, document.ID, maxDocumentRevisions); err != nil {
		slog.WarnContext(ctx, "prune document revisions failed",
			"document_id", document.ID, "error", err)
	}
	return revision, nil
}

// canCoalesceRevision 判断能否把本次保存合并进最新版本。
func canCoalesceRevision(latest documentRevision, authorID string, now time.Time) bool {
	if latest.Source != revisionSourceEdit || latest.AuthorID != authorID || authorID == "" {
		return false
	}
	return now.Sub(latest.UpdatedAt) < documentRevisionWindow
}

// syncDocumentRevision 记录历史版本并把版本号回填到文档，返回带版本信息的文档。
// 历史记录失败不阻断保存本身：正文已经落库，作者不该因为历史服务抖动而看到保存失败。
func syncDocumentRevision(
	ctx context.Context, document projectDocument, authorID, source string, restoredFrom *int,
) projectDocument {
	revision, err := recordDocumentRevision(ctx, document, authorID, source, restoredFrom, time.Now().UTC())
	if err != nil {
		slog.ErrorContext(ctx, "record document revision failed",
			"document_id", document.ID, "error", err)
		return document
	}
	if revision.Version == 0 {
		return document
	}
	if err := projectDocumentRepositoryStore.ApplyRevisionMeta(
		ctx, document.ProjectID, document.ID, revision.Version, authorID); err != nil {
		slog.WarnContext(ctx, "apply document revision meta failed",
			"document_id", document.ID, "error", err)
	}
	document.Version, document.UpdatedBy = revision.Version, authorID
	return document
}

// summarizeRevisions 把历史版本转成列表结构，并补全作者昵称与头像。
func summarizeRevisions(
	ctx context.Context, revisions []documentRevision, currentVersion int,
) []documentRevisionSummary {
	authorIDs := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		if revision.AuthorID != "" {
			authorIDs = append(authorIDs, revision.AuthorID)
		}
	}
	authors := map[string]authUser{}
	if len(authorIDs) > 0 && authRepositoryStore != nil {
		resolved, err := authRepositoryStore.UsersByIDs(ctx, authorIDs)
		if err != nil {
			// 作者资料只是展示信息，取不到就退化成不显示昵称。
			slog.WarnContext(ctx, "resolve revision authors failed", "error", err)
		} else {
			authors = resolved
		}
	}
	summaries := make([]documentRevisionSummary, 0, len(revisions))
	for _, revision := range revisions {
		author := authors[revision.AuthorID]
		summaries = append(summaries, documentRevisionSummary{
			ID: revision.ID, Version: revision.Version, Title: revision.Title,
			Slug: revision.Slug, Source: revision.Source, RestoredFrom: revision.RestoredFrom,
			AuthorID: revision.AuthorID, AuthorName: author.DisplayName, AuthorAvatar: author.Avatar,
			CharCount: revision.CharCount, CreatedAt: revision.CreatedAt,
			UpdatedAt: revision.UpdatedAt, Current: revision.Version == currentVersion,
		})
	}
	return summaries
}

type memoryDocumentRevisionRepository struct {
	sync.RWMutex
	revisions map[string][]documentRevision
}

func newMemoryDocumentRevisionRepository() *memoryDocumentRevisionRepository {
	return &memoryDocumentRevisionRepository{revisions: make(map[string][]documentRevision)}
}

func (repository *memoryDocumentRevisionRepository) Latest(
	_ context.Context, documentID string,
) (documentRevision, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	stored := repository.revisions[documentID]
	if len(stored) == 0 {
		return documentRevision{}, false, nil
	}
	latest := stored[0]
	for _, revision := range stored[1:] {
		if revision.Version > latest.Version {
			latest = revision
		}
	}
	return latest, true, nil
}

func (repository *memoryDocumentRevisionRepository) Append(
	_ context.Context, revision documentRevision,
) (documentRevision, error) {
	repository.Lock()
	defer repository.Unlock()
	for _, existing := range repository.revisions[revision.DocumentID] {
		if existing.Version == revision.Version {
			return documentRevision{}, fmt.Errorf("document revision version %d exists", revision.Version)
		}
	}
	revision.CharCount = len([]rune(revision.Markdown))
	repository.revisions[revision.DocumentID] = append(repository.revisions[revision.DocumentID], revision)
	return revision, nil
}

func (repository *memoryDocumentRevisionRepository) Amend(
	_ context.Context, revisionID, slug, title, markdown string, now time.Time,
) (documentRevision, error) {
	repository.Lock()
	defer repository.Unlock()
	for documentID, stored := range repository.revisions {
		for index, revision := range stored {
			if revision.ID != revisionID {
				continue
			}
			revision.Slug, revision.Title, revision.Markdown = slug, title, markdown
			revision.CharCount, revision.UpdatedAt = len([]rune(markdown)), now
			repository.revisions[documentID][index] = revision
			return revision, nil
		}
	}
	return documentRevision{}, errRevisionNotFound
}

func (repository *memoryDocumentRevisionRepository) List(
	_ context.Context, documentID string, limit int,
) ([]documentRevision, error) {
	repository.RLock()
	defer repository.RUnlock()
	if limit <= 0 {
		limit = documentRevisionListLimit
	}
	stored := append([]documentRevision(nil), repository.revisions[documentID]...)
	sort.SliceStable(stored, func(left, right int) bool {
		return stored[left].Version > stored[right].Version
	})
	if len(stored) > limit {
		stored = stored[:limit]
	}
	return stored, nil
}

func (repository *memoryDocumentRevisionRepository) Find(
	_ context.Context, documentID string, version int,
) (documentRevision, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	for _, revision := range repository.revisions[documentID] {
		if revision.Version == version {
			return revision, true, nil
		}
	}
	return documentRevision{}, false, nil
}

func (repository *memoryDocumentRevisionRepository) Prune(
	_ context.Context, documentID string, keep int,
) error {
	if keep <= 0 {
		return nil
	}
	repository.Lock()
	defer repository.Unlock()
	stored := repository.revisions[documentID]
	if len(stored) <= keep {
		return nil
	}
	sort.SliceStable(stored, func(left, right int) bool {
		return stored[left].Version > stored[right].Version
	})
	repository.revisions[documentID] = stored[:keep]
	return nil
}

type mysqlDocumentRevisionRepository struct{ db *sql.DB }

func newMySQLDocumentRevisionRepository(db *sql.DB) *mysqlDocumentRevisionRepository {
	return &mysqlDocumentRevisionRepository{db: db}
}

const documentRevisionColumns = `id, document_id, project_id, version, slug, title,
	author_id, source, restored_from, created_at, updated_at`

// scanDocumentRevision 读取不含正文的版本记录。
func scanDocumentRevision(scanner rowScanner) (documentRevision, error) {
	var revision documentRevision
	var authorID sql.NullString
	var restoredFrom sql.NullInt64
	err := scanner.Scan(&revision.ID, &revision.DocumentID, &revision.ProjectID, &revision.Version,
		&revision.Slug, &revision.Title, &authorID, &revision.Source, &restoredFrom,
		&revision.CreatedAt, &revision.UpdatedAt)
	if err != nil {
		return revision, fmt.Errorf("scan document revision: %w", err)
	}
	revision.AuthorID = authorID.String
	if restoredFrom.Valid {
		version := int(restoredFrom.Int64)
		revision.RestoredFrom = &version
	}
	return revision, nil
}

// scanDocumentRevisionWithMarkdown 读取带正文的版本记录。
func scanDocumentRevisionWithMarkdown(scanner rowScanner) (documentRevision, error) {
	var revision documentRevision
	var authorID sql.NullString
	var restoredFrom sql.NullInt64
	err := scanner.Scan(&revision.ID, &revision.DocumentID, &revision.ProjectID, &revision.Version,
		&revision.Slug, &revision.Title, &authorID, &revision.Source, &restoredFrom,
		&revision.CreatedAt, &revision.UpdatedAt, &revision.Markdown)
	if err != nil {
		return revision, fmt.Errorf("scan document revision: %w", err)
	}
	revision.AuthorID = authorID.String
	if restoredFrom.Valid {
		version := int(restoredFrom.Int64)
		revision.RestoredFrom = &version
	}
	return revision, nil
}

func (repository *mysqlDocumentRevisionRepository) Latest(
	ctx context.Context, documentID string,
) (documentRevision, bool, error) {
	revision, err := scanDocumentRevision(repository.db.QueryRowContext(ctx,
		`SELECT `+documentRevisionColumns+` FROM project_document_revisions
		 WHERE document_id = ? ORDER BY version DESC LIMIT 1`, documentID))
	if errors.Is(err, sql.ErrNoRows) {
		return documentRevision{}, false, nil
	}
	return revision, err == nil, err
}

func (repository *mysqlDocumentRevisionRepository) Append(
	ctx context.Context, revision documentRevision,
) (documentRevision, error) {
	var author any
	if revision.AuthorID != "" {
		author = revision.AuthorID
	}
	var restoredFrom any
	if revision.RestoredFrom != nil {
		restoredFrom = *revision.RestoredFrom
	}
	_, err := repository.db.ExecContext(ctx, `INSERT INTO project_document_revisions
		(id, document_id, project_id, version, slug, title, content_markdown,
		 author_id, source, restored_from, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		revision.ID, revision.DocumentID, revision.ProjectID, revision.Version, revision.Slug,
		revision.Title, revision.Markdown, author, revision.Source, restoredFrom,
		revision.CreatedAt, revision.UpdatedAt)
	if err != nil {
		return documentRevision{}, fmt.Errorf("append document revision: %w", err)
	}
	return revision, nil
}

func (repository *mysqlDocumentRevisionRepository) Amend(
	ctx context.Context, revisionID, slug, title, markdown string, now time.Time,
) (documentRevision, error) {
	result, err := repository.db.ExecContext(ctx, `UPDATE project_document_revisions
		SET slug = ?, title = ?, content_markdown = ?, updated_at = ?
		WHERE id = ?`, slug, title, markdown, now, revisionID)
	if err != nil {
		return documentRevision{}, fmt.Errorf("amend document revision: %w", err)
	}
	// 连接串带 clientFoundRows=true，受影响行数是匹配行数，
	// 所以正文没变也不会被误判成记录不存在。
	if affected, _ := result.RowsAffected(); affected == 0 {
		return documentRevision{}, errRevisionNotFound
	}
	revision, err := scanDocumentRevisionWithMarkdown(repository.db.QueryRowContext(ctx,
		`SELECT `+documentRevisionColumns+`, content_markdown FROM project_document_revisions
		 WHERE id = ?`, revisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return documentRevision{}, errRevisionNotFound
	}
	return revision, err
}

func (repository *mysqlDocumentRevisionRepository) List(
	ctx context.Context, documentID string, limit int,
) ([]documentRevision, error) {
	if limit <= 0 {
		limit = documentRevisionListLimit
	}
	// 列表要展示体量变化，但不该把 50 篇正文全拉回来，因此只取字符数。
	rows, err := repository.db.QueryContext(ctx,
		`SELECT `+documentRevisionColumns+`, CHAR_LENGTH(content_markdown)
		 FROM project_document_revisions WHERE document_id = ?
		 ORDER BY version DESC LIMIT ?`, documentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list document revisions: %w", err)
	}
	defer rows.Close()
	result := make([]documentRevision, 0)
	for rows.Next() {
		var revision documentRevision
		var authorID sql.NullString
		var restoredFrom sql.NullInt64
		var charCount int
		err := rows.Scan(&revision.ID, &revision.DocumentID, &revision.ProjectID, &revision.Version,
			&revision.Slug, &revision.Title, &authorID, &revision.Source, &restoredFrom,
			&revision.CreatedAt, &revision.UpdatedAt, &charCount)
		if err != nil {
			return nil, fmt.Errorf("scan document revision row: %w", err)
		}
		revision.AuthorID = authorID.String
		if restoredFrom.Valid {
			version := int(restoredFrom.Int64)
			revision.RestoredFrom = &version
		}
		revision.CharCount = charCount
		result = append(result, revision)
	}
	return result, rows.Err()
}

func (repository *mysqlDocumentRevisionRepository) Find(
	ctx context.Context, documentID string, version int,
) (documentRevision, bool, error) {
	revision, err := scanDocumentRevisionWithMarkdown(repository.db.QueryRowContext(ctx,
		`SELECT `+documentRevisionColumns+`, content_markdown FROM project_document_revisions
		 WHERE document_id = ? AND version = ?`, documentID, version))
	if errors.Is(err, sql.ErrNoRows) {
		return documentRevision{}, false, nil
	}
	return revision, err == nil, err
}

func (repository *mysqlDocumentRevisionRepository) Prune(
	ctx context.Context, documentID string, keep int,
) error {
	if keep <= 0 {
		return nil
	}
	// 先定位第 keep 新的版本号，再删掉比它旧的，避免 DELETE 里直接用 LIMIT/OFFSET 子查询。
	var threshold int
	err := repository.db.QueryRowContext(ctx,
		`SELECT version FROM project_document_revisions WHERE document_id = ?
		 ORDER BY version DESC LIMIT 1 OFFSET ?`, documentID, keep-1).Scan(&threshold)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("locate document revision threshold: %w", err)
	}
	if _, err := repository.db.ExecContext(ctx,
		`DELETE FROM project_document_revisions WHERE document_id = ? AND version < ?`,
		documentID, threshold); err != nil {
		return fmt.Errorf("prune document revisions: %w", err)
	}
	return nil
}

var documentRevisionRepositoryStore documentRevisionRepository = newMemoryDocumentRevisionRepository()

var _ documentRevisionRepository = (*memoryDocumentRevisionRepository)(nil)
var _ documentRevisionRepository = (*mysqlDocumentRevisionRepository)(nil)
