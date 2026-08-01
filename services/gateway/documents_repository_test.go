package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSeedDocumentRepository(t *testing.T) {
	repository := seedDocumentRepository{documents: seedDocuments}
	ctx := context.Background()

	nodes, found, err := repository.List(ctx, "atlas-agent")
	if err != nil || !found || len(nodes) != 1 || nodes[0].Slug != "quick-start" {
		t.Fatalf("unexpected Atlas document list: found=%v err=%v nodes=%#v", found, err, nodes)
	}

	document, projectFound, documentFound, err := repository.Get(ctx, "atlas-agent", "quick-start")
	if err != nil || !projectFound || !documentFound || document.ID != "doc-atlas-quick-start" {
		t.Fatalf("unexpected document result: project=%v document=%v err=%v data=%#v",
			projectFound, documentFound, err, document)
	}

	_, projectFound, documentFound, _ = repository.Get(ctx, "missing", "quick-start")
	if projectFound || documentFound {
		t.Fatalf("missing project result = (%v, %v), want false, false", projectFound, documentFound)
	}

	_, projectFound, documentFound, _ = repository.Get(ctx, "atlas-agent", "missing")
	if !projectFound || documentFound {
		t.Fatalf("missing document result = (%v, %v), want true, false", projectFound, documentFound)
	}
}

// TestManagedDocumentRepositoryServesPublishedMarkdown 覆盖阅读页读取真实项目正文：
// 已发布项目必须返回作者写的 Markdown，而不是种子演示内容。
func TestManagedDocumentRepositoryServesPublishedMarkdown(t *testing.T) {
	original := managedProjectRepositoryStore
	repository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = repository
	t.Cleanup(func() { managedProjectRepositoryStore = original })

	ctx := context.Background()
	markdown := "# 真实项目标题\n\n这是作者写的第一段正文。\n\n## 使用方式\n\n```bash\nnpm install real-project\n```\n"
	project, err := repository.Create(ctx, "owner-1", managedProjectInput{
		Slug: "real-project", Name: "Real Project", Summary: "用于验证真实正文读取的项目摘要",
		Description: markdown, Category: "Coding Agent", Tags: []string{"Agent"},
		TechStack: []string{"Go"}, License: "MIT", CurrentVersion: "1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := repository.Submit(ctx, "owner-1", project.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Review(ctx, project.ID, "admin", "approve", "", now); err != nil {
		t.Fatal(err)
	}

	store := managedDocumentRepository{fallback: seedDocumentRepository{documents: seedDocuments}}

	nodes, found, err := store.List(ctx, "real-project")
	if err != nil || !found || len(nodes) != 1 {
		t.Fatalf("published document list: found=%v err=%v nodes=%#v", found, err, nodes)
	}
	if nodes[0].Title != "真实项目标题" || nodes[0].Slug != publishedDocumentSlug {
		t.Fatalf("unexpected document node: %#v", nodes[0])
	}

	document, projectFound, documentFound, err := store.Get(ctx, "real-project", publishedDocumentSlug)
	if err != nil || !projectFound || !documentFound {
		t.Fatalf("published document get: project=%v document=%v err=%v", projectFound, documentFound, err)
	}
	if document.Markdown != markdown {
		t.Fatalf("document markdown is not the author content: %q", document.Markdown)
	}
	if document.Version != "1.2.3" || document.Title != "真实项目标题" {
		t.Fatalf("unexpected document metadata: %#v", document)
	}
	// 大纲与稳定块必须来自真实正文，供页内导航和选区评论使用。
	if len(document.Outline) != 2 || document.Outline[1].Title != "使用方式" {
		t.Fatalf("unexpected outline: %#v", document.Outline)
	}
	if len(document.Blocks) != 2 {
		t.Fatalf("unexpected blocks: %#v", document.Blocks)
	}
	if document.Blocks[0].Type != "paragraph" || document.Blocks[0].Text != "这是作者写的第一段正文。" {
		t.Fatalf("unexpected first block: %#v", document.Blocks[0])
	}
	if document.Blocks[1].Type != "code" || !strings.Contains(document.Blocks[1].Text, "npm install real-project") {
		t.Fatalf("unexpected code block: %#v", document.Blocks[1])
	}

	// 未知文档 slug 属于该项目但不存在。
	if _, projectFound, documentFound, _ := store.Get(ctx, "real-project", "missing"); !projectFound || documentFound {
		t.Fatalf("unknown slug result = (%v, %v), want true, false", projectFound, documentFound)
	}

	// 未发布项目和非托管项目仍回退到种子仓库。
	if _, found, _ := store.List(ctx, "atlas-agent"); !found {
		t.Fatal("seed project should still be served by fallback")
	}
}

// TestManagedProjectDocumentFallbacksForEmptyMarkdown 覆盖正文缺少标题时的降级行为。
func TestManagedProjectDocumentFallbacksForEmptyMarkdown(t *testing.T) {
	document := managedProjectDocument(managedProject{
		ID: "project-1", Name: "无标题项目", Description: "只有一段没有标题的正文。",
		UpdatedAt: time.Now().UTC(),
	})
	if document.Title != "无标题项目" {
		t.Fatalf("title should fall back to project name: %q", document.Title)
	}
	if document.Version != "—" {
		t.Fatalf("version should fall back to placeholder: %q", document.Version)
	}
	if len(document.Blocks) != 1 || document.Blocks[0].Text != "只有一段没有标题的正文。" {
		t.Fatalf("unexpected blocks: %#v", document.Blocks)
	}
}
