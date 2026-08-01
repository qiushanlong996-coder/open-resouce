package main

import (
	"context"
	"testing"
	"time"
)

func TestLevelForExperience(t *testing.T) {
	cases := []struct {
		experience int
		want       int
	}{
		{0, 1}, {99, 1},
		{100, 2}, {299, 2},
		{300, 3}, {699, 3},
		{700, 4}, {1499, 4},
		{1500, 5}, {2999, 5},
		{3000, 6}, {99999, 6},
		{-10, 1}, // 经验不应为负，但要稳健兜底为最低级
	}
	for _, testCase := range cases {
		if got := levelForExperience(testCase.experience); got != testCase.want {
			t.Errorf("levelForExperience(%d) = %d, want %d", testCase.experience, got, testCase.want)
		}
	}
}

func TestLevelForUserAdminOverride(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "boss@example.com")
	// 管理员即使 0 经验也应为最高级。
	if got := levelForUser("boss@example.com", 0); got != maxUserLevel {
		t.Errorf("admin level = %d, want %d", got, maxUserLevel)
	}
	// 大小写不敏感。
	if got := levelForUser("BOSS@example.com", 0); got != maxUserLevel {
		t.Errorf("admin level (mixed case) = %d, want %d", got, maxUserLevel)
	}
	// 普通用户仍按经验计算。
	if got := levelForUser("user@example.com", 300); got != 3 {
		t.Errorf("regular level = %d, want 3", got)
	}
}

func TestMemoryAddExperienceIsIdempotent(t *testing.T) {
	repository := newMemoryAuthRepository()
	ctx := context.Background()
	const userID = "user-1"
	if err := repository.CreateUser(ctx, authUser{ID: userID, Email: "u@example.com", DisplayName: "U"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 首次记入。
	awarded, err := repository.AddExperience(ctx, userID, xpActionComment, "comment-1", xpComment)
	if err != nil {
		t.Fatalf("first award: %v", err)
	}
	if !awarded {
		t.Fatal("first award should be recorded")
	}
	// 同一 (user, action, source_key) 重复动作不再加经验。
	awarded, err = repository.AddExperience(ctx, userID, xpActionComment, "comment-1", xpComment)
	if err != nil {
		t.Fatalf("second award: %v", err)
	}
	if awarded {
		t.Fatal("duplicate award must be ignored")
	}
	// 不同目标应再次记入。
	if _, err := repository.AddExperience(ctx, userID, xpActionComment, "comment-2", xpComment); err != nil {
		t.Fatalf("third award: %v", err)
	}

	user, found, err := repository.FindUserByEmail(ctx, "u@example.com")
	if err != nil || !found {
		t.Fatalf("find user: err=%v found=%v", err, found)
	}
	if user.Experience != xpComment*2 {
		t.Fatalf("experience = %d, want %d (two distinct comments, one duplicate ignored)",
			user.Experience, xpComment*2)
	}
}

func TestMemoryLevelsByUserIDs(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "admin@example.com")
	repository := newMemoryAuthRepository()
	ctx := context.Background()
	users := []authUser{
		{ID: "u-newbie", Email: "newbie@example.com", DisplayName: "N"},
		{ID: "u-mid", Email: "mid@example.com", DisplayName: "M", Experience: 700},
		{ID: "u-admin", Email: "admin@example.com", DisplayName: "A"},
	}
	for _, user := range users {
		if err := repository.CreateUser(ctx, user); err != nil {
			t.Fatalf("create %s: %v", user.ID, err)
		}
	}

	levels, err := repository.LevelsByUserIDs(ctx, []string{"u-newbie", "u-mid", "u-admin", "u-missing"})
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	if levels["u-newbie"] != 1 {
		t.Errorf("newbie level = %d, want 1", levels["u-newbie"])
	}
	if levels["u-mid"] != 4 {
		t.Errorf("mid level = %d, want 4", levels["u-mid"])
	}
	if levels["u-admin"] != maxUserLevel {
		t.Errorf("admin level = %d, want %d", levels["u-admin"], maxUserLevel)
	}
	if _, present := levels["u-missing"]; present {
		t.Error("missing user must not appear in levels map")
	}
}

func TestPublicAuthUserComputesLevel(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "admin@example.com")
	view := publicAuthUser(authUser{ID: "u", Email: "u@example.com", Experience: 1500, PasswordHash: "secret"})
	if view.Level != 5 {
		t.Errorf("level = %d, want 5", view.Level)
	}
	if view.PasswordHash != "" {
		t.Error("password hash must not leak in public view")
	}
	admin := publicAuthUser(authUser{ID: "a", Email: "admin@example.com", Experience: 0})
	if admin.Level != maxUserLevel || !admin.IsAdmin {
		t.Errorf("admin view = level %d admin %v, want level %d admin true", admin.Level, admin.IsAdmin, maxUserLevel)
	}
}

func TestMySQLExperienceIntegration(t *testing.T) {
	database := requireTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID := "user-xp-" + newRequestID()
	email := userID + "@example.com"
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, email, "经验集成用户", "integration-only"); err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	t.Cleanup(func() {
		// experience_events 有 user 外键级联，删用户即清账本。
		if _, err := database.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
			t.Errorf("clean up integration user: %v", err)
		}
	})

	repository := newMySQLAuthRepository(database)

	awarded, err := repository.AddExperience(ctx, userID, xpActionPost, "project-1", xpPost)
	if err != nil || !awarded {
		t.Fatalf("first award: awarded=%v err=%v", awarded, err)
	}
	// 同 (user, action, source_key) 幂等，不重复加。
	awarded, err = repository.AddExperience(ctx, userID, xpActionPost, "project-1", xpPost)
	if err != nil || awarded {
		t.Fatalf("duplicate award should be ignored: awarded=%v err=%v", awarded, err)
	}
	if _, err := repository.AddExperience(ctx, userID, xpActionComment, "comment-1", xpComment); err != nil {
		t.Fatalf("comment award: %v", err)
	}

	user, found, err := repository.FindUserByEmail(ctx, email)
	if err != nil || !found {
		t.Fatalf("find user: err=%v found=%v", err, found)
	}
	if user.Experience != xpPost+xpComment {
		t.Fatalf("experience = %d, want %d", user.Experience, xpPost+xpComment)
	}

	levels, err := repository.LevelsByUserIDs(ctx, []string{userID})
	if err != nil {
		t.Fatalf("levels: %v", err)
	}
	if levels[userID] != levelForExperience(xpPost+xpComment) {
		t.Fatalf("level = %d, want %d", levels[userID], levelForExperience(xpPost+xpComment))
	}
}
