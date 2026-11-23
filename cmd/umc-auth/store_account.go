package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type AuthSession struct {
	ID               int64
	PublicID         string
	FamilyID         string
	UserID           int64
	Client           string
	IP               string
	UserAgent        string
	CreatedAt        int64
	LastUsedAt       int64
	AccessExpiresAt  int64
	RefreshExpiresAt sql.NullInt64
	RevokedAt        sql.NullInt64
}

type AuthSessionInput struct {
	PublicID         string
	AccessHash       string
	RefreshHash      string
	Client           string
	IP               string
	UserAgent        string
	AccessExpiresAt  int64
	RefreshExpiresAt int64
}

type EmailCode struct {
	ID              int64
	UserID          int64
	EmailNormalized string
	Purpose         string
	CodeHash        string
	Attempts        int
	MaxAttempts     int
	SendCount       int
	SentAt          int64
	ExpiresAt       int64
	UsedAt          sql.NullInt64
	CreatedAt       int64
}

type MailOutboxEntry struct {
	ID            int64
	Recipient     string
	Subject       string
	Body          string
	Kind          string
	Status        string
	Attempts      int
	LastError     string
	NextAttemptAt int64
	CreatedAt     int64
	SentAt        sql.NullInt64
}

type AdminStats struct {
	Users        int64
	ActiveUsers  int64
	PendingUsers int64
	Disabled     int64
	PendingMail  int64
}

func (s *Store) CreateAuthSession(
	input AuthSessionInput,
	familyID string,
	userID int64,
	now int64,
) (*AuthSession, error) {
	var refreshExpires any
	if input.RefreshExpiresAt > 0 {
		refreshExpires = input.RefreshExpiresAt
	}
	var session AuthSession
	session.UserID = userID
	err := s.db.QueryRow(
		`INSERT INTO auth_sessions
		 (public_id, family_id, user_id, access_token_hash, refresh_token_hash, client, ip,
		  user_agent, created_at, last_used_at, access_expires_at, refresh_expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11)
		 RETURNING id, public_id, family_id, client, ip, user_agent, created_at, last_used_at,
		           access_expires_at, refresh_expires_at, revoked_at`,
		input.PublicID, familyID, userID, input.AccessHash, input.RefreshHash,
		input.Client, input.IP, input.UserAgent, now, input.AccessExpiresAt, refreshExpires,
	).Scan(
		&session.ID, &session.PublicID, &session.FamilyID, &session.Client, &session.IP,
		&session.UserAgent, &session.CreatedAt, &session.LastUsedAt,
		&session.AccessExpiresAt, &session.RefreshExpiresAt, &session.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) AuthSessionByAccessToken(tokenHash string, now int64) (*User, *AuthSession, error) {
	var user User
	var session AuthSession
	dest := append(userScanDest(&user),
		&session.ID, &session.PublicID, &session.FamilyID, &session.Client, &session.IP,
		&session.UserAgent, &session.CreatedAt, &session.LastUsedAt,
		&session.AccessExpiresAt, &session.RefreshExpiresAt, &session.RevokedAt,
	)
	err := s.db.QueryRow(
		`SELECT `+userColumnsPrefixed+`,
		        s.id, s.public_id, s.family_id, s.client, s.ip, s.user_agent, s.created_at,
		        s.last_used_at, s.access_expires_at, s.refresh_expires_at, s.revoked_at
		 FROM auth_sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.access_token_hash = $1 AND s.revoked_at IS NULL
		   AND s.access_expires_at > $2 AND u.disabled = FALSE
		   AND u.email_verified = TRUE AND u.status = 'active'`,
		tokenHash, now,
	).Scan(dest...)
	if err != nil {
		return nil, nil, err
	}
	session.UserID = user.ID
	return &user, &session, nil
}

// RotateAuthSession consumes a refresh token and issues a new access and refresh
// token pair. Reusing a revoked refresh token revokes the whole session family.
func (s *Store) RotateAuthSession(
	oldRefreshHash string,
	next AuthSessionInput,
	now int64,
) (*User, *AuthSession, bool, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, false, false, err
	}
	defer tx.Rollback()

	var oldID, oldUserID int64
	var oldFamily string
	var oldRevoked, oldRefreshExpires sql.NullInt64
	err = tx.QueryRow(
		`SELECT id, user_id, family_id, revoked_at, refresh_expires_at
		 FROM auth_sessions WHERE refresh_token_hash = $1 FOR UPDATE`,
		oldRefreshHash,
	).Scan(&oldID, &oldUserID, &oldFamily, &oldRevoked, &oldRefreshExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, false, nil
	}
	if err != nil {
		return nil, nil, false, false, err
	}

	if oldRevoked.Valid {
		if _, err := tx.Exec(
			"UPDATE auth_sessions SET revoked_at = $1 WHERE family_id = $2 AND revoked_at IS NULL",
			now, oldFamily,
		); err != nil {
			return nil, nil, false, false, err
		}
		user, err := userByIDTx(tx, oldUserID)
		if err != nil {
			return nil, nil, false, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, false, false, err
		}
		return user, nil, false, true, nil
	}
	if !oldRefreshExpires.Valid || oldRefreshExpires.Int64 <= now {
		return nil, nil, false, false, nil
	}

	var refreshExpires any
	if next.RefreshExpiresAt > 0 {
		refreshExpires = next.RefreshExpiresAt
	}
	var session AuthSession
	session.UserID = oldUserID
	err = tx.QueryRow(
		`INSERT INTO auth_sessions
		 (public_id, family_id, user_id, access_token_hash, refresh_token_hash, client, ip,
		  user_agent, created_at, last_used_at, access_expires_at, refresh_expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11)
		 RETURNING id, public_id, family_id, client, ip, user_agent, created_at, last_used_at,
		           access_expires_at, refresh_expires_at, revoked_at`,
		next.PublicID, oldFamily, oldUserID, next.AccessHash, next.RefreshHash,
		next.Client, next.IP, next.UserAgent, now, next.AccessExpiresAt, refreshExpires,
	).Scan(
		&session.ID, &session.PublicID, &session.FamilyID, &session.Client, &session.IP,
		&session.UserAgent, &session.CreatedAt, &session.LastUsedAt,
		&session.AccessExpiresAt, &session.RefreshExpiresAt, &session.RevokedAt,
	)
	if err != nil {
		return nil, nil, false, false, err
	}
	if _, err := tx.Exec(
		"UPDATE auth_sessions SET revoked_at = $1, replaced_by = $2 WHERE id = $3",
		now, session.ID, oldID,
	); err != nil {
		return nil, nil, false, false, err
	}
	user, err := userByIDTx(tx, oldUserID)
	if err != nil {
		return nil, nil, false, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, false, err
	}
	return user, &session, true, false, nil
}

