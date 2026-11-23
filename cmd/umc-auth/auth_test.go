package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenAndUUIDHelpers(t *testing.T) {
	token := randomToken()
	if token == "" || token == randomToken() {
		t.Fatal("random tokens must be non-empty and unique")
	}
	if got := hashToken(token); len(got) != 64 || got != hashToken(token) {
		t.Fatalf("unexpected token hash %q", got)
	}
	uuid := newUserUUID()
	if len(uuid) != 32 || strings.ReplaceAll(uuid, "-", "") != uuid {
		t.Fatalf("unexpected UUID %q", uuid)
	}
	if prettyUUID(uuid) == uuid {
		t.Fatal("pretty UUID should include dashes")
	}
	if normalizeUUID(prettyUUID(uuid)) != uuid {
		t.Fatal("UUID normalization round trip failed")
	}
}

func TestUsernameValidation(t *testing.T) {
	for _, valid := range []string{"Player_1", "abc", "A123456789012345"} {
		if err := validateUsername(valid); err != nil {
			t.Errorf("expected %q to be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"ab", "a b", "玩家", strings.Repeat("a", 17)} {
		if err := validateUsername(invalid); err == nil {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "correct horse battery") {
		t.Fatal("correct password did not verify")
	}
	if verifyPassword(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
	if verifyPassword("not-a-hash", "anything") {
		t.Fatal("invalid hash verified")
	}
}

func newTestRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path, nil)
}

func TestClientIP(t *testing.T) {
	request := newTestRequest("/")
	request.RemoteAddr = "192.0.2.10:1234"
	if got := clientIP(request, false); got != "192.0.2.10" {
		t.Fatalf("unexpected direct IP %q", got)
	}
	request.Header.Set("X-Forwarded-For", "203.0.113.4, 10.0.0.1")
	if got := clientIP(request, false); got != "192.0.2.10" {
		t.Fatalf("proxy header should be ignored: %q", got)
	}
	if got := clientIP(request, true); got != "10.0.0.1" {
		t.Fatalf("unexpected forwarded IP %q", got)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://auth.example.com")
	t.Setenv("COOKIE_SECURE", "true")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OriginHost != "auth.example.com" || !cfg.SecureCookies {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	t.Setenv("PUBLIC_ORIGIN", "https://auth.example.com/path")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected a path in PUBLIC_ORIGIN to be rejected")
	}

	t.Setenv("PUBLIC_ORIGIN", "http://127.0.0.1:8080")
	t.Setenv("ALLOW_INSECURE_HTTP", "false")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected plain HTTP to be rejected in production mode")
	}
	t.Setenv("ALLOW_INSECURE_HTTP", "true")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicOrigin != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected development origin %q", cfg.PublicOrigin)
	}
}
