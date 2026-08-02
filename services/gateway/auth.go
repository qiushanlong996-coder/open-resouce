package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	sessionCookieName     = "open_resouce_session"
	sessionLifetime       = 30 * 24 * time.Hour
	passwordResetLifetime = 30 * time.Minute
	passwordRounds        = 210000
)

var errEmailExists = errors.New("email already exists")

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

type fixedWindowLimiter struct {
	sync.Mutex
	entries map[string]rateLimitEntry
}

type authAttemptLimiter interface {
	Allow(context.Context, string, int, time.Duration, time.Time) (bool, time.Duration, error)
}

func newFixedWindowLimiter() *fixedWindowLimiter {
	return &fixedWindowLimiter{entries: make(map[string]rateLimitEntry)}
}

func (limiter *fixedWindowLimiter) Allow(
	_ context.Context, key string, limit int, window time.Duration, now time.Time,
) (bool, time.Duration, error) {
	limiter.Lock()
	defer limiter.Unlock()
	entry := limiter.entries[key]
	if entry.ResetAt.IsZero() || !entry.ResetAt.After(now) {
		entry = rateLimitEntry{ResetAt: now.Add(window)}
	}
	entry.Count++
	limiter.entries[key] = entry
	return entry.Count <= limit, entry.ResetAt.Sub(now), nil
}

var authRateLimiter authAttemptLimiter = newFixedWindowLimiter()

type authUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	HasPassword  bool   `json:"has_password"`
	IsAdmin      bool   `json:"is_admin"`
	Experience   int    `json:"experience"`
	Level        int    `json:"level"`
	AvatarFrame  string `json:"avatar_frame"`
	PasswordHash string `json:"-"`
}

// publicUserRecord 是公开用户资料的最小载体：email 只用于计算展示等级，绝不外泄。
type publicUserRecord struct {
	ID          string
	DisplayName string
	Email       string
	Experience  int
	AvatarFrame string
	CreatedAt   time.Time
}

// zodiacAvatarFrameIDs 是 12 星座预设头像框 id 的 Go 侧白名单，
// 必须与前端 avatarFrameData.ts 的 AVATAR_FRAME_IDS 保持一致。
var zodiacAvatarFrameIDs = map[string]struct{}{
	"zodiac-aries": {}, "zodiac-taurus": {}, "zodiac-gemini": {}, "zodiac-cancer": {},
	"zodiac-leo": {}, "zodiac-virgo": {}, "zodiac-libra": {}, "zodiac-scorpio": {},
	"zodiac-sagittarius": {}, "zodiac-capricorn": {}, "zodiac-aquarius": {}, "zodiac-pisces": {},
}

// validAvatarFrame 校验头像框取值：空（回退等级框）、预设星座 id，
// 或 custom:<objectKey>（对象键须归属当前用户，即位于 uploads/<userID>/ 前缀下）。
func validAvatarFrame(frame, userID string) bool {
	if frame == "" {
		return true
	}
	if _, ok := zodiacAvatarFrameIDs[frame]; ok {
		return true
	}
	if key, found := strings.CutPrefix(frame, "custom:"); found {
		prefix := "uploads/" + userID + "/"
		return strings.HasPrefix(key, prefix) && len(key) <= 180 && !strings.Contains(key, "..")
	}
	return false
}

type authSession struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type passwordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type authRepository interface {
	CreateUser(context.Context, authUser) error
	FindUserByEmail(context.Context, string) (authUser, bool, error)
	CreateSession(context.Context, authSession) error
	FindUserByTokenHash(context.Context, string, time.Time) (authUser, bool, error)
	DeleteSession(context.Context, string) error
	DeleteUserSessions(context.Context, string) error
	ListUserSessions(context.Context, string, time.Time) ([]authSession, error)
	DeleteUserSession(context.Context, string, string) (authSession, bool, error)
	UpdatePasswordAndDeleteOtherSessions(context.Context, string, string, string) error
	UpdateDisplayName(context.Context, string, string) (authUser, bool, error)
	// UpdateAvatarFrame 持久化用户所选头像框，返回更新后的用户。未找到时 bool 为 false。
	UpdateAvatarFrame(ctx context.Context, userID, frame string) (authUser, bool, error)
	// FramesByUserIDs 批量返回用户头像框，用于评论作者头像框展示。
	FramesByUserIDs(ctx context.Context, ids []string) (map[string]string, error)
	// AddExperience 幂等地给用户加经验，返回是否实际记入（重复动作返回 false）。
	AddExperience(ctx context.Context, userID, action, sourceKey string, points int) (bool, error)
	// LevelsByUserIDs 批量返回用户等级，用于评论作者等级展示。
	LevelsByUserIDs(ctx context.Context, ids []string) (map[string]int, error)
	// FindPublicUserByID 按用户 ID 返回公开资料（含用于算等级的邮箱，不外泄）。未找到时 bool 为 false。
	FindPublicUserByID(ctx context.Context, id string) (publicUserRecord, bool, error)
	// UsersByIDs 批量返回用户资料，用于举报列表等场景补全举报人信息。
	UsersByIDs(ctx context.Context, ids []string) (map[string]authUser, error)
	// CountUsers 返回注册用户总数，供管理概览统计。
	CountUsers(ctx context.Context) (int, error)
	// ListUsers 分页返回用户摘要，search 非空时按邮箱/昵称模糊匹配，同时返回匹配总数。
	ListUsers(ctx context.Context, search string, limit, offset int) ([]adminUserSummary, int, error)
	// UserStats 汇总最近 days 天的注册趋势（按日零填充）与等级分布，供管理概览图表。
	UserStats(ctx context.Context, days int) (userStatsData, error)
	CreatePasswordResetToken(context.Context, string, passwordResetToken) (authUser, bool, error)
	ConsumePasswordResetToken(context.Context, string, time.Time, string) (bool, error)
}

