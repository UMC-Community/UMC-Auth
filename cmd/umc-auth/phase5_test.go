package main

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestMinecraftProfileAndTexturesSignature(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "TextureUser", "texture@example.com", false)
	profile, err := store.MinecraftProfileByUserID(user.ID)
	if err != nil || profile.PlayerUUID != user.UUID {
		t.Fatalf("user did not get a Minecraft profile: %+v %v", profile, err)
	}

	now := time.Now().Unix()
	skinHash := strings.Repeat("a", 64)
	// The stored URL may come from an older PUBLIC_ORIGIN; responses must
	// rebuild it from the hash and the current origin.
	if err := store.UpdateMinecraftSkin(
		user.ID, "http://old.example.com/textures/skin/"+user.UUID+"/"+skinHash, skinHash, "slim", now,
	); err != nil {
		t.Fatal(err)
	}
	app := newTestApp(t, store)
	response := app.profileResponse(user)
	properties, ok := response["properties"].([]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("unexpected properties: %+v", response)
	}
	property := properties[0].(map[string]any)
	value, _ := property["value"].(string)
	signature, _ := property["signature"].(string)
	if value == "" || signature == "" {
		t.Fatalf("textures property was not signed: %+v", property)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(value))
	if err := rsa.VerifyPKCS1v15(&app.signingKey.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("texture signature did not verify: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["profileId"] != user.UUID || payload["profileName"] != user.Username {
		t.Fatalf("unexpected texture payload: %+v", payload)
	}
	textures := payload["textures"].(map[string]any)
	skin := textures["SKIN"].(map[string]any)
	skinURL, _ := skin["url"].(string)
	if !strings.HasPrefix(skinURL, "https://auth.example.com/textures/skin/") {
		t.Fatalf("skin URL was not rebuilt from the current origin: %q", skinURL)
	}
	metadata := skin["metadata"].(map[string]any)
	if metadata["model"] != "slim" {
		t.Fatalf("skin model missing: %+v", skin)
	}
}

func TestTextureUploadAndServe(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, registered := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "SkinPlayer",
		"email":    "skin@example.com",
		"password": "correct horse battery",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %d %+v", response.StatusCode, registered)
	}
	access, _ := registered["access_token"].(string)

	pngData := testPNG(t, 64, 64)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "skin.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(pngData); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteField("model", "slim")
	_ = writer.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users/me/skin", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+access)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("skin upload failed: %d %+v", response.StatusCode, uploaded)
	}
	profile := uploaded["profile"].(map[string]any)
	skinURL, _ := profile["skin_url"].(string)
	if skinURL == "" || profile["skin_model"] != "slim" {
		t.Fatalf("unexpected profile after upload: %+v", profile)
	}
	parsed, err := url.Parse(skinURL)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Get(server.URL + parsed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("texture fetch failed: %d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected texture content type: %s", response.Header.Get("Content-Type"))
	}
	fetched, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fetched, pngData) {
		t.Fatalf("served texture differs from the uploaded file")
	}

	response, _ = apiRequest(t, client, http.MethodDelete, server.URL+"/api/v1/users/me/skin", access, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("skin delete failed: %d", response.StatusCode)
	}
}

func TestServerInstancesAndJoinRecords(t *testing.T) {
	store := newTestStore(t)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, registered := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", "", map[string]any{
		"username": "ServerAdmin",
		"email":    "server-admin@example.com",
		"password": "correct horse battery",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %d %+v", response.StatusCode, registered)
	}
	access, _ := registered["access_token"].(string)

	response, created := apiRequest(t, client, http.MethodPost, server.URL+"/api/v1/admin/servers", access, map[string]any{
		"name":        "Survival",
		"address":     "mc.example.com:25565",
		"description": "Main world",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("server create failed: %d %+v", response.StatusCode, created)
	}
	serverObject := created["server"].(map[string]any)
	serverID := int64(serverObject["id"].(float64))

	response, listed := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/servers", access, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("server list failed: %d %+v", response.StatusCode, listed)
	}
	if len(listed["servers"].([]any)) != 1 {
		t.Fatalf("unexpected server list: %+v", listed)
	}

	response, updated := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/admin/servers/"+strconv.FormatInt(serverID, 10)+"/status", access,
		map[string]any{"status": "disabled"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("server status failed: %d %+v", response.StatusCode, updated)
	}
	if updated["server"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("server was not disabled: %+v", updated)
	}

	user, err := store.UserByUsername("ServerAdmin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordServerJoin(user.ID, "server-hash", user.Username, "192.0.2.10", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	response, joins := apiRequest(t, client, http.MethodGet, server.URL+"/api/v1/admin/servers/joins?limit=10", access, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("join list failed: %d %+v", response.StatusCode, joins)
	}
	if len(joins["joins"].([]any)) != 1 {
		t.Fatalf("unexpected join records: %+v", joins)
	}

	response, _ = apiRequest(t, client, http.MethodDelete,
		server.URL+"/api/v1/admin/servers/"+strconv.FormatInt(serverID, 10), access, nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("server delete failed: %d", response.StatusCode)
	}
}

func TestYggdrasilJoinRecordsHistory(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "JoinPlayer", "join@example.com", false)
	app := newTestApp(t, store)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := server.Client()

	response, authenticated := apiRequest(t, client, http.MethodPost,
		server.URL+"/api/yggdrasil/authserver/authenticate", "", map[string]any{
			"agent":       map[string]any{"name": "Minecraft", "version": 1},
			"username":    user.Username,
			"password":    "correct horse battery",
			"clientToken": "join-test-client",
		})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Yggdrasil authenticate failed: %d %+v", response.StatusCode, authenticated)
	}
	access, _ := authenticated["accessToken"].(string)
	selected, _ := authenticated["selectedProfile"].(map[string]any)
	if access == "" || selected == nil {
		t.Fatalf("authenticate did not return a token and profile: %+v", authenticated)
	}
	response, _ = apiRequest(t, client, http.MethodPost,
		server.URL+"/api/yggdrasil/sessionserver/session/minecraft/join", "", map[string]any{
			"accessToken":     access,
			"selectedProfile": selected,
			"serverId":        "server-hash",
		})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("Yggdrasil join failed: %d", response.StatusCode)
	}
	records, err := store.ListServerJoinRecords(10, "server-hash")
	if err != nil || len(records) != 1 {
		t.Fatalf("join history was not recorded: %+v %v", records, err)
	}
	if records[0].Username != user.Username || records[0].IP == "" {
		t.Fatalf("unexpected join record: %+v", records[0])
	}
}

func TestAuditSearch(t *testing.T) {
	store := newTestStore(t)
	user := createTestUser(t, store, "AuditUser", "audit@example.com", true)
	now := time.Now().Unix()
	if err := store.AddAudit(user.ID, "server.create", "Survival", now); err != nil {
		t.Fatal(err)
	}
	if err := store.AddAudit(user.ID, "profile.skin_update", "abc", now+1); err != nil {
		t.Fatal(err)
	}
	serverEntries, err := store.SearchAudit("server", 10)
	if err != nil || len(serverEntries) != 1 || serverEntries[0].Action != "server.create" {
		t.Fatalf("unexpected audit search: %+v %v", serverEntries, err)
	}
	all, err := store.SearchAudit("", 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("unexpected audit list: %+v %v", all, err)
	}
}
