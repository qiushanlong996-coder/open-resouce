package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/go-sql-driver/mysql"
)

type mysqlCommentRepository struct {
	db *sql.DB
}

func newMySQLCommentRepository(db *sql.DB) *mysqlCommentRepository {
	return &mysqlCommentRepository{db: db}
}

func (repository *mysqlCommentRepository) List(ctx context.Context, documentID string) ([]documentComment, error) {
	const query = `
		SELECT id, document_id, parent_id, block_id, author_id, author_name, author_region, quote_text, body_text,
		       status, created_at, updated_at, resolved_at
		FROM document_comments
		WHERE document_id = ? AND deleted_at IS NULL
		ORDER BY created_at, id`

	rows, err := repository.db.QueryContext(ctx, query, documentID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]documentComment, 0)
	for rows.Next() {
		comment, err := scanDocumentComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comments: %w", err)
	}
	return nestDocumentComments(comments), nil
}

func (repository *mysqlCommentRepository) Create(ctx context.Context, comment documentComment) (documentComment, error) {
	createdAt, err := time.Parse(time.RFC3339, comment.CreatedAt)
	if err != nil {
		return documentComment{}, fmt.Errorf("parse comment creation time: %w", err)
	}

	const query = `
		INSERT INTO document_comments (
			id, document_id, parent_id, block_id, author_id, author_name, author_region, quote_text, body_text,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := repository.db.ExecContext(ctx, query,
		comment.ID, comment.DocumentID, comment.ParentID, comment.BlockID, nullableString(comment.AuthorID), comment.Author,
		comment.AuthorRegion, comment.Quote, comment.Body, comment.Status, createdAt, createdAt,
	); err != nil {
		return documentComment{}, fmt.Errorf("create comment: %w", err)
	}
	comment.UpdatedAt = comment.CreatedAt
	return comment, nil
}

func (repository *mysqlCommentRepository) CreateReply(
	ctx context.Context,
	parentID string,
	reply documentComment,
) (documentComment, bool, error) {
	createdAt, err := time.Parse(time.RFC3339, reply.CreatedAt)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("parse reply creation time: %w", err)
	}
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("begin create reply: %w", err)
	}
	defer transaction.Rollback()

	var blockID string
	const parentQuery = `
		SELECT block_id
		FROM document_comments
		WHERE id = ? AND document_id = ? AND parent_id IS NULL AND deleted_at IS NULL
		FOR UPDATE`
	if err := transaction.QueryRowContext(ctx, parentQuery, parentID, reply.DocumentID).Scan(&blockID); errors.Is(err, sql.ErrNoRows) {
		return documentComment{}, false, nil
	} else if err != nil {
		return documentComment{}, false, fmt.Errorf("select reply parent: %w", err)
	}

	reply.ParentID = &parentID
	reply.BlockID = blockID
	const insertQuery = `
		INSERT INTO document_comments (
			id, document_id, parent_id, block_id, author_id, author_name, author_region, quote_text, body_text,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := transaction.ExecContext(ctx, insertQuery,
		reply.ID, reply.DocumentID, reply.ParentID, reply.BlockID, nullableString(reply.AuthorID), reply.Author,
		reply.AuthorRegion, reply.Quote, reply.Body, reply.Status, createdAt, createdAt,
	); err != nil {
		return documentComment{}, false, fmt.Errorf("create reply: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return documentComment{}, false, fmt.Errorf("commit reply: %w", err)
	}
	reply.UpdatedAt = reply.CreatedAt
	return reply, true, nil
}

func (repository *mysqlCommentRepository) DeleteComment(
	ctx context.Context,
	documentID, commentID, actorID, deletedAtValue string,
) (bool, error) {
	deletedAt, err := time.Parse(time.RFC3339, deletedAtValue)
	if err != nil {
		return false, fmt.Errorf("parse comment deletion time: %w", err)
	}
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete comment: %w", err)
	}
	defer transaction.Rollback()

	var selectedID string
	const selectQuery = `
		SELECT id FROM document_comments
		WHERE id = ? AND document_id = ? AND parent_id IS NULL
		  AND (? = '' OR author_id = ?) AND deleted_at IS NULL
		FOR UPDATE`
	err = transaction.QueryRowContext(ctx, selectQuery, commentID, documentID, actorID, actorID).Scan(&selectedID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select comment for deletion: %w", err)
	}
	const childrenQuery = `
		UPDATE document_comments SET deleted_at = ?, updated_at = ?
		WHERE parent_id = ? AND document_id = ? AND deleted_at IS NULL`
	if _, err := transaction.ExecContext(ctx, childrenQuery, deletedAt, deletedAt, commentID, documentID); err != nil {
		return false, fmt.Errorf("delete comment replies: %w", err)
	}
	const parentQuery = `
		UPDATE document_comments SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND document_id = ? AND deleted_at IS NULL`
	if _, err := transaction.ExecContext(ctx, parentQuery, deletedAt, deletedAt, commentID, documentID); err != nil {
		return false, fmt.Errorf("delete comment: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit deleted comment: %w", err)
	}
	return true, nil
}

func (repository *mysqlCommentRepository) DeleteReply(
	ctx context.Context,
	documentID, parentID, replyID, actorID, deletedAtValue string,
) (bool, error) {
	deletedAt, err := time.Parse(time.RFC3339, deletedAtValue)
	if err != nil {
		return false, fmt.Errorf("parse reply deletion time: %w", err)
	}
	const query = `
		UPDATE document_comments
		SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND document_id = ? AND parent_id = ?
		  AND (? = '' OR author_id = ?) AND deleted_at IS NULL`
	result, err := repository.db.ExecContext(ctx, query, deletedAt, deletedAt, replyID, documentID, parentID, actorID, actorID)
	if err != nil {
		return false, fmt.Errorf("delete reply: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read deleted reply result: %w", err)
	}
	return affected == 1, nil
}

func (repository *mysqlCommentRepository) UpdateComment(
	ctx context.Context,
	documentID, commentID, actorID, body, updatedAtValue string,
) (documentComment, bool, error) {
	return repository.updateBody(ctx, documentID, "", commentID, actorID, body, updatedAtValue)
}

func (repository *mysqlCommentRepository) UpdateReply(
	ctx context.Context,
	documentID, parentID, replyID, actorID, body, updatedAtValue string,
) (documentComment, bool, error) {
	return repository.updateBody(ctx, documentID, parentID, replyID, actorID, body, updatedAtValue)
}

func (repository *mysqlCommentRepository) updateBody(
	ctx context.Context,
	documentID, parentID, commentID, actorID, body, updatedAtValue string,
) (documentComment, bool, error) {
	updatedAt, err := time.Parse(time.RFC3339, updatedAtValue)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("parse comment update time: %w", err)
	}
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("begin update comment: %w", err)
	}
	defer transaction.Rollback()

	var result sql.Result
	if parentID == "" {
		const query = `
			UPDATE document_comments
			SET body_text = ?, updated_at = ?
			WHERE id = ? AND document_id = ? AND parent_id IS NULL
			  AND (? = '' OR author_id = ?) AND deleted_at IS NULL`
		result, err = transaction.ExecContext(ctx, query, body, updatedAt, commentID, documentID, actorID, actorID)
	} else {
		const query = `
			UPDATE document_comments
			SET body_text = ?, updated_at = ?
			WHERE id = ? AND document_id = ? AND parent_id = ?
			  AND (? = '' OR author_id = ?) AND deleted_at IS NULL`
		result, err = transaction.ExecContext(ctx, query, body, updatedAt, commentID, documentID, parentID, actorID, actorID)
	}
	if err != nil {
		return documentComment{}, false, fmt.Errorf("update comment body: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return documentComment{}, false, fmt.Errorf("read updated comment result: %w", err)
	}
	if affected == 0 {
		return documentComment{}, false, nil
	}

	const selectQuery = `
		SELECT id, document_id, parent_id, block_id, author_id, author_name, author_region, quote_text, body_text,
		       status, created_at, updated_at, resolved_at
		FROM document_comments
		WHERE id = ? AND document_id = ? AND deleted_at IS NULL`
	comment, err := scanDocumentComment(transaction.QueryRowContext(ctx, selectQuery, commentID, documentID))
	if err != nil {
		return documentComment{}, false, fmt.Errorf("select updated comment: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return documentComment{}, false, fmt.Errorf("commit updated comment: %w", err)
	}
	return comment, true, nil
}

func (repository *mysqlCommentRepository) Resolve(
	ctx context.Context,
	documentID, commentID, actorID, resolvedAtValue string,
) (documentComment, bool, error) {
	resolvedAt, err := time.Parse(time.RFC3339, resolvedAtValue)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("parse comment resolution time: %w", err)
	}

	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return documentComment{}, false, fmt.Errorf("begin resolve comment: %w", err)
	}
	defer transaction.Rollback()

	const selectQuery = `
		SELECT id, document_id, parent_id, block_id, author_id, author_name, author_region, quote_text, body_text,
		       status, created_at, updated_at, resolved_at
		FROM document_comments
		WHERE id = ? AND document_id = ? AND parent_id IS NULL
		  AND (? = '' OR author_id = ?) AND deleted_at IS NULL
		FOR UPDATE`
	comment, err := scanDocumentComment(transaction.QueryRowContext(ctx, selectQuery, commentID, documentID, actorID, actorID))
	if errors.Is(err, sql.ErrNoRows) {
		return documentComment{}, false, nil
	}
	if err != nil {
		return documentComment{}, false, fmt.Errorf("select comment for resolve: %w", err)
	}

	if comment.Status != "resolved" {
		const updateQuery = `
			UPDATE document_comments
			SET status = 'resolved', resolved_at = ?, updated_at = ?
			WHERE id = ? AND document_id = ?`
		if _, err := transaction.ExecContext(ctx, updateQuery, resolvedAt, resolvedAt, commentID, documentID); err != nil {
			return documentComment{}, false, fmt.Errorf("resolve comment: %w", err)
		}
		comment.Status = "resolved"
		comment.ResolvedAt = &resolvedAtValue
		comment.UpdatedAt = resolvedAtValue
	}
	if err := transaction.Commit(); err != nil {
		return documentComment{}, false, fmt.Errorf("commit resolved comment: %w", err)
	}
	return comment, true, nil
}

