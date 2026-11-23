package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestApp(t *testing.T, store *Store) *App {
	t.Helper()
	dummy, err := hashPassword("dummy-password")
	if err != nil {
		t.Fatal(err)
	}
	signingKey, publicPEM, err := store.LoadOrCreateSigningKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return &App{
		store: store,
		cfg: Config{
			ServerName:             "Test Auth",
			PublicOrigin:           "https://auth.example.com",
			UploadDir:              t.TempDir(),
			RegistrationMode:       "open",
			EmailVerification:      "off",
			CaptchaMode:            "none",
			SessionDays:            30,
			TokenDays:              30,
			AccessTokenMinutes:     15,
			RefreshTokenDays:       30,
			RegisterPerHour:        1000,
			RegisterPerDay:         1000,
			RegisterGlobalPerHour:  1000,
			LoginPerMinute:         1000,
			PowChallengePerMinute:  1000,
			ProfileLookupPerMinute: 1000,
			ForgotLimitPerMinute:   1000,
			VerifyLimitPerMinute:   1000,
			RefreshLimitPerMinute:  1000,
		},
		signingKey:        signingKey,
		pubKeyPEM:         publicPEM,
		dummyPasswordHash: dummy,
		registerHourly:    newSlidingWindow(1000, time.Hour),
		registerDaily:     newSlidingWindow(1000, 24*time.Hour),
		registerGlobal:    newSlidingWindow(1000, time.Hour),
		loginLimiter:      newSlidingWindow(1000, time.Minute),
		powLimiter:        newSlidingWindow(1000, time.Minute),
		apiLimiter:        newSlidingWindow(1000, time.Minute),
		forgotLimiter:     newSlidingWindow(1000, time.Minute),
		verifyLimiter:     newSlidingWindow(1000, time.Minute),
		refreshLimiter:    newSlidingWindow(1000, time.Minute),
	}
}

func apiRequest(t *testing.T, client *http.Client, method, url, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	if response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s %s: %v", method, url, err)
		}
	}
	response.Body.Close()
	return response, payload
}

func TestJSONOnlyRootAndFallback(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, root := apiRequest(t, client, http.MethodGet, server.URL+"/", "", nil)
	if response.StatusCode != http.StatusOK || root["service"] != "UMC Auth" {
		t.Fatalf("root did not return service JSON: %d %+v", response.StatusCode, root)
	}

	response, notFound := apiRequest(t, client, http.MethodGet, server.URL+"/login", "", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("page route did not return 404: %d", response.StatusCode)
	}
	errorBody, _ := notFound["error"].(map[string]any)
	if errorBody["code"] != "not_found" {
		t.Fatalf("page route did not return a JSON error: %+v", notFound)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("fallback was not JSON: %s", contentType)
	}
}

