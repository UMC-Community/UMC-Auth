package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const serviceVersion = "0.1.0"

type authenticateRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ClientToken string `json:"clientToken"`
	RequestUser bool   `json:"requestUser"`
}

type refreshRequest struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
	RequestUser bool   `json:"requestUser"`
}

type tokenRequest struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
}

type signoutRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type profileRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type joinRequest struct {
	AccessToken     string     `json:"accessToken"`
	SelectedProfile profileRef `json:"selectedProfile"`
	ServerID        string     `json:"serverId"`
}

func (a *App) metadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"meta": map[string]any{
			"serverName":            a.cfg.ServerName,
			"implementationType":    "UMC Auth",
			"implementationVersion": serviceVersion,
		},
		"skinDomains":        []string{a.cfg.OriginHost},
		"signaturePublickey": a.pubKeyPEM,
	})
}

func (a *App) authenticate(w http.ResponseWriter, r *http.Request) {
	var request authenticateRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.loginLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "TooManyRequests", "Too many authentication attempts")
		return
	}
	user, err := a.store.UserByUsername(strings.TrimSpace(request.Username))
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid username or password")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	if !verifyPassword(user.PasswordHash, request.Password) {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid username or password")
		return
	}
	if user.Disabled || !user.EmailVerified {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid username or password")
		return
	}

	clientToken := strings.TrimSpace(request.ClientToken)
	if clientToken == "" {
		clientToken = randomToken()
	}
	accessToken := randomToken()
	now := time.Now().Unix()
	expiresAt := now + int64(a.cfg.TokenDays)*86_400
	if err := a.store.InsertAccessToken(user.ID, hashToken(accessToken), clientToken, now, expiresAt); err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}

	profile := a.profileResponse(user)
	response := map[string]any{
		"accessToken":       accessToken,
		"clientToken":       clientToken,
		"availableProfiles": []any{profile},
		"selectedProfile":   profile,
	}
	if request.RequestUser {
		response["user"] = userJSON(user.UUID)
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	user, storedClient, err := a.accessTokenUser(request.AccessToken, request.ClientToken)
	if err != nil {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid token")
		return
	}
	clientToken := strings.TrimSpace(request.ClientToken)
	if clientToken == "" {
		clientToken = storedClient
	}
	accessToken := randomToken()
	now := time.Now().Unix()
	expiresAt := now + int64(a.cfg.TokenDays)*86_400
	if err := a.store.RotateAccessToken(
		hashToken(request.AccessToken), hashToken(accessToken), clientToken, user.ID, now, expiresAt,
	); err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	profile := a.profileResponse(user)
	response := map[string]any{
		"accessToken":     accessToken,
		"clientToken":     clientToken,
		"selectedProfile": profile,
	}
	if request.RequestUser {
		response["user"] = userJSON(user.UUID)
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) validate(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if _, _, err := a.accessTokenUser(request.AccessToken, request.ClientToken); err != nil {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) invalidate(w http.ResponseWriter, r *http.Request) {
	var request tokenRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if _, _, err := a.accessTokenUser(request.AccessToken, request.ClientToken); err == nil {
		if err := a.store.DeleteAccessToken(hashToken(request.AccessToken)); err != nil {
			apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) signout(w http.ResponseWriter, r *http.Request) {
	var request signoutRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.loginLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "TooManyRequests", "Too many authentication attempts")
		return
	}
	user, err := a.store.UserByUsername(strings.TrimSpace(request.Username))
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid username or password")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	if !verifyPassword(user.PasswordHash, request.Password) {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid username or password")
		return
	}
	if err := a.store.DeleteUserTokens(user.ID); err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) join(w http.ResponseWriter, r *http.Request) {
	var request joinRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.ServerID == "" || len(request.ServerID) > 128 {
		apiError(w, http.StatusBadRequest, "IllegalArgumentException", "Invalid serverId")
		return
	}
	user, _, err := a.accessTokenUser(request.AccessToken, "")
	if err != nil {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException", "Invalid token")
		return
	}
	if normalizeUUID(request.SelectedProfile.ID) != user.UUID ||
		!strings.EqualFold(request.SelectedProfile.Name, user.Username) {
		apiError(w, http.StatusForbidden, "ForbiddenOperationException",
			"The selected profile does not belong to this account")
		return
	}
	ip := clientIP(r, a.cfg.TrustProxyHeaders)
	now := time.Now().Unix()
	if err := a.store.InsertJoin(user.ID, user.Username, request.ServerID, ip, now); err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	_ = a.store.RecordServerJoin(user.ID, request.ServerID, user.Username, ip, now)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) hasJoined(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	serverID := r.URL.Query().Get("serverId")
	if username == "" || serverID == "" {
		apiError(w, http.StatusBadRequest, "IllegalArgumentException",
			"username and serverId are required")
		return
	}
	since := time.Now().Unix() - 60
	uuid, _, ok, err := a.store.ConsumeJoin(
		serverID, username, since, r.URL.Query().Get("ip"),
	)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	user, err := a.store.UserByUUID(uuid)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, a.profileResponse(user))
}

func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	user, err := a.store.UserByUUID(r.PathValue("uuid"))
	if errors.Is(err, sql.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
		return
	}
	writeJSON(w, http.StatusOK, a.profileResponse(user))
}

func (a *App) profilesByName(w http.ResponseWriter, r *http.Request) {
	var names []string
	if !decodeJSON(w, r, &names) {
		return
	}
	if !a.allowLimited(a.apiLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "TooManyRequests", "Too many profile lookups")
		return
	}
	profiles := make([]any, 0, len(names))
	for index, name := range names {
		if index >= 10 {
			break
		}
		user, err := a.store.UserByUsername(strings.TrimSpace(name))
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			apiError(w, http.StatusInternalServerError, "InternalServerError", "An internal error occurred")
			return
		}
		profiles = append(profiles, a.profileResponse(user))
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (a *App) accessTokenUser(accessToken, clientToken string) (*User, string, error) {
	if accessToken == "" {
		return nil, "", sql.ErrNoRows
	}
	user, storedClient, err := a.store.AccessTokenUser(hashToken(accessToken), time.Now().Unix())
	if err != nil {
		return nil, "", err
	}
	if clientToken != "" && !constantTimeEqual(clientToken, storedClient) {
		return nil, "", sql.ErrNoRows
	}
	return user, storedClient, nil
}

func userJSON(uuid string) map[string]any {
	return map[string]any{
		"id":         uuid,
		"properties": []any{},
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		apiError(w, http.StatusBadRequest, "IllegalArgumentException", "Malformed JSON request")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":        code,
		"errorMessage": message,
	})
}
