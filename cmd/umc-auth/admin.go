package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (*User, string, bool) {
	user, csrf, err := a.currentSession(r)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load the session")
		return nil, "", false
	}
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, "", false
	}
	if !user.IsAdmin {
		a.webError(w, r, http.StatusForbidden, "Administrator access required")
		return nil, "", false
	}
	return user, csrf, true
}

func (a *App) adminBase(user *User) BaseData {
	return BaseData{
		ServerName: a.cfg.ServerName,
		Username:   user.Username,
		LoggedIn:   true,
		IsAdmin:    true,
	}
}

func (a *App) adminDashboard(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	count, err := a.store.CountUsers()
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load statistics")
		return
	}
	audit, err := a.store.ListAudit(10)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load audit log")
		return
	}
	a.templates.render(w, http.StatusOK, "admin_dashboard", AdminData{
		BaseData:          a.adminBase(user),
		CSRF:              csrf,
		RegistrationMode:  a.cfg.RegistrationMode,
		CaptchaMode:       a.cfg.CaptchaMode,
		EmailVerification: a.cfg.EmailVerification,
		UserCount:         count,
		Audit:             audit,
	})
}

func (a *App) adminInvites(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	a.renderInvites(w, r, user, csrf, nil, "")
}

func (a *App) renderInvites(
	w http.ResponseWriter,
	r *http.Request,
	user *User,
	csrf string,
	newCodes []string,
	message string,
) {
	invites, err := a.store.ListInvites(100)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load invites")
		return
	}
	a.templates.render(w, http.StatusOK, "admin_invites", AdminData{
		BaseData: a.adminBase(user),
		CSRF:     csrf,
		Invites:  invites,
		NewCodes: newCodes,
		Error:    message,
	})
}

func (a *App) adminCreateInvites(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed form")
		return
	}
	if !constantTimeEqual(csrf, r.PostFormValue("csrf")) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token")
		return
	}
	count, err := formInt(r, "count", 1, 50)
	if err != nil {
		a.renderInvites(w, r, user, csrf, nil, err.Error())
		return
	}
	uses, err := formInt(r, "uses", 1, 100)
	if err != nil {
		a.renderInvites(w, r, user, csrf, nil, err.Error())
		return
	}
	days, err := formInt(r, "days", 1, 365)
	if err != nil {
		a.renderInvites(w, r, user, csrf, nil, err.Error())
		return
	}
	note := strings.TrimSpace(r.PostFormValue("note"))
	if len(note) > 200 {
		a.renderInvites(w, r, user, csrf, nil, "Note is too long")
		return
	}
	now := time.Now()
	codes := make([]string, 0, count)
	for index := 0; index < count; index++ {
		code := newInviteCode()
		if err := a.store.CreateInvite(
			hashToken(code),
			code[:12],
			note,
			uses,
			now.Add(time.Duration(days)*24*time.Hour).Unix(),
			user.ID,
			now.Unix(),
		); err != nil {
			a.renderInvites(w, r, user, csrf, codes, "Could not create an invite code")
			return
		}
		codes = append(codes, code)
	}
	_ = a.store.AddAudit(user.ID, "invite.create",
		"created "+strconv.Itoa(len(codes))+" code(s), "+strconv.Itoa(uses)+" use(s) each", now.Unix())
	a.renderInvites(w, r, user, csrf, codes, "")
}

func (a *App) adminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed form")
		return
	}
	if !constantTimeEqual(csrf, r.PostFormValue("csrf")) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token")
		return
	}
	id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if err != nil {
		a.webError(w, r, http.StatusBadRequest, "Invalid invite")
		return
	}
	if err := a.store.RevokeInvite(id, time.Now().Unix()); err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not revoke the invite")
		return
	}
	_ = a.store.AddAudit(user.ID, "invite.revoke", "invite "+strconv.FormatInt(id, 10), time.Now().Unix())
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}

func (a *App) adminUsers(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	users, err := a.store.ListUsers(100)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load users")
		return
	}
	a.templates.render(w, http.StatusOK, "admin_users", AdminData{
		BaseData: a.adminBase(user),
		CSRF:     csrf,
		Users:    users,
	})
}

func (a *App) adminUserStatus(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed form")
		return
	}
	if !constantTimeEqual(csrf, r.PostFormValue("csrf")) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token")
		return
	}
	id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if err != nil {
		a.webError(w, r, http.StatusBadRequest, "Invalid user")
		return
	}
	disabled := r.PostFormValue("disabled") == "true"
	target, err := a.store.UserByID(id)
	if err != nil {
		a.webError(w, r, http.StatusNotFound, "User not found")
		return
	}
	if target.IsAdmin || target.ID == user.ID {
		a.webError(w, r, http.StatusForbidden, "Administrators cannot be disabled here")
		return
	}
	if err := a.store.SetUserDisabled(id, disabled, time.Now().Unix()); err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not update the user")
		return
	}
	action := "user.enable"
	if disabled {
		action = "user.disable"
	}
	_ = a.store.AddAudit(user.ID, action, target.Username, time.Now().Unix())
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (a *App) adminAudit(w http.ResponseWriter, r *http.Request) {
	user, csrf, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	audit, err := a.store.ListAudit(100)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load audit log")
		return
	}
	a.templates.render(w, http.StatusOK, "admin_audit", AdminData{
		BaseData: a.adminBase(user),
		CSRF:     csrf,
		Audit:    audit,
	})
}

func formInt(r *http.Request, name string, minimum, maximum int) (int, error) {
	value, err := strconv.Atoi(r.PostFormValue(name))
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return value, nil
}