func TestAPIRegisterLoginRefreshLogout(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, meta := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/meta", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("meta failed: %d %+v", response.StatusCode, meta)
	}
	if meta["registration_open"] != true || meta["first_account"] != true {
		t.Fatalf("unexpected initial meta: %+v", meta)
	}
	if meta["bootstrap_required"] != false {
		t.Fatalf("bootstrap token should not be required: %+v", meta)
	}

	response, registered := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "ApiPlayer",
		"email":    "api-player@example.com",
		"password": "correct horse battery",
		"client":   "api",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %d %+v", response.StatusCode, registered)
	}
	response, meta = apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/meta", "", nil)
	if response.StatusCode != http.StatusOK || meta["first_account"] != false {
		t.Fatalf("meta did not reflect the first account: %d %+v", response.StatusCode, meta)
	}
	access, _ := registered["access_token"].(string)
	refresh, _ := registered["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("register did not return tokens: %+v", registered)
	}

	response, me := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/users/me", access, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("me failed: %d %+v", response.StatusCode, me)
	}
	if user, _ := me["user"].(map[string]any); user["username"] != "ApiPlayer" {
		t.Fatalf("unexpected current user: %+v", me)
	}
	response, profile := apiRequest(t, client, http.MethodGet,
		server.URL+"/api/v1/profiles/by-username/ApiPlayer", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("profile lookup failed: %d %+v", response.StatusCode, profile)
	}
	if profile["name"] != "ApiPlayer" {
		t.Fatalf("unexpected profile: %+v", profile)
	}

	response, refreshed := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": refresh,
		"client":        "api",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("refresh failed: %d %+v", response.StatusCode, refreshed)
	}
	newAccess, _ := refreshed["access_token"].(string)
	if newAccess == "" || newAccess == access {
		t.Fatalf("refresh did not rotate the access token: %+v", refreshed)
	}

	// Reusing the old refresh token revokes the family, including newAccess.
	response, reused := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": refresh,
	})
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh reuse was not rejected: %d %+v", response.StatusCode, reused)
	}
	response, _ = apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/users/me", newAccess, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("family revocation did not invalidate the new access token: %d", response.StatusCode)
	}

	response, loggedIn := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/login", "", map[string]any{
		"login":    "api-player@example.com",
		"password": "correct horse battery",
		"client":   "api",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d %+v", response.StatusCode, loggedIn)
	}
	loginAccess, _ := loggedIn["access_token"].(string)
	loginRefresh, _ := loggedIn["refresh_token"].(string)

	response, _ = apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/logout", "", map[string]any{
		"refresh_token": loginRefresh,
	})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout failed: %d", response.StatusCode)
	}
	response, _ = apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/users/me", loginAccess, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("access token survived logout: %d", response.StatusCode)
	}
}

func TestAPIAdminEndpoints(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, first := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "AdminPlayer",
		"email":    "admin@example.com",
		"password": "correct horse battery",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("admin register failed: %d %+v", response.StatusCode, first)
	}
	adminUser, _ := first["user"].(map[string]any)
	if adminUser["role"] != "superadmin" {
		t.Fatalf("first account should be superadmin: %+v", adminUser)
	}
	adminAccess, _ := first["access_token"].(string)

	response, second := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "ManagedPlayer",
		"email":    "managed@example.com",
		"password": "correct horse battery",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("second register failed: %d %+v", response.StatusCode, second)
	}
	managedUser, _ := second["user"].(map[string]any)
	managedUUID, _ := managedUser["uuid"].(string)

	response, stats := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/stats", adminAccess, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stats failed: %d %+v", response.StatusCode, stats)
	}
	if users, _ := stats["users"].(float64); users != 2 {
		t.Fatalf("unexpected user count: %+v", stats)
	}

	response, role := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/users/"+managedUUID+"/role", adminAccess,
		map[string]any{"role": "moderator"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("role change failed: %d %+v", response.StatusCode, role)
	}
	if changed, _ := role["user"].(map[string]any); changed["role"] != "moderator" {
		t.Fatalf("role was not changed: %+v", role)
	}

	response, disabled := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/users/"+managedUUID+"/status", adminAccess,
		map[string]any{"disabled": true})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("disable failed: %d %+v", response.StatusCode, disabled)
	}
	if changed, _ := disabled["user"].(map[string]any); changed["status"] != "disabled" {
		t.Fatalf("account was not disabled: %+v", disabled)
	}

	response, verified := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/users/"+managedUUID+"/verify-email", adminAccess, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("manual verification failed: %d %+v", response.StatusCode, verified)
	}
	if changed, _ := verified["user"].(map[string]any); changed["email_verified"] != true {
		t.Fatalf("email was not verified: %+v", verified)
	}

	response, reset := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/users/"+managedUUID+"/reset-password", adminAccess,
		map[string]any{})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin reset failed: %d %+v", response.StatusCode, reset)
	}
	if generated, _ := reset["generated_password"].(string); generated == "" {
		t.Fatalf("admin reset did not generate a password: %+v", reset)
	}

	response, audit := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/audit?limit=20", adminAccess, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("audit failed: %d %+v", response.StatusCode, audit)
	}
	entries, _ := audit["entries"].([]any)
	if len(entries) < 3 {
		t.Fatalf("expected admin actions in the audit log: %+v", audit)
	}
}
