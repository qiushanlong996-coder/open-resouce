package main

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"
)

// setupRevisionIntegrationFixtures 造出真实的用户、已发布项目和文档。
// 版本表对 project_documents 与 managed_projects 都有外键，
// project_documents.created_by 又对 users 有外键，所以不能用凭空编的 ID。
func setupRevisionIntegrationFixtures(
	t *testing.T, ctx context.Context, db *sql.DB, suffix string,
) (userID, projectID string, document projectDocument) {
	t.Helper()
	userID = "user-revision-test-" + suffix
	_, err := db.ExecContext(ctx, `INSERT INTO users
		(id,email,display_name,password_hash) VALUES (?,?,?,?)`,
		userID, userID+"@example.com", "版本历史集成测试", "test-only")
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}

	projects := newMySQLManagedProjectRepository(db)
	project, err := projects.Create(ctx, userID, managedProjectInput{
		Slug: "revision-project-" + suffix, Name: "版本历史集成项目",
		Summary:     "用于真实数据库版本历史仓库的集成测试项目",
		Description: "这是一段足够长的项目说明，用来满足创建项目时的字段校验要求。",
		Category:    "Testing", Tags: []string{"MySQL"}, TechStack: []string{"Go"},
		License: "MIT", CurrentVersion: "0.1.0",
	})
	if err != nil {
		t.Fatalf("create integration project: %v", err)
	}
	projectID = project.ID

	documents := newMySQLProjectDocumentRepository(db)
	document, err = documents.Create(ctx, projectID, userID, projectDocumentInput{
		Slug: "revision-doc-" + suffix, Title: "版本历史集成测试", Markdown: "# 初稿\n\n第一版内容。",
	})
	if err != nil {
		t.Fatalf("create integration document: %v", err)
	}

	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// 版本表与文档表的外键都是 ON DELETE CASCADE，
		// 删项目会连带清掉文档和它们的历史版本。
		for _, statement := range []struct {
			query string
			arg   string
		}{
			{`DELETE FROM project_document_revisions WHERE project_id=?`, projectID},
			{`DELETE FROM project_documents WHERE project_id=?`, projectID},
			{`DELETE FROM project_review_events WHERE project_id=?`, projectID},
			{`DELETE FROM managed_projects WHERE id=?`, projectID},
			{`DELETE FROM users WHERE id=?`, userID},
		} {
			if _, err := db.ExecContext(cleanup, statement.query, statement.arg); err != nil {
				t.Errorf("clean up %s: %v", statement.query, err)
			}
		}
	})
	return userID, projectID, document
}

