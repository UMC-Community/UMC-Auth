package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type User struct {
	ID                 int64
	UUID               string
	Username           string
	UsernameNormalized string
	Email              string
	EmailNormalized    string
	PasswordHash       string
	EmailVerified      bool
	Disabled           bool
	CreatedAt          int64
	UpdatedAt          int64
	IsAdmin            bool
	Role               string
	Status             string
}

const userColumns = "id, uuid, username, username_normalized, email, email_normalized, password_hash, email_verified, disabled, created_at, updated_at, is_admin, role, status"

const userColumnsPrefixed = "u.id, u.uuid, u.username, u.username_normalized, u.email, u.email_normalized, u.password_hash, u.email_verified, u.disabled, u.created_at, u.updated_at, u.is_admin, u.role, u.status"

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isAdminRole(role string) bool {
	return role == "admin" || role == "superadmin"
}

func validRole(role string) bool {
	switch role {
	case "user", "moderator", "admin", "superadmin":
		return true
	default:
		return false
	}
}

type Invite struct {
	ID         int64
	CodePrefix string
	Note       string
	MaxUses    int
	UsedCount  int
	ExpiresAt  int64
	RevokedAt  sql.NullInt64
	CreatedAt  int64
}

type AuditEntry struct {
	ID        int64
	Actor     string
	Action    string
	Detail    string
	CreatedAt int64
}

type Store struct {
	db *sql.DB
}