type memoryAuthRepository struct {
	sync.RWMutex
	usersByEmail     map[string]authUser
	userCreatedAt    map[string]time.Time
	sessions         map[string]authSession
	resetTokens      map[string]passwordResetToken
	experienceLedger map[string]struct{}
}

func newMemoryAuthRepository() *memoryAuthRepository {
	return &memoryAuthRepository{
		usersByEmail:     make(map[string]authUser),
		userCreatedAt:    make(map[string]time.Time),
		sessions:         make(map[string]authSession),
		resetTokens:      make(map[string]passwordResetToken),
		experienceLedger: make(map[string]struct{}),
	}
}

func (repository *memoryAuthRepository) CreateUser(_ context.Context, user authUser) error {
	repository.Lock()
	defer repository.Unlock()
	if _, found := repository.usersByEmail[user.Email]; found {
		return errEmailExists
	}
	repository.usersByEmail[user.Email] = user
	repository.userCreatedAt[user.ID] = time.Now().UTC()
	return nil
}

func (repository *memoryAuthRepository) CountUsers(_ context.Context) (int, error) {
	repository.RLock()
	defer repository.RUnlock()
	return len(repository.usersByEmail), nil
}

func (repository *memoryAuthRepository) ListUsers(
	_ context.Context, search string, limit, offset int,
) ([]adminUserSummary, int, error) {
	repository.RLock()
	defer repository.RUnlock()
	needle := strings.ToLower(strings.TrimSpace(search))
	matched := make([]adminUserSummary, 0)
	for _, user := range repository.usersByEmail {
		if needle != "" &&
			!strings.Contains(strings.ToLower(user.Email), needle) &&
			!strings.Contains(strings.ToLower(user.DisplayName), needle) {
			continue
		}
		matched = append(matched, adminUserSummary{
			ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
			Experience: user.Experience, Level: levelForUser(user.Email, user.Experience),
			IsAdmin: isAdminEmail(user.Email), CreatedAt: repository.userCreatedAt[user.ID],
		})
	}
	// 稳定排序：注册时间倒序，时间相同按 ID 兜底。
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	total := len(matched)
	if offset >= total {
		return []adminUserSummary{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	page := make([]adminUserSummary, end-offset)
	copy(page, matched[offset:end])
	return page, total, nil
}

func (repository *memoryAuthRepository) UserStats(_ context.Context, days int) (userStatsData, error) {
	if days <= 0 {
		days = userStatsDefaultDays
	}
	repository.RLock()
	defer repository.RUnlock()
	registrations, index := buildRegistrationBuckets(days)
	levelCounts := make(map[int]int)
	total := 0
	for _, user := range repository.usersByEmail {
		total++
		levelCounts[levelForUser(user.Email, user.Experience)]++
		created, ok := repository.userCreatedAt[user.ID]
		if !ok {
			continue
		}
		if pos, ok := index[created.UTC().Format("2006-01-02")]; ok {
			registrations[pos].Count++
		}
	}
	return userStatsData{
		TotalUsers: total, Days: days,
		Registrations: registrations, LevelHistogram: levelHistogramFromCounts(levelCounts),
	}, nil
}

func (repository *memoryAuthRepository) FindUserByEmail(_ context.Context, email string) (authUser, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	user, found := repository.usersByEmail[email]
	return user, found, nil
}

func (repository *memoryAuthRepository) CreateSession(_ context.Context, session authSession) error {
	repository.Lock()
	defer repository.Unlock()
	repository.sessions[session.TokenHash] = session
	return nil
}

func (repository *memoryAuthRepository) FindUserByTokenHash(_ context.Context, tokenHash string, now time.Time) (authUser, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	session, found := repository.sessions[tokenHash]
	if !found || !session.ExpiresAt.After(now) {
		return authUser{}, false, nil
	}
	for _, user := range repository.usersByEmail {
		if user.ID == session.UserID {
			return user, true, nil
		}
	}
	return authUser{}, false, nil
}

func (repository *memoryAuthRepository) DeleteSession(_ context.Context, tokenHash string) error {
	repository.Lock()
	defer repository.Unlock()
	delete(repository.sessions, tokenHash)
	return nil
}

func (repository *memoryAuthRepository) DeleteUserSessions(_ context.Context, userID string) error {
	repository.Lock()
	defer repository.Unlock()
	for tokenHash, session := range repository.sessions {
		if session.UserID == userID {
			delete(repository.sessions, tokenHash)
		}
	}
	return nil
}

func (repository *memoryAuthRepository) ListUserSessions(_ context.Context, userID string, now time.Time) ([]authSession, error) {
	repository.RLock()
	defer repository.RUnlock()
	result := make([]authSession, 0)
	for _, session := range repository.sessions {
		if session.UserID == userID && session.ExpiresAt.After(now) {
			result = append(result, session)
		}
	}
	return result, nil
}

func (repository *memoryAuthRepository) DeleteUserSession(
	_ context.Context, userID string, sessionID string,
) (authSession, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for tokenHash, session := range repository.sessions {
		if session.ID == sessionID && session.UserID == userID {
			delete(repository.sessions, tokenHash)
			return session, true, nil
		}
	}
	return authSession{}, false, nil
}

func (repository *memoryAuthRepository) UpdatePasswordAndDeleteOtherSessions(
	_ context.Context, userID string, passwordHash string, currentTokenHash string,
) error {
	repository.Lock()
	defer repository.Unlock()
	for email, user := range repository.usersByEmail {
		if user.ID == userID {
			user.PasswordHash = passwordHash
			repository.usersByEmail[email] = user
			for tokenHash, session := range repository.sessions {
				if session.UserID == userID && tokenHash != currentTokenHash {
					delete(repository.sessions, tokenHash)
				}
			}
			return nil
		}
	}
	return errors.New("user not found")
}

func (repository *memoryAuthRepository) UpdateDisplayName(
	_ context.Context, userID string, displayName string,
) (authUser, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for email, user := range repository.usersByEmail {
		if user.ID == userID {
			user.DisplayName = displayName
			repository.usersByEmail[email] = user
			return user, true, nil
		}
	}
	return authUser{}, false, nil
}

func (repository *memoryAuthRepository) UpdateAvatarFrame(
	_ context.Context, userID string, frame string,
) (authUser, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	for email, user := range repository.usersByEmail {
		if user.ID == userID {
			user.AvatarFrame = frame
			repository.usersByEmail[email] = user
			return user, true, nil
		}
	}
	return authUser{}, false, nil
}

func (repository *memoryAuthRepository) FramesByUserIDs(
	_ context.Context, ids []string,
) (map[string]string, error) {
	repository.RLock()
	defer repository.RUnlock()
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	frames := make(map[string]string)
	for _, user := range repository.usersByEmail {
		if _, ok := wanted[user.ID]; ok && user.AvatarFrame != "" {
			frames[user.ID] = user.AvatarFrame
		}
	}
	return frames, nil
}

func (repository *memoryAuthRepository) AddExperience(
	_ context.Context, userID, action, sourceKey string, points int,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	key := userID + "|" + action + "|" + sourceKey
	if _, done := repository.experienceLedger[key]; done {
		return false, nil
	}
	for email, user := range repository.usersByEmail {
		if user.ID == userID {
			user.Experience += points
			repository.usersByEmail[email] = user
			repository.experienceLedger[key] = struct{}{}
			return true, nil
		}
	}
	// 用户不存在：不记账本，视为未加。
	return false, nil
}

func (repository *memoryAuthRepository) LevelsByUserIDs(
	_ context.Context, ids []string,
) (map[string]int, error) {
	repository.RLock()
	defer repository.RUnlock()
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	levels := make(map[string]int)
	for _, user := range repository.usersByEmail {
		if _, ok := wanted[user.ID]; ok {
			levels[user.ID] = levelForUser(user.Email, user.Experience)
		}
	}
	return levels, nil
}

func (repository *memoryAuthRepository) FindPublicUserByID(
	_ context.Context, id string,
) (publicUserRecord, bool, error) {
	repository.RLock()
	defer repository.RUnlock()
	for _, user := range repository.usersByEmail {
		if user.ID == id {
			return publicUserRecord{
				ID: user.ID, DisplayName: user.DisplayName, Email: user.Email,
				Experience: user.Experience, AvatarFrame: user.AvatarFrame,
				CreatedAt: repository.userCreatedAt[user.ID],
			}, true, nil
		}
	}
	return publicUserRecord{}, false, nil
}

func (repository *memoryAuthRepository) UsersByIDs(
	_ context.Context, ids []string,
) (map[string]authUser, error) {
	repository.RLock()
	defer repository.RUnlock()
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	users := make(map[string]authUser)
	for _, user := range repository.usersByEmail {
		if _, ok := wanted[user.ID]; ok {
			users[user.ID] = user
		}
	}
	return users, nil
}

func (repository *memoryAuthRepository) CreatePasswordResetToken(
	_ context.Context, email string, token passwordResetToken,
) (authUser, bool, error) {
	repository.Lock()
	defer repository.Unlock()
	user, found := repository.usersByEmail[email]
	if !found || user.PasswordHash == "" {
		return authUser{}, false, nil
	}
	for tokenHash, existing := range repository.resetTokens {
		if existing.UserID == user.ID && existing.UsedAt == nil {
			delete(repository.resetTokens, tokenHash)
		}
	}
	token.UserID = user.ID
	repository.resetTokens[token.TokenHash] = token
	return user, true, nil
}

func (repository *memoryAuthRepository) ConsumePasswordResetToken(
	_ context.Context, tokenHash string, now time.Time, passwordHash string,
) (bool, error) {
	repository.Lock()
	defer repository.Unlock()
	token, found := repository.resetTokens[tokenHash]
	if !found || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return false, nil
	}
	var userEmail string
	var user authUser
	for email, candidate := range repository.usersByEmail {
		if candidate.ID == token.UserID {
			userEmail, user = email, candidate
			break
		}
	}
	if userEmail == "" {
		return false, nil
	}
	user.PasswordHash = passwordHash
	repository.usersByEmail[userEmail] = user
	usedAt := now
	for hash, existing := range repository.resetTokens {
		if existing.UserID == user.ID && existing.UsedAt == nil {
			existing.UsedAt = &usedAt
			repository.resetTokens[hash] = existing
		}
	}
	for hash, session := range repository.sessions {
		if session.UserID == user.ID {
			delete(repository.sessions, hash)
		}
	}
	return true, nil
}

type mysqlAuthRepository struct {
	db *sql.DB
}

func newMySQLAuthRepository(db *sql.DB) *mysqlAuthRepository {
	return &mysqlAuthRepository{db: db}
}

func (repository *mysqlAuthRepository) CreateUser(ctx context.Context, user authUser) error {
	const query = `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`
	if _, err := repository.db.ExecContext(ctx, query, user.ID, user.Email, user.DisplayName, user.PasswordHash); err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return errEmailExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (repository *mysqlAuthRepository) FindUserByEmail(ctx context.Context, email string) (authUser, bool, error) {
	const query = `SELECT id, email, display_name, password_hash, experience, avatar_frame FROM users WHERE email = ?`
	var user authUser
	err := repository.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Experience, &user.AvatarFrame,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return authUser{}, false, nil
	}
	if err != nil {
		return authUser{}, false, fmt.Errorf("find user by email: %w", err)
	}
	return user, true, nil
}

func (repository *mysqlAuthRepository) CreateSession(ctx context.Context, session authSession) error {
	const query = `
		INSERT INTO auth_sessions (id, user_id, token_hash, expires_at)
		VALUES (?, ?, ?, ?)`
	if _, err := repository.db.ExecContext(
		ctx, query, session.ID, session.UserID, session.TokenHash, session.ExpiresAt.UTC(),
	); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (repository *mysqlAuthRepository) FindUserByTokenHash(
	ctx context.Context, tokenHash string, now time.Time,
) (authUser, bool, error) {
	const query = `
		SELECT users.id, users.email, users.display_name, users.password_hash, users.experience, users.avatar_frame
		FROM auth_sessions
		JOIN users ON users.id = auth_sessions.user_id
		WHERE auth_sessions.token_hash = ? AND auth_sessions.expires_at > ?`
	var user authUser
	err := repository.db.QueryRowContext(ctx, query, tokenHash, now.UTC()).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Experience, &user.AvatarFrame,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return authUser{}, false, nil
	}
	if err != nil {
		return authUser{}, false, fmt.Errorf("find session: %w", err)
	}
	return user, true, nil
}

func (repository *mysqlAuthRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := repository.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (repository *mysqlAuthRepository) DeleteUserSessions(ctx context.Context, userID string) error {
	if _, err := repository.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (repository *mysqlAuthRepository) ListUserSessions(ctx context.Context, userID string, now time.Time) ([]authSession, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM auth_sessions
		WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC`
	rows, err := repository.db.QueryContext(ctx, query, userID, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}
	defer rows.Close()
	result := make([]authSession, 0)
	for rows.Next() {
		var session authSession
		if err := rows.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user session: %w", err)
		}
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user sessions: %w", err)
	}
	return result, nil
}

func (repository *mysqlAuthRepository) DeleteUserSession(
	ctx context.Context, userID string, sessionID string,
) (authSession, bool, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return authSession{}, false, fmt.Errorf("begin delete user session: %w", err)
	}
	defer transaction.Rollback()

	var session authSession
	const selectQuery = `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM auth_sessions
		WHERE id = ? AND user_id = ?
		FOR UPDATE`
	err = transaction.QueryRowContext(ctx, selectQuery, sessionID, userID).Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &session.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return authSession{}, false, nil
	}
	if err != nil {
		return authSession{}, false, fmt.Errorf("find user session for deletion: %w", err)
	}
	if _, err := transaction.ExecContext(
		ctx, `DELETE FROM auth_sessions WHERE id = ? AND user_id = ?`, sessionID, userID,
	); err != nil {
		return authSession{}, false, fmt.Errorf("delete user session: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return authSession{}, false, fmt.Errorf("commit user session deletion: %w", err)
	}
	return session, true, nil
}

func (repository *mysqlAuthRepository) UpdatePasswordAndDeleteOtherSessions(
	ctx context.Context, userID string, passwordHash string, currentTokenHash string,
) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password update: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(
		ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read password update result: %w", err)
	}
	if affected != 1 {
		return errors.New("user not found")
	}
	if _, err := transaction.ExecContext(
		ctx, `DELETE FROM auth_sessions WHERE user_id = ? AND token_hash <> ?`, userID, currentTokenHash,
	); err != nil {
		return fmt.Errorf("delete other sessions after password update: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit password update: %w", err)
	}
	return nil
}

func (repository *mysqlAuthRepository) UpdateDisplayName(
	ctx context.Context, userID string, displayName string,
) (authUser, bool, error) {
	result, err := repository.db.ExecContext(
		ctx, `UPDATE users SET display_name = ? WHERE id = ?`, displayName, userID,
	)
	if err != nil {
		return authUser{}, false, fmt.Errorf("update display name: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return authUser{}, false, fmt.Errorf("read display name update result: %w", err)
	}
	if affected != 1 {
		return authUser{}, false, nil
	}
	var user authUser
	err = repository.db.QueryRowContext(
		ctx, `SELECT id, email, display_name, password_hash, experience, avatar_frame FROM users WHERE id = ?`, userID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Experience, &user.AvatarFrame)
	if err != nil {
		return authUser{}, false, fmt.Errorf("read user after display name update: %w", err)
	}
	return user, true, nil
}

func (repository *mysqlAuthRepository) UpdateAvatarFrame(
	ctx context.Context, userID string, frame string,
) (authUser, bool, error) {
	result, err := repository.db.ExecContext(
		ctx, `UPDATE users SET avatar_frame = ? WHERE id = ?`, frame, userID,
	)
	if err != nil {
		return authUser{}, false, fmt.Errorf("update avatar frame: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return authUser{}, false, fmt.Errorf("read avatar frame update result: %w", err)
	}
	if affected != 1 {
		return authUser{}, false, nil
	}
	var user authUser
	err = repository.db.QueryRowContext(
		ctx, `SELECT id, email, display_name, password_hash, experience, avatar_frame FROM users WHERE id = ?`, userID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Experience, &user.AvatarFrame)
	if err != nil {
		return authUser{}, false, fmt.Errorf("read user after avatar frame update: %w", err)
	}
	return user, true, nil
}

func (repository *mysqlAuthRepository) FramesByUserIDs(
	ctx context.Context, ids []string,
) (map[string]string, error) {
	frames := make(map[string]string)
	if len(ids) == 0 {
		return frames, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, avatar_frame FROM users WHERE id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load user frames: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, frame string
		if err := rows.Scan(&id, &frame); err != nil {
			return nil, fmt.Errorf("scan user frame: %w", err)
		}
		if frame != "" {
			frames[id] = frame
		}
	}
	return frames, rows.Err()
}

func (repository *mysqlAuthRepository) AddExperience(
	ctx context.Context, userID, action, sourceKey string, points int,
) (bool, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin experience award: %w", err)
	}
	defer transaction.Rollback()
	// INSERT IGNORE 命中唯一键 (user, action, source_key) 时返回 0 行，
	// 由此实现幂等：同一动作对同一目标只加一次经验。
	result, err := transaction.ExecContext(ctx,
		`INSERT IGNORE INTO experience_events (id, user_id, action, source_key, points)
		 VALUES (?, ?, ?, ?, ?)`,
		"xp-"+newRequestID(), userID, action, sourceKey, points)
	if err != nil {
		return false, fmt.Errorf("insert experience event: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read experience event result: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE users SET experience = experience + ? WHERE id = ?`, points, userID); err != nil {
		return false, fmt.Errorf("increment user experience: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit experience award: %w", err)
	}
	return true, nil
}

func (repository *mysqlAuthRepository) LevelsByUserIDs(
	ctx context.Context, ids []string,
) (map[string]int, error) {
	levels := make(map[string]int)
	if len(ids) == 0 {
		return levels, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, email, experience FROM users WHERE id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load user levels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, email string
		var experience int
		if err := rows.Scan(&id, &email, &experience); err != nil {
			return nil, fmt.Errorf("scan user level: %w", err)
		}
		levels[id] = levelForUser(email, experience)
	}
	return levels, rows.Err()
}

func (repository *mysqlAuthRepository) FindPublicUserByID(
	ctx context.Context, id string,
) (publicUserRecord, bool, error) {
	const query = `SELECT id, email, display_name, experience, avatar_frame, created_at FROM users WHERE id = ?`
	var record publicUserRecord
	err := repository.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID, &record.Email, &record.DisplayName, &record.Experience, &record.AvatarFrame, &record.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return publicUserRecord{}, false, nil
	}
	if err != nil {
		return publicUserRecord{}, false, fmt.Errorf("find public user by id: %w", err)
	}
	return record, true, nil
}

func (repository *mysqlAuthRepository) UsersByIDs(
	ctx context.Context, ids []string,
) (map[string]authUser, error) {
	users := make(map[string]authUser)
	if len(ids) == 0 {
		return users, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, email, display_name FROM users WHERE id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load users by ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var user authUser
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName); err != nil {
			return nil, fmt.Errorf("scan user by id: %w", err)
		}
		users[user.ID] = user
	}
	return users, rows.Err()
}