// TestMySQLDocumentRevisionRepositoryIntegration 用真实 MySQL 验证版本历史仓库。
//
// 这个仓库的 SQL 里有几处只有真跑一次才会暴露问题的地方：CHAR_LENGTH 取字符数
// （不是字节数）、修剪用的 LIMIT 1 OFFSET n、restored_from 的 NULL 处理，
// 以及 version 这个与 VERSION() 同名的列名。内存实现一个都测不到。
func TestMySQLDocumentRevisionRepositoryIntegration(t *testing.T) {
	db := requireTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := newRequestID()
	userID, projectID, document := setupRevisionIntegrationFixtures(t, ctx, db, suffix)
	documents := newMySQLProjectDocumentRepository(db)
	revisions := newMySQLDocumentRevisionRepository(db)

	if document.Version != 1 {
		t.Fatalf("created document version = %d, want 1", document.Version)
	}
	if document.UpdatedBy != userID {
		t.Fatalf("created document updated_by = %q, want %q", document.UpdatedBy, userID)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	first, err := revisions.Append(ctx, documentRevision{
		ID: "revision-" + suffix + "-1", DocumentID: document.ID, ProjectID: projectID,
		Version: 1, Slug: document.Slug, Title: document.Title,
		Markdown: "# 初稿\n\n第一版内容。", AuthorID: userID, Source: revisionSourceCreate,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("append first revision: %v", err)
	}

	latest, found, err := revisions.Latest(ctx, document.ID)
	if err != nil || !found || latest.Version != 1 {
		t.Fatalf("latest after first: found=%v err=%v revision=%#v", found, err, latest)
	}
	if latest.AuthorID != userID {
		t.Fatalf("latest author = %q, want %q", latest.AuthorID, userID)
	}

	// Amend 依赖 clientFoundRows：正文没变也不该被误判成记录不存在。
	amendedMarkdown := "# 初稿\n\n第一版内容，补了一句。"
	amended, err := revisions.Amend(ctx, first.ID, document.Slug, document.Title,
		amendedMarkdown, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("amend revision: %v", err)
	}
	if amended.Markdown != amendedMarkdown {
		t.Fatalf("amended markdown = %q", amended.Markdown)
	}
	if _, err := revisions.Amend(ctx, first.ID, document.Slug, document.Title,
		amendedMarkdown, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("amend with identical markdown should succeed: %v", err)
	}
	if _, err := revisions.Amend(ctx, "revision-does-not-exist", "s", "t", "m", now); err == nil {
		t.Fatal("amend on missing revision should fail")
	}

	// 回滚版本：restored_from 有值，扫描要能还原成非空指针。
	restoredFrom := 1
	restoreMarkdown := "# 回滚\n\n还原后的内容。"
	if _, err := revisions.Append(ctx, documentRevision{
		ID: "revision-" + suffix + "-2", DocumentID: document.ID, ProjectID: projectID,
		Version: 2, Slug: document.Slug, Title: document.Title,
		Markdown: restoreMarkdown, AuthorID: userID, Source: revisionSourceRestore,
		RestoredFrom: &restoredFrom, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("append restore revision: %v", err)
	}

	list, err := revisions.List(ctx, document.ID, documentRevisionListLimit)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(list) != 2 || list[0].Version != 2 || list[1].Version != 1 {
		t.Fatalf("list order wrong: %#v", list)
	}
	// 列表不回传正文，否则 50 篇文章会一次全拉回前端。
	if list[0].Markdown != "" {
		t.Fatalf("list should not carry markdown, got %q", list[0].Markdown)
	}
	if list[0].RestoredFrom == nil || *list[0].RestoredFrom != 1 {
		t.Fatalf("restored_from not scanned: %#v", list[0].RestoredFrom)
	}
	if list[1].RestoredFrom != nil {
		t.Fatal("non-restore revision should have nil restored_from")
	}
	// CHAR_LENGTH 数的是字符不是字节，中文正文必须与 Go 的 rune 数一致。
	if wantChars := len([]rune(restoreMarkdown)); list[0].CharCount != wantChars {
		t.Fatalf("char count = %d, want %d（CHAR_LENGTH 必须按字符计）", list[0].CharCount, wantChars)
	}

	fetched, found, err := revisions.Find(ctx, document.ID, 1)
	if err != nil || !found {
		t.Fatalf("find revision 1: found=%v err=%v", found, err)
	}
	if fetched.Markdown != amendedMarkdown {
		t.Fatalf("found markdown = %q, want %q", fetched.Markdown, amendedMarkdown)
	}
	if _, found, err = revisions.Find(ctx, document.ID, 99); err != nil || found {
		t.Fatalf("missing revision should not be found: found=%v err=%v", found, err)
	}

	// 同一文档不能出现两个 v2，唯一键必须真的生效。
	if _, err := revisions.Append(ctx, documentRevision{
		ID: "revision-" + suffix + "-dup", DocumentID: document.ID, ProjectID: projectID,
		Version: 2, Slug: document.Slug, Title: document.Title,
		AuthorID: userID, Source: revisionSourceEdit, CreatedAt: now, UpdatedAt: now,
	}); err == nil {
		t.Fatal("duplicate version should violate the unique key")
	}

	// 反范式回填：版本号与最后更新人要落到文档行上，且不能顺手改掉 updated_at。
	before, _, err := documents.FindBySlug(ctx, projectID, document.Slug)
	if err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if err := documents.ApplyRevisionMeta(ctx, projectID, document.ID, 2, userID); err != nil {
		t.Fatalf("apply revision meta: %v", err)
	}
	reloaded, _, err := documents.FindBySlug(ctx, projectID, document.Slug)
	if err != nil {
		t.Fatalf("reload document after meta: %v", err)
	}
	if reloaded.Version != 2 || reloaded.UpdatedBy != userID {
		t.Fatalf("document meta = v%d by %q", reloaded.Version, reloaded.UpdatedBy)
	}
	if !reloaded.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("回填版本号不应改动 updated_at：%v -> %v", before.UpdatedAt, reloaded.UpdatedAt)
	}
	// 重复写入相同的值也必须成功：clientFoundRows 下匹配到行就算成功。
	if err := documents.ApplyRevisionMeta(ctx, projectID, document.ID, 2, userID); err != nil {
		t.Fatalf("repeated apply with identical values should succeed: %v", err)
	}
	if err := documents.ApplyRevisionMeta(ctx, projectID, "document-missing", 2, userID); err == nil {
		t.Fatal("apply revision meta on missing document should fail")
	}

	// Prune 的 LIMIT 1 OFFSET n 边界只有真跑一次才知道对不对。
	for version := 3; version <= 8; version++ {
		if _, err := revisions.Append(ctx, documentRevision{
			ID: "revision-" + suffix + "-" + strconv.Itoa(version), DocumentID: document.ID,
			ProjectID: projectID, Version: version, Slug: document.Slug, Title: document.Title,
			Markdown: "第 " + strconv.Itoa(version) + " 版", AuthorID: userID,
			Source: revisionSourceEdit, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("append revision %d: %v", version, err)
		}
	}
	if err := revisions.Prune(ctx, document.ID, 3); err != nil {
		t.Fatalf("prune revisions: %v", err)
	}
	remaining, err := revisions.List(ctx, document.ID, documentRevisionListLimit)
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining revisions = %d, want 3", len(remaining))
	}
	if remaining[0].Version != 8 || remaining[2].Version != 6 {
		t.Fatalf("prune kept the wrong versions: %#v", remaining)
	}
	// 条数没超过上限时不该删任何东西。
	if err := revisions.Prune(ctx, document.ID, 10); err != nil {
		t.Fatalf("prune below limit: %v", err)
	}
	if remaining, err = revisions.List(ctx, document.ID, documentRevisionListLimit); err != nil ||
		len(remaining) != 3 {
		t.Fatalf("prune below limit should keep all: err=%v count=%d", err, len(remaining))
	}

	// 删文档时历史版本必须跟着级联删除，不能留下孤儿记录。
	if err := documents.Delete(ctx, projectID, document.ID, time.Now().UTC()); err != nil {
		t.Fatalf("soft delete document: %v", err)
	}
	// 文档是软删除，所以历史版本仍在——回滚入口靠权限而不是靠删历史来关闭。
	if after, err := revisions.List(ctx, document.ID, documentRevisionListLimit); err != nil ||
		len(after) != 3 {
		t.Fatalf("软删除文档不应清掉历史版本: err=%v count=%d", err, len(after))
	}
}
