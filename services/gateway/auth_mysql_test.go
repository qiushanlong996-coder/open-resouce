package main

import (
	"context"
	"testing"
	"time"
)

func TestMySQLAuthRepositoryIntegration(t *testing.T) {
	database := requireTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userID := "user-integration-" + newRequestID()
	email := userID + "@example.com"
	sessionID := "session-integration-" + newRequestID()
	tokenHash := sessionTokenHash("integration-" + newRequestID())
	t.Cleanup(func() {
		if _, err := database.Exec(`DELETE FROM auth_sessions WHERE id = ?`, sessionID); err != nil {
			t.Errorf("clean up integration session: %v", err)
		}
		if _, err := database.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
			t.Errorf("clean up integration user: %v", err)
		}
	})

	repository := newMySQLAuthRepository(database)
	passwordHash, err := hashPassword("integration-password")
	if err != nil {
		t.Fatal(err)
	}
	user := authUser{ID: userID, Email: email, DisplayName: "集成用户", PasswordHash: passwordHash}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	foundUser, found, err := repository.FindUserByEmail(ctx, email)
	if err != nil || !found || foundUser.ID != userID || !verifyPassword(foundUser.PasswordHash, "integration-password") {
		t.Fatalf("find user: found=%v err=%v user=%#v", found, err, foundUser)
	}
	updatedProfile, found, err := repository.UpdateDisplayName(ctx, userID, "更新后的昵称")
	if err != nil || !found || updatedProfile.DisplayName != "更新后的昵称" {
		t.Fatalf("update display name: found=%v err=%v user=%#v", found, err, updatedProfile)
	}
	if err := repository.CreateSession(ctx, authSession{
		ID: sessionID, UserID: userID, TokenHash: tokenHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionUser, found, err := repository.FindUserByTokenHash(ctx, tokenHash, time.Now().UTC())
	if err != nil || !found || sessionUser.ID != userID {
		t.Fatalf("find session: found=%v err=%v user=%#v", found, err, sessionUser)
	}
	if err := repository.DeleteSession(ctx, tokenHash); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, tokenHash, time.Now().UTC()); err != nil || found {
		t.Fatalf("deleted session still found: found=%v err=%v", found, err)
	}
	secondTokenHash := sessionTokenHash("integration-second-" + newRequestID())
	if err := repository.CreateSession(ctx, authSession{
		ID: "session-integration-" + newRequestID(), UserID: userID,
		TokenHash: secondTokenHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create second session: %v", err)
	}
	sessions, err := repository.ListUserSessions(ctx, userID, time.Now().UTC())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list sessions: count=%d err=%v", len(sessions), err)
	}
	deleted, found, err := repository.DeleteUserSession(ctx, userID, sessions[0].ID)
	if err != nil || !found || deleted.TokenHash != secondTokenHash {
		t.Fatalf("delete individual session: found=%v err=%v session=%#v", found, err, deleted)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, secondTokenHash, time.Now().UTC()); err != nil || found {
		t.Fatalf("individually revoked session still found: found=%v err=%v", found, err)
	}
	thirdTokenHash := sessionTokenHash("integration-third-" + newRequestID())
	if err := repository.CreateSession(ctx, authSession{
		ID: "session-integration-" + newRequestID(), UserID: userID,
		TokenHash: thirdTokenHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create third session: %v", err)
	}
	fourthTokenHash := sessionTokenHash("integration-fourth-" + newRequestID())
	if err := repository.CreateSession(ctx, authSession{
		ID: "session-integration-" + newRequestID(), UserID: userID,
		TokenHash: fourthTokenHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create fourth session: %v", err)
	}
	updatedPasswordHash, err := hashPassword("integration-password-updated")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdatePasswordAndDeleteOtherSessions(
		ctx, userID, updatedPasswordHash, thirdTokenHash,
	); err != nil {
		t.Fatalf("update password: %v", err)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, thirdTokenHash, time.Now().UTC()); err != nil || !found {
		t.Fatalf("current session was revoked after password update: found=%v err=%v", found, err)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, fourthTokenHash, time.Now().UTC()); err != nil || found {
		t.Fatalf("other session remained after password update: found=%v err=%v", found, err)
	}
	updatedUser, found, err := repository.FindUserByEmail(ctx, email)
	if err != nil || !found || !verifyPassword(updatedUser.PasswordHash, "integration-password-updated") {
		t.Fatalf("updated password was not persisted: found=%v err=%v", found, err)
	}
	resetRawToken := "integration-reset-" + newRequestID()
	resetToken := passwordResetToken{
		ID: "reset-integration-" + newRequestID(), TokenHash: sessionTokenHash(resetRawToken),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	resetUser, found, err := repository.CreatePasswordResetToken(ctx, email, resetToken)
	if err != nil || !found || resetUser.ID != userID {
		t.Fatalf("create password reset: found=%v err=%v user=%#v", found, err, resetUser)
	}
	resetPasswordHash, err := hashPassword("integration-password-reset")
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := repository.ConsumePasswordResetToken(
		ctx, sessionTokenHash(resetRawToken), time.Now().UTC(), resetPasswordHash,
	)
	if err != nil || !consumed {
		t.Fatalf("consume password reset: consumed=%v err=%v", consumed, err)
	}
	if consumedAgain, err := repository.ConsumePasswordResetToken(
		ctx, sessionTokenHash(resetRawToken), time.Now().UTC(), resetPasswordHash,
	); err != nil || consumedAgain {
		t.Fatalf("password reset token was reusable: consumed=%v err=%v", consumedAgain, err)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, thirdTokenHash, time.Now().UTC()); err != nil || found {
		t.Fatalf("session remained after password reset: found=%v err=%v", found, err)
	}
	resetUser, found, err = repository.FindUserByEmail(ctx, email)
	if err != nil || !found || !verifyPassword(resetUser.PasswordHash, "integration-password-reset") {
		t.Fatalf("reset password was not persisted: found=%v err=%v", found, err)
	}
	if err := repository.DeleteUserSessions(ctx, userID); err != nil {
		t.Fatalf("delete all sessions: %v", err)
	}
	if _, found, err := repository.FindUserByTokenHash(ctx, thirdTokenHash, time.Now().UTC()); err != nil || found {
		t.Fatalf("revoked session still found: found=%v err=%v", found, err)
	}
}