func userByIDTx(tx *sql.Tx, userID int64) (*User, error) {
	var user User
	if err := tx.QueryRow(
		`SELECT `+userColumns+` FROM users WHERE id = $1`, userID,
	).Scan(userScanDest(&user)...); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) RevokeAuthSessionByAccessHash(tokenHash string, now int64) error {
	_, err := s.db.Exec(
		"UPDATE auth_sessions SET revoked_at = $1 WHERE access_token_hash = $2 AND revoked_at IS NULL",
		now, tokenHash,
	)
	return err
}

func (s *Store) RevokeAuthSessionByRefreshHash(tokenHash string, now int64) error {
	_, err := s.db.Exec(
		"UPDATE auth_sessions SET revoked_at = $1 WHERE refresh_token_hash = $2 AND revoked_at IS NULL",
		now, tokenHash,
	)
	return err
}

func (s *Store) RevokeAuthSessionByPublicID(userID int64, publicID string, now int64) (bool, error) {
	result, err := s.db.Exec(
		`UPDATE auth_sessions SET revoked_at = $1
		 WHERE user_id = $2 AND public_id = $3 AND revoked_at IS NULL`,
		now, userID, publicID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (s *Store) ListAuthSessions(userID int64, now int64) ([]AuthSession, error) {
	rows, err := s.db.Query(
		`SELECT id, public_id, family_id, client, ip, user_agent, created_at, last_used_at,
		        access_expires_at, refresh_expires_at, revoked_at
		 FROM auth_sessions
		 WHERE user_id = $1 AND revoked_at IS NULL
		   AND COALESCE(refresh_expires_at, access_expires_at) > $2
		 ORDER BY created_at DESC LIMIT 100`,
		userID, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []AuthSession
	for rows.Next() {
		session := AuthSession{UserID: userID}
		if err := rows.Scan(
			&session.ID, &session.PublicID, &session.FamilyID, &session.Client, &session.IP,
			&session.UserAgent, &session.CreatedAt, &session.LastUsedAt,
			&session.AccessExpiresAt, &session.RefreshExpiresAt, &session.RevokedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) TouchAuthSession(sessionID int64, now int64) {
	_, _ = s.db.Exec(
		"UPDATE auth_sessions SET last_used_at = $1 WHERE id = $2 AND last_used_at < $1 - 60",
		now, sessionID,
	)
}

func (s *Store) CreateEmailCode(
	userID int64,
	emailNormalized, purpose, codeHash string,
	now, expiresAt int64,
	maxAttempts int,
) error {
	if _, err := s.db.Exec(
		`UPDATE email_verification_codes SET used_at = $1
		 WHERE email_normalized = $2 AND purpose = $3 AND used_at IS NULL`,
		now, emailNormalized, purpose,
	); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO email_verification_codes
		 (user_id, email_normalized, purpose, code_hash, max_attempts, sent_at, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $6)`,
		userID, emailNormalized, purpose, codeHash, maxAttempts, now, expiresAt,
	)
	return err
}

func (s *Store) LastEmailCodeSentAt(emailNormalized, purpose string) (int64, error) {
	var sentAt int64
	err := s.db.QueryRow(
		`SELECT COALESCE(MAX(sent_at), 0) FROM email_verification_codes
		 WHERE email_normalized = $1 AND purpose = $2`,
		emailNormalized, purpose,
	).Scan(&sentAt)
	return sentAt, err
}

// VerifyEmailCode consumes the latest code for an email and purpose. It returns
// success only once; attempts are incremented even when the code is wrong.
func (s *Store) VerifyEmailCode(
	emailNormalized, purpose, codeHash string,
	now int64,
) (int64, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var id int64
	var userID sql.NullInt64
	var storedHash string
	var attempts, maxAttempts int
	var expiresAt int64
	var usedAt sql.NullInt64
	err = tx.QueryRow(
		`SELECT id, user_id, code_hash, attempts, max_attempts, expires_at, used_at
		 FROM email_verification_codes
		 WHERE email_normalized = $1 AND purpose = $2
		 ORDER BY id DESC LIMIT 1 FOR UPDATE`,
		emailNormalized, purpose,
	).Scan(&id, &userID, &storedHash, &attempts, &maxAttempts, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if usedAt.Valid || expiresAt <= now || attempts >= maxAttempts || !userID.Valid {
		return 0, false, nil
	}
	if _, err := tx.Exec(
		"UPDATE email_verification_codes SET attempts = attempts + 1 WHERE id = $1", id,
	); err != nil {
		return 0, false, err
	}
	if !constantTimeEqual(storedHash, codeHash) {
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	if _, err := tx.Exec(
		"UPDATE email_verification_codes SET used_at = $1 WHERE id = $2", now, id,
	); err != nil {
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
		now, userID.Int64,
	); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return userID.Int64, true, nil
}

func (s *Store) CreatePasswordResetCode(
	userID int64,
	codeHash string,
	now, expiresAt int64,
	maxAttempts int,
) error {
	if _, err := s.db.Exec(
		"UPDATE password_reset_tokens SET used_at = $1 WHERE user_id = $2 AND used_at IS NULL",
		now, userID,
	); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO password_reset_tokens
		 (user_id, code_hash, max_attempts, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, codeHash, maxAttempts, expiresAt, now,
	)
	return err
}

func (s *Store) VerifyPasswordResetCode(
	emailNormalized, codeHash string,
	now int64,
) (int64, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var id, userID int64
	var storedHash string
	var attempts, maxAttempts int
	var expiresAt int64
	var usedAt sql.NullInt64
	err = tx.QueryRow(
		`SELECT t.id, t.user_id, t.code_hash, t.attempts, t.max_attempts, t.expires_at, t.used_at
		 FROM password_reset_tokens t JOIN users u ON u.id = t.user_id
		 WHERE u.email_normalized = $1
		 ORDER BY t.id DESC LIMIT 1 FOR UPDATE`,
		emailNormalized,
	).Scan(&id, &userID, &storedHash, &attempts, &maxAttempts, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if usedAt.Valid || expiresAt <= now || attempts >= maxAttempts {
		return 0, false, nil
	}
	if _, err := tx.Exec(
		"UPDATE password_reset_tokens SET attempts = attempts + 1 WHERE id = $1", id,
	); err != nil {
		return 0, false, err
	}
	if !constantTimeEqual(storedHash, codeHash) {
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	if _, err := tx.Exec(
		"UPDATE password_reset_tokens SET used_at = $1 WHERE id = $2", now, id,
	); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

func (s *Store) RecordLoginAttempt(
	userID int64,
	usernameNormalized, ip, userAgent string,
	success bool,
	reason string,
	now int64,
) error {
	var userValue any
	if userID > 0 {
		userValue = userID
	}
	_, err := s.db.Exec(
		`INSERT INTO login_attempts
		 (user_id, username_normalized, ip, user_agent, success, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		userValue, usernameNormalized, ip, truncate(userAgent, 512), success, reason, now,
	)
	return err
}

func (s *Store) RecentLoginFailures(usernameNormalized, ip string, since int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM login_attempts
		 WHERE username_normalized = $1 AND ip = $2 AND success = FALSE AND created_at > $3`,
		usernameNormalized, ip, since,
	).Scan(&count)
	return count, err
}

func (s *Store) EnqueueMail(recipient, subject, body, kind string, now int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO mail_outbox
		 (recipient, subject, body, kind, status, next_attempt_at, created_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $5) RETURNING id`,
		recipient, subject, body, kind, now,
	).Scan(&id)
	return id, err
}

func (s *Store) ClaimDueMail(now int64, limit int) ([]MailOutboxEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, recipient, subject, body, kind, status, attempts, last_error,
		        next_attempt_at, created_at, sent_at
		 FROM mail_outbox
		 WHERE status IN ('pending', 'failed') AND next_attempt_at <= $1 AND attempts < 5
		 ORDER BY next_attempt_at ASC LIMIT $2`,
		now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []MailOutboxEntry
	for rows.Next() {
		var entry MailOutboxEntry
		if err := rows.Scan(
			&entry.ID, &entry.Recipient, &entry.Subject, &entry.Body, &entry.Kind,
			&entry.Status, &entry.Attempts, &entry.LastError, &entry.NextAttemptAt,
			&entry.CreatedAt, &entry.SentAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) MarkMailSent(id int64, now int64) error {
	_, err := s.db.Exec(
		`UPDATE mail_outbox
		 SET status = 'sent', sent_at = $1, last_error = '', body = ''
		 WHERE id = $2`,
		now, id,
	)
	return err
}

func (s *Store) MarkMailFailed(id int64, message string, nextAttempt int64) error {
	_, err := s.db.Exec(
		`UPDATE mail_outbox
		 SET status = 'failed', attempts = attempts + 1, last_error = $1, next_attempt_at = $2
		 WHERE id = $3`,
		truncate(message, 1000), nextAttempt, id,
	)
	return err
}

func (s *Store) ListMail(limit int, status string) ([]MailOutboxEntry, error) {
	query := `SELECT id, recipient, subject, body, kind, status, attempts, last_error,
	                 next_attempt_at, created_at, sent_at
	          FROM mail_outbox`
	args := []any{}
	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT $" + fmt.Sprint(len(args)+1)
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []MailOutboxEntry
	for rows.Next() {
		var entry MailOutboxEntry
		if err := rows.Scan(
			&entry.ID, &entry.Recipient, &entry.Subject, &entry.Body, &entry.Kind,
			&entry.Status, &entry.Attempts, &entry.LastError, &entry.NextAttemptAt,
			&entry.CreatedAt, &entry.SentAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) RetryMail(id int64, now int64) (bool, error) {
	result, err := s.db.Exec(
		`UPDATE mail_outbox
		 SET status = 'pending', attempts = 0, last_error = '', next_attempt_at = $1
		 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (s *Store) SearchUsers(query string, limit, offset int) ([]User, int, error) {
	pattern := "%" + normalizeUsername(query) + "%"
	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM users
		 WHERE $1 = '%%' OR username_normalized LIKE $1 OR email_normalized LIKE $1 OR uuid LIKE $1`,
		pattern,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(
		`SELECT `+userColumns+` FROM users
		 WHERE $1 = '%%' OR username_normalized LIKE $1 OR email_normalized LIKE $1 OR uuid LIKE $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		pattern, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *user)
	}
	return users, total, rows.Err()
}

func (s *Store) AdminStats() (AdminStats, error) {
	var stats AdminStats
	err := s.db.QueryRow(
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE status = 'active'),
		        COUNT(*) FILTER (WHERE status = 'pending'),
		        COUNT(*) FILTER (WHERE status = 'disabled')
		 FROM users`,
	).Scan(&stats.Users, &stats.ActiveUsers, &stats.PendingUsers, &stats.Disabled)
	if err != nil {
		return stats, err
	}
	err = s.db.QueryRow(
		"SELECT COUNT(*) FROM mail_outbox WHERE status IN ('pending', 'failed')",
	).Scan(&stats.PendingMail)
	return stats, err
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return strings.TrimSpace(value[:maximum])
}
