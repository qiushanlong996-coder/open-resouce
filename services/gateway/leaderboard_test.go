package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLeaderboardHandler(t *testing.T) {
	originalAuth := authRepositoryStore
	originalLimiter := authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore = originalAuth
		authRateLimiter = originalLimiter
	})

	_, first := registerTestUser(t, "leaderboard-a@example.com", "甲一")
	_, second := registerTestUser(t, "leaderboard-b@example.com", "乙二")
	_, third := registerTestUser(t, "leaderboard-c@example.com", "丙三")
	if _, err := authRepositoryStore.AddExperience(context.Background(), first.ID, "test", "lb-1", 80); err != nil {
		t.Fatal(err)
	}
	if _, err := authRepositoryStore.AddExperience(context.Background(), second.ID, "test", "lb-2", 40); err != nil {
		t.Fatal(err)
	}
	if _, err := authRepositoryStore.AddExperience(context.Background(), third.ID, "test", "lb-3", 120); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"display_name":"丙三"`) {
		t.Fatalf("leaderboard should put 丙 first: %s", body)
	}
	thirdIndex := strings.Index(body, `"display_name":"丙三"`)
	firstIndex := strings.Index(body, `"display_name":"甲一"`)
	if thirdIndex < 0 || firstIndex < 0 || thirdIndex > firstIndex {
		t.Fatalf("order wrong: %s", body)
	}
}

func TestLeaderboardHandlerValidatesLimit(t *testing.T) {
	for _, limit := range []string{"0", "-1", "51", "abc"} {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?limit="+limit, nil))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("limit=%q status = %d, want 422", limit, response.Code)
		}
	}
}
