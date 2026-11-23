package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxTextureBytes = 1 << 20
	maxUploadBytes  = 2 << 20
)

type texturesPayload struct {
	Timestamp   int64          `json:"timestamp"`
	ProfileID   string         `json:"profileId"`
	ProfileName string         `json:"profileName"`
	Textures    map[string]any `json:"textures"`
}

func (a *App) textureURL(kind, uuid, hash string) string {
	if hash == "" {
		return ""
	}
	return fmt.Sprintf("%s/textures/%s/%s/%s", a.cfg.PublicOrigin, kind, uuid, hash)
}

// applyTextureURLs rebuilds texture URLs from the stored hashes so a change to
// PUBLIC_ORIGIN does not break existing profiles or signed properties.
func (a *App) applyTextureURLs(uuid string, profile *MinecraftProfile) *MinecraftProfile {
	if profile == nil {
		return nil
	}
	profile.SkinURL = a.textureURL("skin", uuid, profile.SkinHash)
	profile.CapeURL = a.textureURL("cape", uuid, profile.CapeHash)
	return profile
}

func (a *App) texturesProperty(user *User, profile *MinecraftProfile) (map[string]any, error) {
	textures := map[string]any{}
	updated := time.Now().UnixMilli()
	if profile != nil {
		if profile.UpdatedAt > 0 {
			updated = profile.UpdatedAt * 1000
		}
		if skinURL := a.textureURL("skin", user.UUID, profile.SkinHash); skinURL != "" {
			skin := map[string]any{"url": skinURL}
			if profile.SkinModel == "slim" {
				skin["metadata"] = map[string]any{"model": "slim"}
			}
			textures["SKIN"] = skin
		}
		if capeURL := a.textureURL("cape", user.UUID, profile.CapeHash); capeURL != "" {
			textures["CAPE"] = map[string]any{"url": capeURL}
		}
	}

	payload := texturesPayload{
		Timestamp:   updated,
		ProfileID:   user.UUID,
		ProfileName: user.Username,
		Textures:    textures,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	value := base64.StdEncoding.EncodeToString(raw)
	if a.signingKey == nil {
		return map[string]any{"name": "textures", "value": value}, nil
	}
	digest := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.signingKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      "textures",
		"value":     value,
		"signature": base64.StdEncoding.EncodeToString(signature),
	}, nil
}

func (a *App) profileResponse(user *User) map[string]any {
	profile, err := a.minecraftProfile(user)
	if err != nil {
		profile = nil
	}
	properties := []any{}
	if property, err := a.texturesProperty(user, profile); err == nil {
		properties = append(properties, property)
	}
	return map[string]any{
		"id":         user.UUID,
		"name":       user.Username,
		"properties": properties,
	}
}

func (a *App) minecraftProfile(user *User) (*MinecraftProfile, error) {
	profile, err := a.store.MinecraftProfileByUserID(user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := a.store.EnsureMinecraftProfile(
			user.ID, user.Username, user.UUID, time.Now().Unix(),
		); err != nil {
			return nil, err
		}
		return a.store.MinecraftProfileByUserID(user.ID)
	}
	if err != nil {
		return nil, err
	}
	return a.applyTextureURLs(user.UUID, profile), nil
}

func apiMinecraftJSON(profile *MinecraftProfile) map[string]any {
	if profile == nil {
		return nil
	}
	return map[string]any{
		"uuid":       profile.PlayerUUID,
		"name":       profile.PlayerName,
		"skin_url":   profile.SkinURL,
		"cape_url":   profile.CapeURL,
		"skin_model": profile.SkinModel,
		"updated_at": profile.UpdatedAt,
	}
}

func (a *App) updatedMinecraftJSON(user *User) map[string]any {
	updated, err := a.store.MinecraftProfileByUserID(user.ID)
	if err != nil {
		return nil
	}
	return apiMinecraftJSON(a.applyTextureURLs(user.UUID, updated))
}

func (a *App) apiMinecraftProfile(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	profile, err := a.minecraftProfile(user)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the Minecraft profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": apiMinecraftJSON(profile)})
}

func readTextureUpload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return nil, errors.New("malformed multipart upload")
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, errors.New("a PNG file is required")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTextureBytes+1))
	if err != nil {
		return nil, errors.New("could not read the uploaded file")
	}
	if len(data) > maxTextureBytes {
		return nil, errors.New("texture must be 1 MB or smaller")
	}
	return data, nil
}

func pngDimensions(data []byte) (int, int, error) {
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return 0, 0, errors.New("file is not a PNG image")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, errors.New("file is not a valid PNG image")
	}
	return config.Width, config.Height, nil
}

func isTextureHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func isTextureUUID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func (a *App) saveTexture(kind, uuid string, data []byte) (string, string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	directory := filepath.Join(a.cfg.UploadDir, kind+"s")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, uuid+"-"+hash+".png")
	if _, err := os.Stat(path); err == nil {
		return a.textureURL(kind, uuid, hash), hash, nil
	}
	temp, err := os.CreateTemp(directory, ".upload-*")
	if err != nil {
		return "", "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", "", err
	}
	if err := temp.Close(); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return "", "", err
	}
	if err := os.Rename(tempName, path); err != nil {
		return "", "", err
	}
	return a.textureURL(kind, uuid, hash), hash, nil
}

func (a *App) removeTexture(kind, uuid, hash string) {
	if hash == "" {
		return
	}
	_ = os.Remove(filepath.Join(a.cfg.UploadDir, kind+"s", uuid+"-"+hash+".png"))
}

func (a *App) apiUploadSkin(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	data, err := readTextureUpload(w, r)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", err.Error())
		return
	}
	width, height, err := pngDimensions(data)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", err.Error())
		return
	}
	if width != 64 || (height != 64 && height != 32) {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", "Skin must be a 64x64 or 64x32 PNG")
		return
	}
	model := strings.ToLower(strings.TrimSpace(r.FormValue("model")))
	if model == "" {
		model = "classic"
	}
	if model != "classic" && model != "slim" {
		apiV1Error(w, http.StatusBadRequest, "invalid_model", "Skin model must be classic or slim")
		return
	}
	profile, err := a.minecraftProfile(user)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the Minecraft profile")
		return
	}
	url, hash, err := a.saveTexture("skin", user.UUID, data)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not store the skin")
		return
	}
	now := time.Now().Unix()
	if err := a.store.UpdateMinecraftSkin(user.ID, url, hash, model, now); err != nil {
		a.removeTexture("skin", user.UUID, hash)
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the skin")
		return
	}
	if profile.SkinHash != "" && profile.SkinHash != hash {
		a.removeTexture("skin", user.UUID, profile.SkinHash)
	}
	_ = a.store.AddAudit(user.ID, "profile.skin_update", hash, now)
	writeJSON(w, http.StatusOK, map[string]any{"profile": a.updatedMinecraftJSON(user)})
}

func (a *App) apiDeleteSkin(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	profile, err := a.minecraftProfile(user)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the Minecraft profile")
		return
	}
	now := time.Now().Unix()
	if err := a.store.ClearMinecraftSkin(user.ID, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not remove the skin")
		return
	}
	a.removeTexture("skin", user.UUID, profile.SkinHash)
	_ = a.store.AddAudit(user.ID, "profile.skin_remove", "", now)
	writeJSON(w, http.StatusOK, map[string]any{"profile": a.updatedMinecraftJSON(user)})
}

func (a *App) apiUploadCape(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	data, err := readTextureUpload(w, r)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", err.Error())
		return
	}
	width, height, err := pngDimensions(data)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", err.Error())
		return
	}
	if width != 64 || (height != 32 && height != 64) {
		apiV1Error(w, http.StatusBadRequest, "invalid_texture", "Cape must be a 64x32 or 64x64 PNG")
		return
	}
	profile, err := a.minecraftProfile(user)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the Minecraft profile")
		return
	}
	url, hash, err := a.saveTexture("cape", user.UUID, data)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not store the cape")
		return
	}
	now := time.Now().Unix()
	if err := a.store.UpdateMinecraftCape(user.ID, url, hash, now); err != nil {
		a.removeTexture("cape", user.UUID, hash)
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the cape")
		return
	}
	if profile.CapeHash != "" && profile.CapeHash != hash {
		a.removeTexture("cape", user.UUID, profile.CapeHash)
	}
	_ = a.store.AddAudit(user.ID, "profile.cape_update", hash, now)
	writeJSON(w, http.StatusOK, map[string]any{"profile": a.updatedMinecraftJSON(user)})
}

func (a *App) apiDeleteCape(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	profile, err := a.minecraftProfile(user)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the Minecraft profile")
		return
	}
	now := time.Now().Unix()
	if err := a.store.ClearMinecraftCape(user.ID, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not remove the cape")
		return
	}
	a.removeTexture("cape", user.UUID, profile.CapeHash)
	_ = a.store.AddAudit(user.ID, "profile.cape_remove", "", now)
	writeJSON(w, http.StatusOK, map[string]any{"profile": a.updatedMinecraftJSON(user)})
}

func (a *App) texture(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(r.PathValue("kind"))
	if kind != "skin" && kind != "cape" {
		http.NotFound(w, r)
		return
	}
	uuid := normalizeUUID(r.PathValue("uuid"))
	hash := strings.ToLower(r.PathValue("hash"))
	if !isTextureUUID(uuid) || !isTextureHash(hash) {
		http.NotFound(w, r)
		return
	}
	profile, err := a.store.MinecraftProfileByUUID(uuid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	expected := profile.SkinHash
	if kind == "cape" {
		expected = profile.CapeHash
	}
	if expected == "" || expected != hash {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(a.cfg.UploadDir, kind+"s", uuid+"-"+hash+".png")
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}