func OpenStore(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if err := db.PingContext(ctx); err == nil {
			break
		} else if ctx.Err() != nil {
			db.Close()
			return nil, fmt.Errorf("database is not reachable: %w", err)
		}
		select {
		case <-ctx.Done():
			db.Close()
			return nil, fmt.Errorf("database is not reachable: %w", err)
		case <-time.After(time.Second):
		}
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.Cleanup(time.Now().Unix()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Cleanup(now int64) error {
	statements := []struct {
		query  string
		cutoff int64
	}{
		{"DELETE FROM access_tokens WHERE expires_at <= $1", now},
		{"DELETE FROM web_sessions WHERE expires_at <= $1", now},
		{"DELETE FROM joins WHERE created_at <= $1", now - 300},
		{"DELETE FROM email_verifications WHERE expires_at <= $1", now},
		{"DELETE FROM pow_challenges WHERE expires_at <= $1", now},
		{"DELETE FROM auth_sessions WHERE COALESCE(refresh_expires_at, access_expires_at) <= $1", now},
		{"DELETE FROM auth_sessions WHERE revoked_at IS NOT NULL AND revoked_at <= $1", now - 30*86400},
		{"DELETE FROM email_verification_codes WHERE expires_at <= $1", now - 86400},
		{"DELETE FROM password_reset_tokens WHERE expires_at <= $1", now - 86400},
		{"DELETE FROM login_attempts WHERE created_at <= $1", now - 30*86400},
		{"DELETE FROM mail_outbox WHERE status = 'sent' AND sent_at <= $1", now - 30*86400},
		{"DELETE FROM mail_outbox WHERE status = 'failed' AND created_at <= $1", now - 30*86400},
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement.query, statement.cutoff); err != nil {
			return fmt.Errorf("cleanup database: %w", err)
		}
	}
	return nil
}

func (s *Store) CountUsers() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *Store) CreateUser(
	uuid, username, passwordHash, email string,
	emailVerified, isAdmin bool,
	now int64,
) (int64, error) {
	role := "user"
	if isAdmin {
		role = "admin"
	}
	status := "active"
	if !emailVerified {
		status = "pending"
	}
	var id int64
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	err = tx.QueryRow(
		`INSERT INTO users
		 (uuid, username, username_normalized, password_hash, email, email_normalized,
		  email_verified, disabled, created_at, updated_at, is_admin, role, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8, $8, $9, $10, $11) RETURNING id`,
		uuid,
		strings.TrimSpace(username),
		normalizeUsername(username),
		passwordHash,
		strings.TrimSpace(email),
		normalizeEmail(email),
		emailVerified,
		now,
		isAdmin,
		role,
		status,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		`INSERT INTO minecraft_profiles (user_id, player_name, player_uuid, updated_at)
		 VALUES ($1, $2, $3, $4)`,
		id, strings.TrimSpace(username), normalizeUUID(uuid), now,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) UserByUsername(username string) (*User, error) {
	return scanUser(s.db.QueryRow(
		`SELECT `+userColumns+` FROM users WHERE username_normalized = $1`,
		normalizeUsername(username),
	))
}

func (s *Store) UserByEmail(email string) (*User, error) {
	return scanUser(s.db.QueryRow(
		`SELECT `+userColumns+` FROM users WHERE email_normalized = $1 AND email <> ''`,
		normalizeEmail(email),
	))
}

func (s *Store) UserByID(id int64) (*User, error) {
	return scanUser(s.db.QueryRow(
		`SELECT `+userColumns+` FROM users WHERE id = $1`,
		id,
	))
}

func (s *Store) UserByUUID(uuid string) (*User, error) {
	return scanUser(s.db.QueryRow(
		`SELECT `+userColumns+` FROM users WHERE uuid = $1`,
		normalizeUUID(uuid),
	))
}

func (s *Store) ListUsers(limit int) ([]User, error) {
	rows, err := s.db.Query(
		`SELECT `+userColumns+` FROM users ORDER BY created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

func (s *Store) DeleteUser(userID int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = $1", userID)
	return err
}

func (s *Store) SetUserDisabled(userID int64, disabled bool, now int64) error {
	if _, err := s.db.Exec(
		`UPDATE users
		 SET disabled = $1,
		     status = CASE
		         WHEN $1 THEN 'disabled'
		         WHEN email_verified THEN 'active'
		         ELSE 'pending'
		     END,
		     updated_at = $3
		 WHERE id = $2`,
		disabled, userID, now,
	); err != nil {
		return err
	}
	if disabled {
		if err := s.RevokeAllUserSessions(userID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetUserRole(userID int64, role string, now int64) error {
	if !validRole(role) {
		return fmt.Errorf("invalid role %q", role)
	}
	_, err := s.db.Exec(
		`UPDATE users SET role = $1, is_admin = $2, updated_at = $3 WHERE id = $4`,
		role, isAdminRole(role), now, userID,
	)
	return err
}

func (s *Store) SetUserEmailVerified(userID int64, verified bool, now int64) error {
	_, err := s.db.Exec(
		`UPDATE users
		 SET email_verified = $1,
		     status = CASE
		         WHEN disabled THEN 'disabled'
		         WHEN status = 'locked' THEN 'locked'
		         WHEN $1 THEN 'active'
		         ELSE 'pending'
		     END,
		     updated_at = $3
		 WHERE id = $2`,
		verified, userID, now,
	)
	return err
}

func (s *Store) UpdateUserPassword(userID int64, passwordHash string, now int64) error {
	if _, err := s.db.Exec(
		"UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3",
		passwordHash, now, userID,
	); err != nil {
		return err
	}
	return s.RevokeAllUserSessions(userID, now)
}

func (s *Store) RevokeAllUserSessions(userID int64, now int64) error {
	if _, err := s.db.Exec(
		"UPDATE auth_sessions SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL",
		now, userID,
	); err != nil {
		return err
	}
	if _, err := s.db.Exec("DELETE FROM access_tokens WHERE user_id = $1", userID); err != nil {
		return err
	}
	_, err := s.db.Exec("DELETE FROM web_sessions WHERE user_id = $1", userID)
	return err
}

func (s *Store) InsertAccessToken(userID int64, tokenHash, clientToken string, now, expiresAt int64) error {
	_, err := s.db.Exec(
		`INSERT INTO access_tokens (user_id, token_hash, client_token, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, tokenHash, clientToken, now, expiresAt,
	)
	return err
}

func (s *Store) RotateAccessToken(oldHash, newHash, clientToken string, userID, now, expiresAt int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM access_tokens WHERE token_hash = $1", oldHash); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO access_tokens (user_id, token_hash, client_token, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, newHash, clientToken, now, expiresAt,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AccessTokenUser(tokenHash string, now int64) (*User, string, error) {
	var user User
	var clientToken string
	dest := append(userScanDest(&user), &clientToken)
	err := s.db.QueryRow(
		`SELECT `+userColumnsPrefixed+`, t.client_token
		 FROM access_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1 AND t.expires_at > $2 AND u.disabled = FALSE
		   AND u.email_verified = TRUE AND u.status = 'active'`,
		tokenHash, now,
	).Scan(dest...)
	if err != nil {
		return nil, "", err
	}
	return &user, clientToken, nil
}

func (s *Store) DeleteAccessToken(tokenHash string) error {
	_, err := s.db.Exec("DELETE FROM access_tokens WHERE token_hash = $1", tokenHash)
	return err
}

func (s *Store) DeleteUserTokens(userID int64) error {
	_, err := s.db.Exec("DELETE FROM access_tokens WHERE user_id = $1", userID)
	return err
}

func (s *Store) CreateSession(userID int64, tokenHash, csrf string, now, expiresAt int64) error {
	_, err := s.db.Exec(
		`INSERT INTO web_sessions (user_id, token_hash, csrf_token, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, tokenHash, csrf, now, expiresAt,
	)
	return err
}

func (s *Store) SessionUser(tokenHash string, now int64) (*User, string, error) {
	var user User
	var csrf string
	dest := append(userScanDest(&user), &csrf)
	err := s.db.QueryRow(
		`SELECT `+userColumnsPrefixed+`, s.csrf_token
		 FROM web_sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > $2 AND u.disabled = FALSE
		   AND u.email_verified = TRUE AND u.status = 'active'`,
		tokenHash, now,
	).Scan(dest...)
	if err != nil {
		return nil, "", err
	}
	return &user, csrf, nil
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec("DELETE FROM web_sessions WHERE token_hash = $1", tokenHash)
	return err
}

func (s *Store) InsertJoin(userID int64, username, serverID, ip string, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO joins (user_id, username, server_id, ip, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, username, serverID, ip, now,
	)
	return err
}

func (s *Store) ConsumeJoin(serverID, username string, since int64, requestedIP string) (string, string, bool, error) {
	var id int64
	var uuid, name string
	var joinedIP sql.NullString
	err := s.db.QueryRow(
		`SELECT j.id, u.uuid, u.username, j.ip FROM joins j
		 JOIN users u ON u.id = j.user_id
		 WHERE j.server_id = $1 AND LOWER(j.username) = LOWER($2) AND j.created_at > $3
		 ORDER BY j.created_at DESC LIMIT 1`,
		serverID, username, since,
	).Scan(&id, &uuid, &name, &joinedIP)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if requestedIP != "" && joinedIP.Valid && joinedIP.String != "" && requestedIP != joinedIP.String {
		return "", "", false, nil
	}
	if _, err := s.db.Exec("DELETE FROM joins WHERE id = $1", id); err != nil {
		return "", "", false, err
	}
	return uuid, name, true, nil
}

func (s *Store) CreateInvite(codeHash, codePrefix, note string, maxUses int, expiresAt, createdBy, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO invites (code_hash, code_prefix, max_uses, expires_at, created_by, note, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		codeHash, codePrefix, maxUses, expiresAt, createdBy, note, now,
	)
	return err
}

func (s *Store) ListInvites(limit int) ([]Invite, error) {
	rows, err := s.db.Query(
		`SELECT id, code_prefix, note, max_uses, used_count, expires_at, revoked_at, created_at
		 FROM invites ORDER BY created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invites []Invite
	for rows.Next() {
		var invite Invite
		if err := rows.Scan(
			&invite.ID, &invite.CodePrefix, &invite.Note, &invite.MaxUses,
			&invite.UsedCount, &invite.ExpiresAt, &invite.RevokedAt, &invite.CreatedAt,
		); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// ConsumeInvite atomically reserves one use. It returns the invite ID and
// whether a use was available.
func (s *Store) ConsumeInvite(codeHash string, now int64) (int64, bool, error) {
	var id int64
	err := s.db.QueryRow(
		`UPDATE invites SET used_count = used_count + 1
		 WHERE code_hash = $1 AND revoked_at IS NULL AND expires_at > $2 AND used_count < max_uses
		 RETURNING id`,
		codeHash, now,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Store) ReleaseInvite(id int64) {
	_, _ = s.db.Exec(
		"UPDATE invites SET used_count = GREATEST(used_count - 1, 0) WHERE id = $1", id,
	)
}

func (s *Store) RevokeInvite(id int64, now int64) error {
	_, err := s.db.Exec(
		"UPDATE invites SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL", now, id,
	)
	return err
}

func (s *Store) CreateEmailVerification(userID int64, tokenHash string, now, expiresAt int64) error {
	_, err := s.db.Exec(
		`INSERT INTO email_verifications (user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, $4)`,
		userID, tokenHash, expiresAt, now,
	)
	return err
}

func (s *Store) ConsumeEmailVerification(tokenHash string, now int64) (int64, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var userID int64
	err = tx.QueryRow(
		`UPDATE email_verifications SET used_at = $1
		 WHERE token_hash = $2 AND used_at IS NULL AND expires_at > $1
		 RETURNING user_id`,
		now, tokenHash,
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(
		`UPDATE users
		 SET email_verified = TRUE,
		     status = CASE
		         WHEN disabled THEN 'disabled'
		         WHEN status = 'locked' THEN 'locked'
		         ELSE 'active'
		     END,
		     updated_at = $1
		 WHERE id = $2`,
		now, userID,
	); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

func (s *Store) StorePowChallenge(challenge string, expiresAt int64) error {
	_, err := s.db.Exec(
		"INSERT INTO pow_challenges (challenge, expires_at) VALUES ($1, $2)", challenge, expiresAt,
	)
	return err
}

func (s *Store) ConsumePowChallenge(challenge string, now int64) (bool, error) {
	result, err := s.db.Exec(
		"UPDATE pow_challenges SET used_at = $1 WHERE challenge = $2 AND used_at IS NULL AND expires_at > $1",
		now, challenge,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (s *Store) AddAudit(actorID int64, action, detail string, now int64) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_log (actor_id, action, detail, created_at) VALUES ($1, $2, $3, $4)",
		actorID, action, detail, now,
	)
	return err
}

func (s *Store) ListAudit(limit int) ([]AuditEntry, error) {
	rows, err := s.db.Query(
		`SELECT a.id, COALESCE(u.username, 'system'), a.action, a.detail, a.created_at
		 FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		 ORDER BY a.created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Actor, &entry.Action, &entry.Detail, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) LoadOrCreateSigningKeyPair() (*rsa.PrivateKey, string, error) {
	privatePEM, err := s.LoadOrCreateSecret("signing_key_pkcs8", func() (string, error) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", fmt.Errorf("generate signing key: %w", err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return "", fmt.Errorf("encode signing key: %w", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})), nil
	})
	if err != nil {
		return nil, "", err
	}
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, "", errors.New("stored signing key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse signing key: %w", err)
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, "", errors.New("stored signing key is not RSA")
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("encode signing public key: %w", err)
	}
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	return privateKey, publicPEM, nil
}

func (s *Store) LoadOrCreateSigningKey() (string, error) {
	_, publicPEM, err := s.LoadOrCreateSigningKeyPair()
	return publicPEM, err
}

func (s *Store) LoadOrCreateSecret(name string, generate func() (string, error)) (string, error) {
	value, err := s.loadSetting(name)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read setting %s: %w", name, err)
	}
	candidate, err := generate()
	if err != nil {
		return "", err
	}
	if _, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING",
		name, candidate,
	); err != nil {
		return "", fmt.Errorf("store setting %s: %w", name, err)
	}
	value, err = s.loadSetting(name)
	if err != nil {
		return "", fmt.Errorf("read setting %s: %w", name, err)
	}
	return value, nil
}

func (s *Store) loadSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = $1", key).Scan(&value)
	return value, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func userScanDest(user *User) []any {
	return []any{
		&user.ID, &user.UUID, &user.Username, &user.UsernameNormalized,
		&user.Email, &user.EmailNormalized, &user.PasswordHash, &user.EmailVerified,
		&user.Disabled, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin,
		&user.Role, &user.Status,
	}
}

func scanUser(row rowScanner) (*User, error) {
	var user User
	if err := row.Scan(userScanDest(&user)...); err != nil {
		return nil, err
	}
	return &user, nil
}
