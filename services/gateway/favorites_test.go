package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFavoriteLifecycle(t *testing.T) {
	originalAuth := authRepositoryStore
	originalFavorites := favoriteRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	favoriteRepositoryStore = newMemoryFavoriteRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		favoriteRepositoryStore = originalFavorites
		authRateLimiter = originalLimiter
	})
	cookie, _ := registerTestUser(t, "favorite@example.com", "收藏用户")

	setRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/favorite", nil)
	setRequest.AddCookie(cookie)
	setResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(setResponse, setRequest)
	if setResponse.Code != http.StatusNoContent {
		t.Fatalf("set favorite status = %d: %s", setResponse.Code, setResponse.Body)
	}

	// Repeating the operation is intentionally idempotent.
	secondSetResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(secondSetResponse, setRequest.Clone(setRequest.Context()))
	if secondSetResponse.Code != http.StatusNoContent {
		t.Fatalf("repeat set favorite status = %d", secondSetResponse.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	listRequest.AddCookie(cookie)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	var listed favoriteListResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listResponse.Code != http.StatusOK || len(listed.Data) != 1 || listed.Data[0] != "atlas" {
		t.Fatalf("favorites response = %d %#v", listResponse.Code, listed.Data)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/atlas-agent/favorite", nil)
	deleteRequest.AddCookie(cookie)
	deleteResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete favorite status = %d", deleteResponse.Code)
	}

	listAfterDelete := httptest.NewRecorder()
	newHandler().ServeHTTP(listAfterDelete, listRequest)
	if err := json.Unmarshal(listAfterDelete.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 0 {
		t.Fatalf("favorites remained after delete: %#v", listed.Data)
	}
}

func TestFavoritesRequireAuthenticationAndKnownProject(t *testing.T) {
	originalAuth := authRepositoryStore
	originalFavorites := favoriteRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	favoriteRepositoryStore = newMemoryFavoriteRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		favoriteRepositoryStore = originalFavorites
		authRateLimiter = originalLimiter
	})

	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list status = %d", response.Code)
	}

	cookie, _ := registerTestUser(t, "favorite-errors@example.com", "收藏校验用户")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/not-found/favorite", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown project status = %d", response.Code)
	}
}

func TestMySQLFavoriteRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("MYSQL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MYSQL_TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := openMySQLDatabase(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	// 关闭必须通过 t.Cleanup 注册：defer 早于 t.Cleanup 执行，会让后面的清理语句在连接关闭后静默失败。
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	userID := "user-favorite-" + newRequestID()
	_, err = database.ExecContext(
		ctx,
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.com", "收藏集成用户", "integration-only",
	)
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := database.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
			t.Errorf("clean up integration user: %v", err)
		}
	})

	repository := newMySQLFavoriteRepository(database)
	if err := repository.SetFavorite(ctx, userID, "atlas", true); err != nil {
		t.Fatalf("set favorite: %v", err)
	}
	if err := repository.SetFavorite(ctx, userID, "atlas", true); err != nil {
		t.Fatalf("repeat set favorite: %v", err)
	}
	projectIDs, err := repository.ListProjectIDs(ctx, userID)
	if err != nil || len(projectIDs) != 1 || projectIDs[0] != "atlas" {
		t.Fatalf("list favorites: %#v err=%v", projectIDs, err)
	}
	if err := repository.SetFavorite(ctx, userID, "atlas", false); err != nil {
		t.Fatalf("delete favorite: %v", err)
	}
	projectIDs, err = repository.ListProjectIDs(ctx, userID)
	if err != nil || len(projectIDs) != 0 {
		t.Fatalf("favorites after delete: %#v err=%v", projectIDs, err)
	}
}
