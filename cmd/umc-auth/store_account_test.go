package main

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := OpenStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for _, table := range []string{
		"auth_sessions", "access_tokens", "web_sessions", "joins", "invites",
		"email_verifications", "email_verification_codes", "password_reset_tokens",
		"login_attempts", "mail_outbox", "pow_challenges", "audit_log",
		"server_join_records", "server_instances", "minecraft_profiles", "users",
	} {
		if _, err := store.db.Exec("TRUNCATE " + table + " RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	if _, err := store.db.Exec("DELETE FROM settings"); err != nil {
		t.Fatal(err)
	}
	return store
}

func createTestUser(t *testing.T, store *Store, username, email string, admin bool) *User {
	t.Helper()
	hash, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	id, err := store.CreateUser(newUserUUID(), username, hash, email, true, admin, now)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.UserByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestAuthSessionRotationAndReuse(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "RotateUser", "rotate@example.com", false)
	now := time.Now().Unix()
	access := randomToken()
	refresh := randomToken()
	first, err := store.CreateAuthSession(AuthSessionInput{
		PublicID:         newUserUUID(),
		AccessHash:       hashToken(access),
		RefreshHash:      hashToken(refresh),
		Client:           "api",
		IP:               "192.0.2.10",
		UserAgent:        "test",
		AccessExpiresAt:  now + 900,
		RefreshExpiresAt: now + 3600,
	}, newUserUUID(), user.ID, now)
	if err != nil {
		t.Fatal(err)
	}

	foundUser, foundSession, err := store.AuthSessionByAccessToken(hashToken(access), now)
	if err != nil || foundUser.ID != user.ID || foundSession.PublicID != first.PublicID {
		t.Fatalf("unexpected session lookup: %+v %+v %v", foundUser, foundSession, err)
	}

	newAccess := randomToken()
	newRefresh := randomToken()
	rotatedUser, rotated, ok, reuse, err := store.RotateAuthSession(
		hashToken(refresh),
		AuthSessionInput{
			PublicID:         newUserUUID(),
			AccessHash:       hashToken(newAccess),
			RefreshHash:      hashToken(newRefresh),
			Client:           "api",
			IP:               "192.0.2.10",
			UserAgent:        "test",
			AccessExpiresAt:  now + 1800,
			RefreshExpiresAt: now + 7200,
		},
		now+1,
	)
	if err != nil || !ok || reuse || rotated == nil || rotatedUser.ID != user.ID {
		t.Fatalf("rotation failed: ok=%v reuse=%v user=%+v session=%+v err=%v", ok, reuse, rotatedUser, rotated, err)
	}
	if rotated.FamilyID != first.FamilyID {
		t.Fatalf("rotation changed the session family: %q != %q", rotated.FamilyID, first.FamilyID)
	}
	if _, _, err := store.AuthSessionByAccessToken(hashToken(access), now+2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("the old access token still works after rotation")
	}

	// Reusing the old refresh token must revoke the whole family.
	_, _, ok, reuse, err = store.RotateAuthSession(hashToken(refresh), AuthSessionInput{
		PublicID:         newUserUUID(),
		AccessHash:       hashToken(randomToken()),
		RefreshHash:      hashToken(randomToken()),
		Client:           "api",
		AccessExpiresAt:  now + 1800,
		RefreshExpiresAt: now + 7200,
	}, now+3)
	if err != nil || ok || !reuse {
		t.Fatalf("expected refresh reuse detection: ok=%v reuse=%v err=%v", ok, reuse, err)
	}
	if _, _, err := store.AuthSessionByAccessToken(hashToken(newAccess), now+4); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("the rotated access token survived refresh-token reuse")
	}
}

