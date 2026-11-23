package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestAPIAdminInvites(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, registered := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "InviteAdmin",
		"email":    "invite-admin@example.com",
		"password": "correct horse battery",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %d %+v", response.StatusCode, registered)
	}
	access, _ := registered["access_token"].(string)

	response, created := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/admin/invites", access, map[string]any{
		"count": 2,
		"uses":  3,
		"days":  7,
		"note":  "test batch",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("invite create failed: %d %+v", response.StatusCode, created)
	}
	codes, _ := created["codes"].([]any)
	if len(codes) != 2 {
		t.Fatalf("expected two invite codes: %+v", created)
	}
	for _, value := range codes {
		code, _ := value.(string)
		if len(code) < 12 || code[:4] != "UMC-" {
			t.Fatalf("unexpected invite code format: %q", code)
		}
	}
	invites, _ := created["invites"].([]any)
	if len(invites) != 2 {
		t.Fatalf("expected two listed invites: %+v", created)
	}
	first := invites[0].(map[string]any)
	if first["status"] != "active" || first["note"] != "test batch" {
		t.Fatalf("unexpected invite: %+v", first)
	}
	inviteID := int64(first["id"].(float64))

	response, listed := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/invites", access, nil)
	if response.StatusCode != http.StatusOK || len(listed["invites"].([]any)) != 2 {
		t.Fatalf("invite list failed: %d %+v", response.StatusCode, listed)
	}

	response, _ = apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/invites/"+strconv.FormatInt(inviteID, 10)+"/revoke", access, nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("invite revoke failed: %d", response.StatusCode)
	}
	response, listed = apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/invites", access, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("invite list failed after revoke: %d", response.StatusCode)
	}
	found := false
	for _, value := range listed["invites"].([]any) {
		invite := value.(map[string]any)
		if int64(invite["id"].(float64)) == inviteID {
			found = true
			if invite["status"] != "revoked" {
				t.Fatalf("invite was not revoked: %+v", invite)
			}
		}
	}
	if !found {
		t.Fatalf("revoked invite disappeared from the list: %+v", listed)
	}
}