type commentScanner interface {
	Scan(dest ...any) error
}

func scanDocumentComment(scanner commentScanner) (documentComment, error) {
	var comment documentComment
	var createdAt time.Time
	var updatedAt time.Time
	var parentID sql.NullString
	var authorID sql.NullString
	var resolvedAt sql.NullTime
	err := scanner.Scan(
		&comment.ID, &comment.DocumentID, &parentID, &comment.BlockID, &authorID, &comment.Author,
		&comment.AuthorRegion, &comment.Quote, &comment.Body, &comment.Status, &createdAt, &updatedAt, &resolvedAt,
	)
	if err != nil {
		return documentComment{}, err
	}
	comment.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	comment.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	if parentID.Valid {
		comment.ParentID = &parentID.String
	}
	if authorID.Valid {
		comment.AuthorID = authorID.String
	}
	comment.Replies = []documentComment{}
	if resolvedAt.Valid {
		value := resolvedAt.Time.UTC().Format(time.RFC3339)
		comment.ResolvedAt = &value
	}
	return comment, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func openMySQLDatabase(ctx context.Context, databaseURL string) (*sql.DB, error) {
	dsn, err := mysqlDSN(databaseURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

func mysqlDSN(databaseURL string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", errors.New("DATABASE_URL is not a valid URL")
	}
	if parsed.Scheme != "mysql" || parsed.User == nil || parsed.Host == "" {
		return "", errors.New("DATABASE_URL must use mysql://user:password@host/database")
	}
	password, _ := parsed.User.Password()
	databaseName := parsed.Path
	if len(databaseName) > 0 && databaseName[0] == '/' {
		databaseName = databaseName[1:]
	}
	if databaseName == "" {
		return "", errors.New("DATABASE_URL must include a database name")
	}

	config := mysql.NewConfig()
	config.User = parsed.User.Username()
	config.Passwd = password
	config.Net = "tcp"
	config.Addr = parsed.Host
	config.DBName = databaseName
	config.ParseTime = true
	config.ClientFoundRows = true
	config.Loc = time.UTC
	config.Collation = "utf8mb4_unicode_ci"
	config.Params = map[string]string{"time_zone": "'+00:00'"}
	return config.FormatDSN(), nil
}

// CountAll 返回未删除评论（含回复）总数，供管理概览统计。
// 通过 commentCounter 断言可选调用，不属于 commentRepository 接口。
func (repository *mysqlCommentRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	if err := repository.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM document_comments WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count comments: %w", err)
	}
	return count, nil
}

var _ commentRepository = (*mysqlCommentRepository)(nil)