func (repository *mysqlAuthRepository) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (repository *mysqlAuthRepository) ListUsers(
	ctx context.Context, search string, limit, offset int,
) ([]adminUserSummary, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where, arguments := "", []any{}
	if needle := strings.TrimSpace(search); needle != "" {
		where = ` WHERE email LIKE ? OR display_name LIKE ?`
		pattern := "%" + needle + "%"
		arguments = append(arguments, pattern, pattern)
	}
	var total int
	if err := repository.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users`+where, arguments...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count matched users: %w", err)
	}
	pageArguments := append(append([]any{}, arguments...), limit, offset)
	rows, err := repository.db.QueryContext(ctx,
		`SELECT id, email, display_name, experience, created_at FROM users`+where+
			` ORDER BY created_at DESC, id ASC LIMIT ? OFFSET ?`, pageArguments...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	result := make([]adminUserSummary, 0)
	for rows.Next() {
		var summary adminUserSummary
		if err := rows.Scan(&summary.ID, &summary.Email, &summary.DisplayName,
			&summary.Experience, &summary.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user summary: %w", err)
		}
		summary.Level = levelForUser(summary.Email, summary.Experience)
		summary.IsAdmin = isAdminEmail(summary.Email)
		result = append(result, summary)
	}
	return result, total, rows.Err()
}

func (repository *mysqlAuthRepository) UserStats(ctx context.Context, days int) (userStatsData, error) {
	if days <= 0 {
		days = userStatsDefaultDays
	}
	var total int
	if err := repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return userStatsData{}, fmt.Errorf("count users for stats: %w", err)
	}
	registrations, index := buildRegistrationBuckets(days)
	since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	trend, err := repository.db.QueryContext(ctx,
		`SELECT DATE(created_at), COUNT(*) FROM users WHERE created_at >= ? GROUP BY DATE(created_at)`, since)
	if err != nil {
		return userStatsData{}, fmt.Errorf("registration trend: %w", err)
	}
	defer trend.Close()
	for trend.Next() {
		var day string
		var count int
		if err := trend.Scan(&day, &count); err != nil {
			return userStatsData{}, fmt.Errorf("scan registration trend: %w", err)
		}
		// DATE(...) 可能以带时间的形式回读，统一截到 YYYY-MM-DD。
		if len(day) > 10 {
			day = day[:10]
		}
		if pos, ok := index[day]; ok {
			registrations[pos].Count = count
		}
	}
	if err := trend.Err(); err != nil {
		return userStatsData{}, fmt.Errorf("iterate registration trend: %w", err)
	}
	// 等级由邮箱（管理员恒为最高级）与经验共同决定，逐行在 Go 侧归档。
	levelCounts := make(map[int]int)
	levels, err := repository.db.QueryContext(ctx, `SELECT email, experience FROM users`)
	if err != nil {
		return userStatsData{}, fmt.Errorf("level histogram: %w", err)
	}
	defer levels.Close()
	for levels.Next() {
		var email string
		var experience int
		if err := levels.Scan(&email, &experience); err != nil {
			return userStatsData{}, fmt.Errorf("scan level histogram: %w", err)
		}
		levelCounts[levelForUser(email, experience)]++
	}
	if err := levels.Err(); err != nil {
		return userStatsData{}, fmt.Errorf("iterate level histogram: %w", err)
	}
	return userStatsData{
		TotalUsers: total, Days: days,
		Registrations: registrations, LevelHistogram: levelHistogramFromCounts(levelCounts),
	}, nil
}

func (repository *mysqlAuthRepository) CreatePasswordResetToken(
	ctx context.Context, email string, token passwordResetToken,
) (authUser, bool, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return authUser{}, false, fmt.Errorf("begin password reset creation: %w", err)
	}
	defer transaction.Rollback()
	var user authUser
	err = transaction.QueryRowContext(
		ctx,
		`SELECT id, email, display_name, password_hash, experience, avatar_frame FROM users WHERE email = ? FOR UPDATE`,
		email,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Experience, &user.AvatarFrame)
	if errors.Is(err, sql.ErrNoRows) {
		return authUser{}, false, nil
	}
	if err != nil {
		return authUser{}, false, fmt.Errorf("find user for password reset: %w", err)
	}
	if user.PasswordHash == "" {
		return authUser{}, false, nil
	}
	if _, err := transaction.ExecContext(
		ctx, `DELETE FROM password_reset_tokens WHERE user_id = ? AND used_at IS NULL`, user.ID,
	); err != nil {
		return authUser{}, false, fmt.Errorf("delete previous password reset tokens: %w", err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		token.ID, user.ID, token.TokenHash, token.ExpiresAt.UTC(),
	); err != nil {
		return authUser{}, false, fmt.Errorf("create password reset token: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return authUser{}, false, fmt.Errorf("commit password reset creation: %w", err)
	}
	return user, true, nil
}

func (repository *mysqlAuthRepository) ConsumePasswordResetToken(
	ctx context.Context, tokenHash string, now time.Time, passwordHash string,
) (bool, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin password reset: %w", err)
	}
	defer transaction.Rollback()
	var tokenID, userID string
	err = transaction.QueryRowContext(
		ctx,
		`SELECT id, user_id FROM password_reset_tokens
		 WHERE token_hash = ? AND used_at IS NULL AND expires_at > ? FOR UPDATE`,
		tokenHash, now.UTC(),
	).Scan(&tokenID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find password reset token: %w", err)
	}
	if _, err := transaction.ExecContext(
		ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID,
	); err != nil {
		return false, fmt.Errorf("update reset password: %w", err)
	}
	if _, err := transaction.ExecContext(
		ctx, `UPDATE password_reset_tokens SET used_at = ? WHERE user_id = ? AND used_at IS NULL`,
		now.UTC(), userID,
	); err != nil {
		return false, fmt.Errorf("consume password reset tokens: %w", err)
	}
	if _, err := transaction.ExecContext(
		ctx, `DELETE FROM auth_sessions WHERE user_id = ?`, userID,
	); err != nil {
		return false, fmt.Errorf("delete sessions after password reset: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit password reset: %w", err)
	}
	return true, nil
}

var authRepositoryStore authRepository

type authRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type profileUpdateRequest struct {
	DisplayName string `json:"display_name"`
}

type avatarFrameUpdateRequest struct {
	Frame string `json:"frame"`
}

type passwordResetRequest struct {
	Email string `json:"email"`
}

type passwordResetConfirmRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type acceptedResponse struct {
	RequestID string `json:"request_id"`
}

type authResponse struct {
	Data      authUser `json:"data"`
	RequestID string   `json:"request_id"`
}

type sessionInfo struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Current   bool   `json:"current"`
}

type sessionListResponse struct {
	Data      []sessionInfo `json:"data"`
	RequestID string        `json:"request_id"`
}

func authRegisterHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if authRepositoryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "auth_unavailable", "身份服务暂不可用")
		return
	}
	var input authRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的注册数据")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !allowAuthAttempt(writer, request, "register", input.Email, 5, time.Hour) {
		return
	}
	if !validEmail(input.Email) || len([]rune(input.DisplayName)) < 2 || len([]rune(input.DisplayName)) > 80 ||
		len(input.Password) < 8 || len(input.Password) > 128 {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_registration", "邮箱、昵称或密码不符合要求")
		return
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	user := authUser{
		ID: "user-" + newRequestID(), Email: input.Email,
		DisplayName: input.DisplayName, PasswordHash: passwordHash,
	}
	if err := authRepositoryStore.CreateUser(request.Context(), user); errors.Is(err, errEmailExists) {
		auditAuth(request, "registration_rejected", input.Email, "")
		writeAPIError(writer, request, http.StatusConflict, "email_exists", "该邮箱已注册")
		return
	} else if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if err := createLoginSession(writer, request, user); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	auditAuth(request, "registration_succeeded", input.Email, user.ID)
	writeJSON(writer, http.StatusCreated, authResponse{Data: publicAuthUser(user), RequestID: requestIDFromContext(request.Context())})
}

func authLoginHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if authRepositoryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "auth_unavailable", "身份服务暂不可用")
		return
	}
	var input authRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的登录数据")
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if !allowAuthAttempt(writer, request, "login", email, 10, 15*time.Minute) {
		return
	}
	user, found, err := authRepositoryStore.FindUserByEmail(request.Context(), email)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !found || !verifyPassword(user.PasswordHash, input.Password) {
		auditAuth(request, "login_failed", email, "")
		writeAPIError(writer, request, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码不正确")
		return
	}
	if err := createLoginSession(writer, request, user); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	auditAuth(request, "login_succeeded", email, user.ID)
	writeJSON(writer, http.StatusOK, authResponse{Data: publicAuthUser(user), RequestID: requestIDFromContext(request.Context())})
}

func authMeHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodPatch {
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := currentUser(request)
	if !ok {
		writeAPIError(writer, request, http.StatusUnauthorized, "authentication_required", "请先登录")
		return
	}
	if request.Method == http.MethodPatch {
		var input profileUpdateRequest
		if err := decodeJSONBody(request, &input); err != nil {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的用户资料")
			return
		}
		input.DisplayName = strings.TrimSpace(input.DisplayName)
		if length := len([]rune(input.DisplayName)); length < 2 || length > 80 {
			writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_display_name", "昵称长度必须为 2 到 80 个字符")
			return
		}
		updated, found, err := authRepositoryStore.UpdateDisplayName(
			request.Context(), user.ID, input.DisplayName,
		)
		if err != nil {
			writeAuthInternalError(writer, request, err)
			return
		}
		if !found {
			writeAPIError(writer, request, http.StatusUnauthorized, "authentication_required", "请先登录")
			return
		}
		user = updated
		auditAuth(request, "profile_updated", user.Email, user.ID)
	}
	writeJSON(writer, http.StatusOK, authResponse{Data: publicAuthUser(user), RequestID: requestIDFromContext(request.Context())})
}

func authAvatarFrameHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		writer.Header().Set("Allow", http.MethodPut)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	var input avatarFrameUpdateRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的头像框数据")
		return
	}
	input.Frame = strings.TrimSpace(input.Frame)
	if !validAvatarFrame(input.Frame, user.ID) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_avatar_frame", "头像框取值不合法")
		return
	}
	updated, found, err := authRepositoryStore.UpdateAvatarFrame(request.Context(), user.ID, input.Frame)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusUnauthorized, "authentication_required", "请先登录")
		return
	}
	auditAuth(request, "avatar_frame_updated", updated.Email, updated.ID)
	writeJSON(writer, http.StatusOK, authResponse{Data: publicAuthUser(updated), RequestID: requestIDFromContext(request.Context())})
}

func publicAuthUser(user authUser) authUser {
	user.HasPassword = user.PasswordHash != ""
	user.IsAdmin = isAdminEmail(user.Email)
	user.Level = levelForUser(user.Email, user.Experience)
	user.PasswordHash = ""
	return user
}

func authLogoutHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if cookie, err := request.Cookie(sessionCookieName); err == nil && authRepositoryStore != nil {
		if err := authRepositoryStore.DeleteSession(request.Context(), sessionTokenHash(cookie.Value)); err != nil {
			writeAuthInternalError(writer, request, err)
			return
		}
	}
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode,
	})
	writer.WriteHeader(http.StatusNoContent)
}

func authLogoutAllHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if err := authRepositoryStore.DeleteUserSessions(request.Context(), user.ID); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode,
	})
	auditAuth(request, "all_sessions_revoked", user.Email, user.ID)
	writer.WriteHeader(http.StatusNoContent)
}

func authSessionsHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	sessions, err := authRepositoryStore.ListUserSessions(request.Context(), user.ID, time.Now().UTC())
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	currentTokenHash := ""
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		currentTokenHash = sessionTokenHash(cookie.Value)
	}
	result := make([]sessionInfo, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, sessionInfo{
			ID: session.ID, CreatedAt: session.CreatedAt.UTC().Format(time.RFC3339),
			ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
			Current:   session.TokenHash == currentTokenHash,
		})
	}
	writeJSON(writer, http.StatusOK, sessionListResponse{Data: result, RequestID: requestIDFromContext(request.Context())})
}

func authSessionHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		writer.Header().Set("Allow", http.MethodDelete)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(request.PathValue("sessionID"))
	if sessionID == "" {
		writeAPIError(writer, request, http.StatusNotFound, "session_not_found", "登录会话不存在")
		return
	}
	session, found, err := authRepositoryStore.DeleteUserSession(request.Context(), user.ID, sessionID)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !found {
		writeAPIError(writer, request, http.StatusNotFound, "session_not_found", "登录会话不存在")
		return
	}
	if cookie, err := request.Cookie(sessionCookieName); err == nil &&
		session.TokenHash == sessionTokenHash(cookie.Value) {
		http.SetCookie(writer, &http.Cookie{
			Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode,
		})
	}
	auditAuth(request, "session_revoked", user.Email, user.ID)
	writer.WriteHeader(http.StatusNoContent)
}

func authPasswordHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		writer.Header().Set("Allow", http.MethodPut)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if user.PasswordHash == "" {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "password_unavailable", "此账号未设置邮箱密码")
		return
	}
	var input passwordChangeRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的密码修改数据")
		return
	}
	if !allowAuthAttempt(writer, request, "password-change", user.Email, 5, time.Hour) {
		return
	}
	if len(input.NewPassword) < 8 || len(input.NewPassword) > 128 ||
		input.CurrentPassword == input.NewPassword {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_new_password", "新密码不符合要求或与当前密码相同")
		return
	}
	if !verifyPassword(user.PasswordHash, input.CurrentPassword) {
		auditAuth(request, "password_change_failed", user.Email, user.ID)
		writeAPIError(writer, request, http.StatusUnauthorized, "invalid_password", "当前密码不正确")
		return
	}
	passwordHash, err := hashPassword(input.NewPassword)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeAPIError(writer, request, http.StatusUnauthorized, "authentication_required", "请先登录")
		return
	}
	if err := authRepositoryStore.UpdatePasswordAndDeleteOtherSessions(
		request.Context(), user.ID, passwordHash, sessionTokenHash(cookie.Value),
	); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	auditAuth(request, "password_changed", user.Email, user.ID)
	writer.WriteHeader(http.StatusNoContent)
}

func authPasswordResetRequestHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if authRepositoryStore == nil || passwordResetDeliveryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "password_reset_unavailable", "密码找回服务暂时不可用")
		return
	}
	var input passwordResetRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的密码找回数据")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if !validEmail(input.Email) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_email", "请输入有效邮箱")
		return
	}
	if !allowAuthAttempt(writer, request, "password-reset", input.Email, 5, time.Hour) {
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeAuthInternalError(writer, request, fmt.Errorf("generate password reset token: %w", err))
		return
	}
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	resetToken := passwordResetToken{
		ID: "reset-" + newRequestID(), TokenHash: sessionTokenHash(rawToken),
		ExpiresAt: time.Now().UTC().Add(passwordResetLifetime),
	}
	user, found, err := authRepositoryStore.CreatePasswordResetToken(
		request.Context(), input.Email, resetToken,
	)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if found {
		baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
		resetURL := baseURL + "/?reset_token=" + url.QueryEscape(rawToken)
		deliveryContext, cancel := context.WithTimeout(request.Context(), 25*time.Second)
		err = passwordResetDeliveryStore.SendPasswordReset(deliveryContext, user, resetURL)
		cancel()
		if err != nil {
			slog.ErrorContext(request.Context(), "password reset delivery failed",
				"request_id", requestIDFromContext(request.Context()), "error", err)
			auditAuth(request, "password_reset_delivery_failed", input.Email, user.ID)
		} else {
			auditAuth(request, "password_reset_requested", input.Email, user.ID)
		}
	} else {
		auditAuth(request, "password_reset_requested", input.Email, "")
	}
	writeJSON(writer, http.StatusAccepted, acceptedResponse{
		RequestID: requestIDFromContext(request.Context()),
	})
}

func authPasswordResetConfirmHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	if authRepositoryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "password_reset_unavailable", "密码找回服务暂时不可用")
		return
	}
	var input passwordResetConfirmRequest
	if err := decodeJSONBody(request, &input); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_body", "请求正文不是有效的密码重置数据")
		return
	}
	input.Token = strings.TrimSpace(input.Token)
	if input.Token == "" || len(input.Token) > 512 ||
		len(input.NewPassword) < 8 || len(input.NewPassword) > 128 {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_password_reset", "重置链接无效或新密码不符合要求")
		return
	}
	if !allowAuthAttempt(writer, request, "password-reset-confirm", shortHash(input.Token), 10, time.Hour) {
		return
	}
	passwordHash, err := hashPassword(input.NewPassword)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	consumed, err := authRepositoryStore.ConsumePasswordResetToken(
		request.Context(), sessionTokenHash(input.Token), time.Now().UTC(), passwordHash,
	)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !consumed {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_password_reset", "重置链接无效或已过期")
		return
	}
	auditAuth(request, "password_reset_succeeded", "", "")
	writer.WriteHeader(http.StatusNoContent)
}

func requireCurrentUser(writer http.ResponseWriter, request *http.Request) (authUser, bool) {
	if authRepositoryStore == nil {
		return authUser{}, true
	}
	user, ok := currentUser(request)
	if !ok {
		auditAuth(request, "authentication_required", "", "")
		writeAPIError(writer, request, http.StatusUnauthorized, "authentication_required", "请先登录")
	}
	return user, ok
}

func currentUser(request *http.Request) (authUser, bool) {
	if authRepositoryStore == nil {
		return authUser{}, false
	}
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return authUser{}, false
	}
	user, found, err := authRepositoryStore.FindUserByTokenHash(
		request.Context(), sessionTokenHash(cookie.Value), time.Now().UTC(),
	)
	if err != nil || !found {
		return authUser{}, false
	}
	return user, true
}

func createLoginSession(writer http.ResponseWriter, request *http.Request, user authUser) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(sessionLifetime)
	session := authSession{
		ID: "session-" + newRequestID(), UserID: user.ID,
		TokenHash: sessionTokenHash(token), ExpiresAt: expiresAt, CreatedAt: createdAt,
	}
	if err := authRepositoryStore.CreateSession(request.Context(), session); err != nil {
		return err
	}
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(sessionLifetime.Seconds()), HttpOnly: true,
		Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func requestIsHTTPS(request *http.Request) bool {
	return request.TLS != nil || strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https")
}

func validEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 || strings.Count(email, "@") != 1 {
		return false
	}
	parts := strings.SplitN(email, "@", 2)
	return parts[0] != "" && strings.Contains(parts[1], ".") &&
		!strings.ContainsAny(email, " \t\r\n")
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := pbkdf2SHA256([]byte(password), salt, passwordRounds, 32)
	return "pbkdf2-sha256$" + strconv.Itoa(passwordRounds) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(hash), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 100000 || rounds > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, rounds, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func pbkdf2SHA256(password, salt []byte, rounds, length int) []byte {
	hashLength := sha256.Size
	blocks := (length + hashLength - 1) / hashLength
	output := make([]byte, 0, blocks*hashLength)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		value := append([]byte(nil), u...)
		for index := 1; index < rounds; index++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for offset := range value {
				value[offset] ^= u[offset]
			}
		}
		output = append(output, value...)
	}
	return output[:length]
}

func writeAuthInternalError(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(
		request.Context(),
		"authentication operation failed",
		"request_id", requestIDFromContext(request.Context()),
		"error", err,
	)
	writeAPIError(writer, request, http.StatusInternalServerError, "authentication_error", "登录服务暂时不可用")
}

func allowAuthAttempt(
	writer http.ResponseWriter,
	request *http.Request,
	action, email string,
	limit int,
	window time.Duration,
) bool {
	now := time.Now()
	keys := []string{action + ":ip:" + requestClientIP(request)}
	if email != "" {
		keys = append(keys, action+":identity:"+shortHash(email))
	}
	var retryAfter time.Duration
	for _, key := range keys {
		allowed, retry, err := authRateLimiter.Allow(request.Context(), key, limit, window, now)
		if err != nil {
			writeAuthInternalError(writer, request, fmt.Errorf("authentication rate limiter: %w", err))
			return false
		}
		if !allowed && retry > retryAfter {
			retryAfter = retry
		}
	}
	if retryAfter <= 0 {
		return true
	}
	seconds := int(retryAfter.Seconds()) + 1
	writer.Header().Set("Retry-After", strconv.Itoa(seconds))
	auditAuth(request, action+"_rate_limited", email, "")
	writeAPIError(writer, request, http.StatusTooManyRequests, "rate_limited", "请求过于频繁，请稍后重试")
	return false
}

func requestClientIP(request *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func auditAuth(request *http.Request, action, email, userID string) {
	slog.InfoContext(
		request.Context(),
		"security audit",
		"event", action,
		"request_id", requestIDFromContext(request.Context()),
		"client_ip", requestClientIP(request),
		"identity_hash", shortHash(email),
		"user_id", userID,
	)
}
