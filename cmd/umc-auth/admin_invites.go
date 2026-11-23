package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func inviteStatus(invite *Invite, now int64) string {
	switch {
	case invite.RevokedAt.Valid:
		return "revoked"
	case invite.ExpiresAt <= now:
		return "expired"
	case invite.UsedCount >= invite.MaxUses:
		return "exhausted"
	default:
		return "active"
	}
}

func apiInviteJSON(invite *Invite, now int64) map[string]any {
	var revokedAt any
	if invite.RevokedAt.Valid {
		revokedAt = invite.RevokedAt.Int64
	}
	return map[string]any{
		"id":          invite.ID,
		"code_prefix": invite.CodePrefix,
		"note":        invite.Note,
		"max_uses":    invite.MaxUses,
		"used_count":  invite.UsedCount,
		"expires_at":  invite.ExpiresAt,
		"revoked_at":  revokedAt,
		"created_at":  invite.CreatedAt,
		"status":      inviteStatus(invite, now),
	}
}

func (a *App) apiAdminInvites(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIAdmin(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100, 1, 500)
	invites, err := a.store.ListInvites(limit)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load invite codes")
		return
	}
	now := time.Now().Unix()
	result := make([]map[string]any, 0, len(invites))
	for index := range invites {
		result = append(result, apiInviteJSON(&invites[index], now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": result})
}

type apiCreateInvitesRequest struct {
	Count int    `json:"count"`
	Uses  int    `json:"uses"`
	Days  int    `json:"days"`
	Note  string `json:"note"`
}

func (a *App) apiAdminCreateInvites(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	var request apiCreateInvitesRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	if request.Count < 1 || request.Count > 50 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Count must be 1-50")
		return
	}
	if request.Uses < 1 || request.Uses > 100 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Uses must be 1-100")
		return
	}
	if request.Days < 1 || request.Days > 365 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Days must be 1-365")
		return
	}
	note := strings.TrimSpace(request.Note)
	if len(note) > 200 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Note is too long")
		return
	}

	now := time.Now()
	codes := make([]string, 0, request.Count)
	for index := 0; index < request.Count; index++ {
		code := newInviteCode()
		if err := a.store.CreateInvite(
			hashToken(code),
			code[:12],
			note,
			request.Uses,
			now.Add(time.Duration(request.Days)*24*time.Hour).Unix(),
			actor.ID,
			now.Unix(),
		); err != nil {
			apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not create invite codes")
			return
		}
		codes = append(codes, code)
	}
	_ = a.store.AddAudit(
		actor.ID, "invite.create",
		"count="+strconv.Itoa(request.Count)+" uses="+strconv.Itoa(request.Uses),
		now.Unix(),
	)

	invites, err := a.store.ListInvites(100)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Codes were created but the list could not be loaded")
		return
	}
	result := make([]map[string]any, 0, len(invites))
	for index := range invites {
		result = append(result, apiInviteJSON(&invites[index], now.Unix()))
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"codes":   codes,
		"invites": result,
	})
}

func (a *App) apiAdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Invalid invite id")
		return
	}
	now := time.Now().Unix()
	if err := a.store.RevokeInvite(id, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not revoke the invite")
		return
	}
	_ = a.store.AddAudit(actor.ID, "invite.revoke", "invite "+strconv.FormatInt(id, 10), now)
	w.WriteHeader(http.StatusNoContent)
}