func TestEmailCodeVerification(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "CodeUser", "code@example.com", false)
	now := time.Now().Unix()
	if _, err := store.db.Exec(
		"UPDATE users SET email_verified = FALSE, status = 'pending' WHERE id = $1", user.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEmailCode(
		user.ID, user.EmailNormalized, emailPurposeRegister, hashToken("123456"),
		now, now+600, 3,
	); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := store.VerifyEmailCode(user.EmailNormalized, emailPurposeRegister, hashToken("000000"), now); err != nil || ok {
		t.Fatalf("wrong code was accepted: ok=%v err=%v", ok, err)
	}
	verifiedID, ok, err := store.VerifyEmailCode(user.EmailNormalized, emailPurposeRegister, hashToken("123456"), now)
	if err != nil || !ok || verifiedID != user.ID {
		t.Fatalf("correct code failed: id=%d ok=%v err=%v", verifiedID, ok, err)
	}
	if _, ok, _ := store.VerifyEmailCode(user.EmailNormalized, emailPurposeRegister, hashToken("123456"), now); ok {
		t.Fatal("verification code was reusable")
	}
	updated, err := store.UserByID(user.ID)
	if err != nil || !updated.EmailVerified || updated.Status != "active" {
		t.Fatalf("user was not activated: %+v %v", updated, err)
	}
}

func TestPasswordResetCode(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "ResetUser", "reset@example.com", false)
	now := time.Now().Unix()
	if err := store.CreatePasswordResetCode(user.ID, hashToken("654321"), now, now+600, 3); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.VerifyPasswordResetCode(user.EmailNormalized, hashToken("000000"), now); ok {
		t.Fatal("wrong reset code was accepted")
	}
	userID, ok, err := store.VerifyPasswordResetCode(user.EmailNormalized, hashToken("654321"), now)
	if err != nil || !ok || userID != user.ID {
		t.Fatalf("reset code failed: id=%d ok=%v err=%v", userID, ok, err)
	}
	if _, ok, _ := store.VerifyPasswordResetCode(user.EmailNormalized, hashToken("654321"), now); ok {
		t.Fatal("reset code was reusable")
	}
}

func TestLoginAttemptsAndAdminSearch(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "SearchUser", "search@example.com", true)
	now := time.Now().Unix()
	for index := 0; index < 3; index++ {
		if err := store.RecordLoginAttempt(user.ID, user.UsernameNormalized, "192.0.2.10", "test", false, "bad_password", now+int64(index)); err != nil {
			t.Fatal(err)
		}
	}
	count, err := store.RecentLoginFailures(user.UsernameNormalized, "192.0.2.10", now-60)
	if err != nil || count != 3 {
		t.Fatalf("unexpected failure count: %d %v", count, err)
	}
	users, total, err := store.SearchUsers("search", 10, 0)
	if err != nil || total != 1 || len(users) != 1 || users[0].ID != user.ID {
		t.Fatalf("unexpected search result: total=%d users=%+v err=%v", total, users, err)
	}
	stats, err := store.AdminStats()
	if err != nil || stats.Users != 1 || stats.ActiveUsers != 1 {
		t.Fatalf("unexpected stats: %+v %v", stats, err)
	}
}

func TestMailOutbox(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().Unix()
	id, err := store.EnqueueMail("user@example.com", "Subject", "Body", "test", now)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.ClaimDueMail(now, 10)
	if err != nil || len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("unexpected claim: %+v %v", entries, err)
	}
	if err := store.MarkMailFailed(id, "smtp unavailable", now+60); err != nil {
		t.Fatal(err)
	}
	if entries, err := store.ClaimDueMail(now, 10); err != nil || len(entries) != 0 {
		t.Fatalf("failed message was claimed before its retry time: %+v %v", entries, err)
	}
	if retried, err := store.RetryMail(id, now+120); err != nil || !retried {
		t.Fatalf("retry failed: %v %v", retried, err)
	}
	entries, err = store.ClaimDueMail(now+120, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("retried message was not claimable: %+v %v", entries, err)
	}
	if err := store.MarkMailSent(id, now+121); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListMail(10, "sent")
	if err != nil || len(list) != 1 || !list[0].SentAt.Valid {
		t.Fatalf("unexpected mail list: %+v %v", list, err)
	}
}

func TestNormalizationAndRoles(t *testing.T) {
	if normalizeUsername("  Player_1 ") != "player_1" {
		t.Fatal("username normalization failed")
	}
	if normalizeEmail("  User@Example.COM ") != "user@example.com" {
		t.Fatal("email normalization failed")
	}
	for _, role := range []string{"user", "moderator", "admin", "superadmin"} {
		if !validRole(role) {
			t.Fatalf("role %q should be valid", role)
		}
	}
	if validRole("owner") {
		t.Fatal("unknown role was accepted")
	}
}
