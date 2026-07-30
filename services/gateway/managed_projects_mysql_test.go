package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMySQLManagedProjectIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := openMySQLDatabase(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	userID := "user-project-test-" + newRequestID()
	email := userID + "@example.com"
	_, err = db.ExecContext(ctx, `INSERT INTO users
		(id,email,display_name,password_hash) VALUES (?,?,?,?)`,
		userID, email, "项目集成测试", "test-only")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=?`, userID)
	})
	repository := newMySQLManagedProjectRepository(db)
	input := managedProjectInput{
		Slug: "mysql-project-" + newRequestID(), Name: "MySQL Project",
		Summary:     "用于真实数据库项目审核流程的集成测试项目",
		Description: "这是一段足够长的项目说明，用来覆盖创建、提交、审核以及公开读取流程。",
		Category:    "Testing", Tags: []string{"MySQL"}, TechStack: []string{"Go"},
		License: "MIT", CurrentVersion: "0.1.0",
	}
	project, err := repository.Create(ctx, userID, input)
	if err != nil {
		t.Fatal(err)
	}
	project, err = repository.Submit(ctx, userID, project.ID, time.Now().UTC())
	if err != nil || project.Status != "pending_review" {
		t.Fatalf("submit: %#v %v", project, err)
	}
	project, err = repository.Review(ctx, project.ID, userID, "approve", "通过", time.Now().UTC())
	if err != nil || project.Status != "published" {
		t.Fatalf("approve: %#v %v", project, err)
	}
	published, found, err := repository.FindPublishedBySlug(ctx, input.Slug)
	if err != nil || !found || published.ID != project.ID {
		t.Fatalf("find published: %#v %v %v", published, found, err)
	}
	var events int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_review_events WHERE project_id=?`, project.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("review event count = %d", events)
	}
}
