package main

import (
	"database/sql"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestStoreIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := OpenStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec("TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("DELETE FROM settings"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	hash, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	uuid := newUserUUID()
	userID, err := store.CreateUser(uuid, "TestPlayer", hash, "", true, true, now)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.UserByUsername("testplayer")
	if err != nil || user.ID != userID || !user.IsAdmin {
		t.Fatalf("unexpected user: %+v, %v", user, err)
	}
	if _, err := store.CreateUser(newUserUUID(), "TESTPLAYER", hash, "", true, false, now); err == nil {
		t.Fatal("case-insensitive duplicate username was accepted")
	}

	accessToken := randomToken()
	if err := store.InsertAccessToken(userID, hashToken(accessToken), "client-1", now, now+3600); err != nil {
		t.Fatal(err)
	}
	tokenUser, client, err := store.AccessTokenUser(hashToken(accessToken), now)
	if err != nil || tokenUser.ID != userID || client != "client-1" {
		t.Fatalf("unexpected token user: %+v, %q, %v", tokenUser, client, err)
	}
	rotated := randomToken()
	if err := store.RotateAccessToken(hashToken(accessToken), hashToken(rotated), "client-1", userID, now, now+3600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AccessTokenUser(hashToken(accessToken), now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("old access token still works after rotation")
	}

	sessionToken := randomToken()
	if err := store.CreateSession(userID, hashToken(sessionToken), "csrf-1", now, now+3600); err != nil {
		t.Fatal(err)
	}
	sessionUser, csrf, err := store.SessionUser(hashToken(sessionToken), now)
	if err != nil || sessionUser.ID != userID || csrf != "csrf-1" {
		t.Fatalf("unexpected session: %+v, %q, %v", sessionUser, csrf, err)
	}

	if err := store.InsertJoin(userID, "TestPlayer", "server-1", "192.0.2.10", now); err != nil {
		t.Fatal(err)
	}
	joinedUUID, joinedName, ok, err := store.ConsumeJoin("server-1", "testplayer", now-60, "192.0.2.10")
	if err != nil || !ok || joinedUUID != uuid || joinedName != "TestPlayer" {
		t.Fatalf("unexpected join result: %q %q %v %v", joinedUUID, joinedName, ok, err)
	}
	if _, _, ok, _ := store.ConsumeJoin("server-1", "testplayer", now-60, ""); ok {
		t.Fatal("join record was replayed")
	}

	inviteHash := hashToken("UMC-TEST-CODE")
	if err := store.CreateInvite(inviteHash, "UMC-TEST", "test", 1, now+3600, userID, now); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ConsumeInvite(inviteHash, now); err != nil || !ok {
		t.Fatalf("first invite use failed: %v", err)
	}
	if _, ok, err := store.ConsumeInvite(inviteHash, now); err != nil || ok {
		t.Fatalf("invite was reusable: %v", err)
	}

	verificationToken := randomToken()
	if err := store.CreateEmailVerification(userID, hashToken(verificationToken), now, now+3600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ConsumeEmailVerification(hashToken(verificationToken), now); err != nil || !ok {
		t.Fatalf("email verification failed: %v", err)
	}
	if _, ok, _ := store.ConsumeEmailVerification(hashToken(verificationToken), now); ok {
		t.Fatal("email verification token was reusable")
	}

	pow := NewPoW(store, []byte("test-secret"), 6)
	powToken, challenge, difficulty, _, err := pow.Issue("192.0.2.10", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var nonce uint64
	for !hasLeadingZeroBits(challenge+":"+strconv.FormatUint(nonce, 10), difficulty) {
		nonce++
	}
	if err := pow.Verify(powToken, strconv.FormatUint(nonce, 10), "192.0.2.10", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := pow.Verify(powToken, strconv.FormatUint(nonce, 10), "192.0.2.10", time.Now()); err == nil {
		t.Fatal("proof-of-work token was reusable")
	}

	firstKey, err := store.LoadOrCreateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := store.LoadOrCreateSigningKey()
	if err != nil || firstKey != secondKey || len(firstKey) == 0 {
		t.Fatalf("signing key was not reused: %v", err)
	}
}
