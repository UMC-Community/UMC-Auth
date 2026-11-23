package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

const (
	refreshCookie     = "umc_auth_refresh"
	sessionHintCookie = "umc_auth_session_hint"

	emailPurposeRegister = "register"
	emailPurposeReset    = "password_reset"

	emailCodeMinutes    = 10
	emailCodeMaxTries   = 5
	emailResendSeconds  = 60
	loginFailureWindow  = 15 * 60
	loginFailureMaximum = 10
)

type apiV1ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiV1ErrorBody struct {
	Error apiV1ErrorDetail `json:"error"`
}

func apiV1Error(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiV1ErrorBody{Error: apiV1ErrorDetail{Code: code, Message: message}})
}

func decodeV1JSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_json", "Malformed JSON request")
		return false
	}
	return true
}

func apiUserJSON(user *User) map[string]any {
	return map[string]any{
		"id":             user.UUID,
		"uuid":           user.UUID,
		"username":       user.Username,
		"email":          user.Email,
		"email_verified": user.EmailVerified,
		"role":           user.Role,
		"status":         user.Status,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	}
}

func apiSessionJSON(session AuthSession, current bool) map[string]any {
	expiresAt := session.AccessExpiresAt
	if session.RefreshExpiresAt.Valid {
		expiresAt = session.RefreshExpiresAt.Int64
	}
	return map[string]any{
		"id":           session.PublicID,
		"client":       session.Client,
		"ip":           session.IP,
		"user_agent":   session.UserAgent,
		"created_at":   session.CreatedAt,
		"last_used_at": session.LastUsedAt,
		"expires_at":   expiresAt,
		"current":      current,
	}
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if len(header) < 7 || !strings.EqualFold(header[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func normalizeClient(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "web":
		return "web"
	case "cli":
		return "cli"
	case "launcher":
		return "launcher"
	default:
		return "api"
	}
}

func (a *App) apiUser(r *http.Request) (*User, *AuthSession, error) {
	token := bearerToken(r)
	if token == "" {
		return nil, nil, sql.ErrNoRows
	}
	now := time.Now().Unix()
	user, session, err := a.store.AuthSessionByAccessToken(hashToken(token), now)
	if err != nil {
		return nil, nil, err
	}
	a.store.TouchAuthSession(session.ID, now)
	return user, session, nil
}

func (a *App) requireAPIUser(w http.ResponseWriter, r *http.Request) (*User, *AuthSession, bool) {
	user, session, err := a.apiUser(r)
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusUnauthorized, "unauthorized", "A valid bearer token is required")
		return nil, nil, false
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the session")
		return nil, nil, false
	}
	return user, session, true
}

func (a *App) requireAPIAdmin(w http.ResponseWriter, r *http.Request) (*User, *AuthSession, bool) {
	user, session, ok := a.requireAPIUser(w, r)
	if !ok {
		return nil, nil, false
	}
	if !user.IsAdmin && !isAdminRole(user.Role) {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return nil, nil, false
	}
	return user, session, true
}

func (a *App) requireAPIStaff(w http.ResponseWriter, r *http.Request) (*User, *AuthSession, bool) {
	user, session, ok := a.requireAPIUser(w, r)
	if !ok {
		return nil, nil, false
	}
	if !user.IsAdmin && user.Role != "moderator" && !isAdminRole(user.Role) {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Staff access required")
		return nil, nil, false
	}
	return user, session, true
}

func (a *App) setRefreshCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    token,
		Path:     "/api/v1/auth",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	// A non-sensitive hint lets the SPA skip a doomed refresh call on guest
	// pages without exposing the refresh token to JavaScript.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookie,
		Value:    "1",
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: false,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) issueAPISession(
	w http.ResponseWriter,
	r *http.Request,
	user *User,
	client string,
) (map[string]any, error) {
	if user.Disabled || user.Status == "disabled" || user.Status == "locked" {
		return nil, errAccountUnavailable
	}
	now := time.Now().Unix()
	accessToken := randomToken()
	refreshToken := randomToken()
	accessExpires := now + int64(a.cfg.AccessTokenMinutes)*60
	refreshExpires := now + int64(a.cfg.RefreshTokenDays)*86_400
	session, err := a.store.CreateAuthSession(
		AuthSessionInput{
			PublicID:         newUserUUID(),
			AccessHash:       hashToken(accessToken),
			RefreshHash:      hashToken(refreshToken),
			Client:           client,
			IP:               clientIP(r, a.cfg.TrustProxyHeaders),
			UserAgent:        truncate(r.UserAgent(), 512),
			AccessExpiresAt:  accessExpires,
			RefreshExpiresAt: refreshExpires,
		},
		newUserUUID(),
		user.ID,
		now,
	)
	if err != nil {
		return nil, err
	}
	web := client == "web"
	if web {
		a.setRefreshCookie(w, refreshToken, a.cfg.RefreshTokenDays*86_400)
	}
	response := map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int64(a.cfg.AccessTokenMinutes) * 60,
		"user":         apiUserJSON(user),
		"session":      apiSessionJSON(*session, true),
	}
	if !web {
		response["refresh_token"] = refreshToken
		response["refresh_expires_in"] = int64(a.cfg.RefreshTokenDays) * 86_400
	}
	return response, nil
}

var errAccountUnavailable = errors.New("account is not available")

func newNumericCode(digits int) string {
	const alphabet = "0123456789"
	bytes := randomBytes(digits)
	var builder strings.Builder
	for _, value := range bytes {
		builder.WriteByte(alphabet[int(value)%len(alphabet)])
	}
	return builder.String()
}

func newPassword(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	bytes := randomBytes(length)
	var builder strings.Builder
	for _, value := range bytes {
		builder.WriteByte(alphabet[int(value)%len(alphabet)])
	}
	return builder.String()
}

func (a *App) enqueueVerificationEmail(user *User) error {
	if a.mailer == nil {
		return errAccountUnavailable
	}
	code := newNumericCode(6)
	now := time.Now()
	if err := a.store.CreateEmailCode(
		user.ID, user.EmailNormalized, emailPurposeRegister, hashToken(code),
		now.Unix(), now.Add(emailCodeMinutes*time.Minute).Unix(), emailCodeMaxTries,
	); err != nil {
		return err
	}
	body := fmt.Sprintf(
		"Hello %s,\r\n\r\n"+
			"Your %s verification code is:\r\n\r\n    %s\r\n\r\n"+
			"It expires in %d minutes. If you did not request this account, ignore this message.\r\n",
		user.Username, a.cfg.ServerName, code, emailCodeMinutes,
	)
	_, err := a.store.EnqueueMail(
		user.Email, a.cfg.ServerName+" verification code", body, emailPurposeRegister, now.Unix(),
	)
	return err
}

func (a *App) enqueuePasswordResetEmail(user *User) error {
	if a.mailer == nil {
		return errAccountUnavailable
	}
	code := newNumericCode(6)
	now := time.Now()
	if err := a.store.CreatePasswordResetCode(
		user.ID, hashToken(code), now.Unix(),
		now.Add(emailCodeMinutes*time.Minute).Unix(), emailCodeMaxTries,
	); err != nil {
		return err
	}
	body := fmt.Sprintf(
		"Hello %s,\r\n\r\n"+
			"Your %s password reset code is:\r\n\r\n    %s\r\n\r\n"+
			"It expires in %d minutes. If you did not request a reset, ignore this message.\r\n",
		user.Username, a.cfg.ServerName, code, emailCodeMinutes,
	)
	_, err := a.store.EnqueueMail(
		user.Email, a.cfg.ServerName+" password reset", body, emailPurposeReset, now.Unix(),
	)
	return err
}

func (a *App) mailWorker(ctx context.Context) {
	a.processMailOutbox()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.processMailOutbox()
		}
	}
}

func (a *App) processMailOutbox() {
	now := time.Now().Unix()
	entries, err := a.store.ClaimDueMail(now, 10)
	if err != nil {
		slog.Warn("mail outbox claim failed", "error", err)
		return
	}
	for _, entry := range entries {
		if err := a.mailer.Send(entry.Recipient, entry.Subject, entry.Body); err != nil {
			attempts := entry.Attempts + 1
			delay := time.Minute * time.Duration(1<<uint(minInt(attempts, 6)))
			if delay > time.Hour {
				delay = time.Hour
			}
			_ = a.store.MarkMailFailed(entry.ID, err.Error(), now+int64(delay.Seconds()))
			slog.Warn("mail delivery failed", "id", entry.ID, "attempts", attempts, "error", err)
			continue
		}
		if err := a.store.MarkMailSent(entry.ID, now); err != nil {
			slog.Warn("mail outbox update failed", "id", entry.ID, "error", err)
		}
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

type apiRegisterRequest struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	InviteCode     string `json:"invite_code"`
	BootstrapToken string `json:"bootstrap_token"`
	TurnstileToken string `json:"turnstile_token"`
	PowToken       string `json:"pow_token"`
	PowNonce       string `json:"pow_nonce"`
	Client         string `json:"client"`
}

func (a *App) apiRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":   "UMC Auth",
		"api":       a.cfg.PublicOrigin + "/api/v1",
		"yggdrasil": a.yggdrasilURL(),
	})
}

func (a *App) apiNotFound(w http.ResponseWriter, r *http.Request) {
	apiV1Error(w, http.StatusNotFound, "not_found", "This host only serves the UMC Auth API")
}

func (a *App) apiMeta(w http.ResponseWriter, r *http.Request) {
	if !a.allowLimited(a.apiLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many metadata requests")
		return
	}
	count, err := a.store.CountUsers()
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load server metadata")
		return
	}
	registrationOpen := a.cfg.RegistrationMode != "closed" || count == 0
	writeJSON(w, http.StatusOK, map[string]any{
		"server_name":        a.cfg.ServerName,
		"registration_mode":  a.cfg.RegistrationMode,
		"registration_open":  registrationOpen,
		"first_account":      count == 0,
		"bootstrap_required": a.cfg.BootstrapToken != "" && count == 0,
		"email_verification": a.cfg.EmailVerification,
		"captcha_mode":       a.cfg.CaptchaMode,
		"turnstile_site_key": a.cfg.TurnstileSiteKey,
		"auth_origin":        a.cfg.PublicOrigin,
		"yggdrasil_url":      a.yggdrasilURL(),
	})
}

func (a *App) apiRegister(w http.ResponseWriter, r *http.Request) {
	var request apiRegisterRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	userCount, err := a.store.CountUsers()
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load registration settings")
		return
	}
	firstUser := userCount == 0
	if a.cfg.RegistrationMode == "closed" && !firstUser {
		apiV1Error(w, http.StatusForbidden, "registration_closed", "Registration is closed")
		return
	}
	if !a.registrationAllowed(r) {
		w.Header().Set("Retry-After", "3600")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many registration attempts")
		return
	}

	username := strings.TrimSpace(request.Username)
	if err := validateUsername(username); err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if err := validatePassword(request.Password); err != nil {
		apiV1Error(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err != nil || address.Address == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
		return
	}
	email := address.Address
	requireVerification := a.cfg.EmailVerification == "required" && !firstUser
	if requireVerification && a.mailer == nil {
		apiV1Error(w, http.StatusServiceUnavailable, "email_unavailable",
			"Email delivery is not configured")
		return
	}

	if firstUser && a.cfg.BootstrapToken != "" &&
		!constantTimeEqual(a.cfg.BootstrapToken, request.BootstrapToken) {
		apiV1Error(w, http.StatusForbidden, "bootstrap_required", "Invalid bootstrap token")
		return
	}
	if !firstUser {
		switch a.cfg.CaptchaMode {
		case "turnstile":
			if a.turnstile == nil {
				apiV1Error(w, http.StatusInternalServerError, "captcha_unavailable",
					"Turnstile is not configured correctly")
				return
			}
			if err := a.turnstile.Verify(
				r.Context(), request.TurnstileToken,
				clientIP(r, a.cfg.TrustProxyHeaders), "register",
			); err != nil {
				apiV1Error(w, http.StatusBadRequest, "captcha_failed", err.Error())
				return
			}
		case "pow":
			if a.poW == nil {
				apiV1Error(w, http.StatusInternalServerError, "captcha_unavailable",
					"Proof-of-work is not configured correctly")
				return
			}
			if err := a.poW.Verify(
				request.PowToken, request.PowNonce,
				clientIP(r, a.cfg.TrustProxyHeaders), time.Now(),
			); err != nil {
				apiV1Error(w, http.StatusBadRequest, "captcha_failed", err.Error())
				return
			}
		}
	}

	var inviteID int64
	inviteUsed := false
	releaseInvite := func() {
		if inviteUsed {
			a.store.ReleaseInvite(inviteID)
			inviteUsed = false
		}
	}
	if !firstUser && a.cfg.RegistrationMode == "invite" {
		code := strings.ToUpper(strings.TrimSpace(request.InviteCode))
		if code == "" {
			apiV1Error(w, http.StatusForbidden, "invite_required", "An invite code is required")
			return
		}
		id, ok, err := a.store.ConsumeInvite(hashToken(code), time.Now().Unix())
		if err != nil {
			apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not validate the invite code")
			return
		}
		if !ok {
			apiV1Error(w, http.StatusForbidden, "invalid_invite", "The invite code is invalid or expired")
			return
		}
		inviteID = id
		inviteUsed = true
	}

	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		releaseInvite()
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not create the account")
		return
	}
	now := time.Now()
	emailVerified := firstUser || !requireVerification
	userID, err := a.store.CreateUser(
		newUserUUID(), username, passwordHash, email, emailVerified, firstUser, now.Unix(),
	)
	if err != nil {
		releaseInvite()
		if isUniqueViolation(err) {
			apiV1Error(w, http.StatusConflict, "registration_conflict",
				"That username or email is already registered")
			return
		}
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not create the account")
		return
	}
	if firstUser {
		if err := a.store.SetUserRole(userID, "superadmin", now.Unix()); err != nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not initialize the administrator")
			return
		}
	}
	user, err := a.store.UserByID(userID)
	if err != nil {
		releaseInvite()
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the account")
		return
	}

	if requireVerification {
		if err := a.enqueueVerificationEmail(user); err != nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			apiV1Error(w, http.StatusServiceUnavailable, "email_unavailable",
				"Could not queue the verification email")
			return
		}
		_ = a.store.AddAudit(userID, "user.register", username, now.Unix())
		writeJSON(w, http.StatusAccepted, map[string]any{
			"user":                  apiUserJSON(user),
			"verification_required": true,
			"message":               "Check your email for a six-digit verification code",
		})
		return
	}

	_ = a.store.AddAudit(userID, "user.register", username, now.Unix())
	response, err := a.issueAPISession(w, r, user, normalizeClient(request.Client))
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not start a session")
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

type apiVerifyEmailRequest struct {
	Email  string `json:"email"`
	Code   string `json:"code"`
	Client string `json:"client"`
}

func (a *App) apiVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var request apiVerifyEmailRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.verifyLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many verification attempts")
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err != nil || address.Address == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
		return
	}
	code := strings.TrimSpace(request.Code)
	if len(code) != 6 {
		apiV1Error(w, http.StatusBadRequest, "invalid_code", "The code is invalid or expired")
		return
	}
	userID, ok, err := a.store.VerifyEmailCode(
		normalizeEmail(address.Address), emailPurposeRegister, hashToken(code), time.Now().Unix(),
	)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not verify the code")
		return
	}
	if !ok {
		apiV1Error(w, http.StatusBadRequest, "invalid_code", "The code is invalid or expired")
		return
	}
	user, err := a.store.UserByID(userID)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the account")
		return
	}
	_ = a.store.AddAudit(userID, "user.verify_email", "", time.Now().Unix())
	client := normalizeClient(request.Client)
	if strings.TrimSpace(request.Client) == "" {
		client = normalizeClient(r.Header.Get("X-UMC-Client"))
	}
	response, err := a.issueAPISession(w, r, user, client)
	if err != nil {
		apiV1Error(w, http.StatusForbidden, "account_unavailable", "The account is not available")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type apiEmailRequest struct {
	Email string `json:"email"`
}

func (a *App) apiResendVerification(w http.ResponseWriter, r *http.Request) {
	var request apiEmailRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.verifyLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many verification attempts")
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err != nil || address.Address == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
		return
	}
	if a.mailer == nil {
		apiV1Error(w, http.StatusServiceUnavailable, "email_unavailable", "Email delivery is not configured")
		return
	}
	user, err := a.store.UserByEmail(address.Address)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the account")
		return
	}
	if err == nil && !user.EmailVerified && !user.Disabled {
		now := time.Now().Unix()
		last, err := a.store.LastEmailCodeSentAt(user.EmailNormalized, emailPurposeRegister)
		if err == nil && last > 0 && now-last < emailResendSeconds {
			w.Header().Set("Retry-After", "60")
			apiV1Error(w, http.StatusTooManyRequests, "resend_too_soon", "Wait before requesting another code")
			return
		}
		_ = a.enqueueVerificationEmail(user)
		_ = a.store.AddAudit(user.ID, "user.verification_resend", "", now)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "If the address needs verification, a new code was sent",
	})
}

type apiLoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	Client   string `json:"client"`
}

func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) {
	var request apiLoginRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	login := strings.TrimSpace(request.Login)
	if login == "" || request.Password == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Login and password are required")
		return
	}
	ip := clientIP(r, a.cfg.TrustProxyHeaders)
	userAgent := truncate(r.UserAgent(), 512)
	normalized := normalizeUsername(login)
	if strings.Contains(login, "@") {
		normalized = normalizeEmail(login)
	}
	now := time.Now()
	if !a.allowLimited(a.loginLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many sign-in attempts")
		return
	}
	failures, err := a.store.RecentLoginFailures(normalized, ip, now.Unix()-loginFailureWindow)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not check sign-in attempts")
		return
	}
	if failures >= loginFailureMaximum {
		_ = a.store.RecordLoginAttempt(0, normalized, ip, userAgent, false, "locked", now.Unix())
		w.Header().Set("Retry-After", "900")
		apiV1Error(w, http.StatusTooManyRequests, "account_locked", "Too many failed sign-in attempts")
		return
	}

	var user *User
	if strings.Contains(login, "@") {
		user, err = a.store.UserByEmail(login)
	} else {
		user, err = a.store.UserByUsername(login)
	}
	if errors.Is(err, sql.ErrNoRows) {
		verifyPassword(a.dummyPasswordHash, request.Password)
		_ = a.store.RecordLoginAttempt(0, normalized, ip, userAgent, false, "unknown_account", now.Unix())
		apiV1Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid login or password")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not sign in")
		return
	}
	if !verifyPassword(user.PasswordHash, request.Password) {
		_ = a.store.RecordLoginAttempt(user.ID, normalized, ip, userAgent, false, "bad_password", now.Unix())
		apiV1Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid login or password")
		return
	}
	if user.Disabled || user.Status == "disabled" {
		_ = a.store.RecordLoginAttempt(user.ID, normalized, ip, userAgent, false, "disabled", now.Unix())
		apiV1Error(w, http.StatusForbidden, "account_disabled", "This account is disabled")
		return
	}
	if user.Status == "locked" {
		_ = a.store.RecordLoginAttempt(user.ID, normalized, ip, userAgent, false, "locked", now.Unix())
		apiV1Error(w, http.StatusForbidden, "account_locked", "This account is locked")
		return
	}
	if !user.EmailVerified || user.Status == "pending" {
		_ = a.store.RecordLoginAttempt(user.ID, normalized, ip, userAgent, false, "email_unverified", now.Unix())
		apiV1Error(w, http.StatusForbidden, "email_not_verified", "Verify your email address before signing in")
		return
	}

	_ = a.store.RecordLoginAttempt(user.ID, normalized, ip, userAgent, true, "", now.Unix())
	client := normalizeClient(request.Client)
	if strings.TrimSpace(request.Client) == "" {
		client = normalizeClient(r.Header.Get("X-UMC-Client"))
	}
	response, err := a.issueAPISession(w, r, user, client)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not start a session")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type apiRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	Client       string `json:"client"`
}

func (a *App) apiRefresh(w http.ResponseWriter, r *http.Request) {
	var request apiRefreshRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	token := strings.TrimSpace(request.RefreshToken)
	fromCookie := false
	if token == "" {
		if cookie, err := r.Cookie(refreshCookie); err == nil && cookie.Value != "" {
			token = cookie.Value
			fromCookie = true
		}
	}
	if token == "" {
		apiV1Error(w, http.StatusUnauthorized, "invalid_grant", "A refresh token is required")
		return
	}
	if !a.allowLimited(a.refreshLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many refresh attempts")
		return
	}
	client := normalizeClient(request.Client)
	if fromCookie {
		client = "web"
	}
	now := time.Now().Unix()
	accessToken := randomToken()
	refreshToken := randomToken()
	user, session, ok, reuse, err := a.store.RotateAuthSession(
		hashToken(token),
		AuthSessionInput{
			PublicID:         newUserUUID(),
			AccessHash:       hashToken(accessToken),
			RefreshHash:      hashToken(refreshToken),
			Client:           client,
			IP:               clientIP(r, a.cfg.TrustProxyHeaders),
			UserAgent:        truncate(r.UserAgent(), 512),
			AccessExpiresAt:  now + int64(a.cfg.AccessTokenMinutes)*60,
			RefreshExpiresAt: now + int64(a.cfg.RefreshTokenDays)*86_400,
		},
		now,
	)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not refresh the session")
		return
	}
	if reuse {
		a.clearRefreshCookie(w)
		if user != nil {
			_ = a.store.AddAudit(user.ID, "session.refresh_reuse", "", now)
		}
		apiV1Error(w, http.StatusUnauthorized, "invalid_grant", "The refresh token was already used")
		return
	}
	if !ok || user == nil || session == nil {
		a.clearRefreshCookie(w)
		apiV1Error(w, http.StatusUnauthorized, "invalid_grant", "The refresh token is invalid or expired")
		return
	}
	if user.Disabled || !user.EmailVerified ||
		user.Status == "disabled" || user.Status == "locked" || user.Status == "pending" {
		_, _ = a.store.RevokeAuthSessionByPublicID(user.ID, session.PublicID, now)
		a.clearRefreshCookie(w)
		apiV1Error(w, http.StatusForbidden, "account_unavailable", "The account is not available")
		return
	}
	web := client == "web"
	if web {
		a.setRefreshCookie(w, refreshToken, a.cfg.RefreshTokenDays*86_400)
	}
	response := map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int64(a.cfg.AccessTokenMinutes) * 60,
		"user":         apiUserJSON(user),
		"session":      apiSessionJSON(*session, true),
	}
	if !web {
		response["refresh_token"] = refreshToken
		response["refresh_expires_in"] = int64(a.cfg.RefreshTokenDays) * 86_400
	}
	writeJSON(w, http.StatusOK, response)
}

type apiLogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) {
	var request apiLogoutRequest
	if r.ContentLength > 0 {
		if !decodeV1JSON(w, r, &request) {
			return
		}
	}
	now := time.Now().Unix()
	token := strings.TrimSpace(request.RefreshToken)
	if token == "" {
		if cookie, err := r.Cookie(refreshCookie); err == nil {
			token = cookie.Value
		}
	}
	if token != "" {
		_ = a.store.RevokeAuthSessionByRefreshHash(hashToken(token), now)
	} else if access := bearerToken(r); access != "" {
		_ = a.store.RevokeAuthSessionByAccessHash(hashToken(access), now)
	}
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) apiMe(w http.ResponseWriter, r *http.Request) {
	user, session, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":    apiUserJSON(user),
		"session": apiSessionJSON(*session, true),
	})
}

func (a *App) apiListSessions(w http.ResponseWriter, r *http.Request) {
	user, current, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	sessions, err := a.store.ListAuthSessions(user.ID, time.Now().Unix())
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load sessions")
		return
	}
	result := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, apiSessionJSON(session, session.PublicID == current.PublicID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": result})
}

func (a *App) apiRevokeSession(w http.ResponseWriter, r *http.Request) {
	user, current, ok := a.requireAPIUser(w, r)
	if !ok {
		return
	}
	publicID := strings.TrimSpace(r.PathValue("id"))
	if publicID == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "A session id is required")
		return
	}
	revoked, err := a.store.RevokeAuthSessionByPublicID(user.ID, publicID, time.Now().Unix())
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not revoke the session")
		return
	}
	if !revoked {
		apiV1Error(w, http.StatusNotFound, "not_found", "Session not found")
		return
	}
	if publicID == current.PublicID {
		a.clearRefreshCookie(w)
	}
	_ = a.store.AddAudit(user.ID, "session.revoke", publicID, time.Now().Unix())
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) apiProfile(w http.ResponseWriter, r *http.Request) {
	if !a.allowLimited(a.apiLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many profile lookups")
		return
	}
	user, err := a.store.UserByUUID(r.PathValue("uuid"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "Profile not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the profile")
		return
	}
	writeJSON(w, http.StatusOK, a.profileResponse(user))
}

func (a *App) apiProfileByName(w http.ResponseWriter, r *http.Request) {
	if !a.allowLimited(a.apiLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many profile lookups")
		return
	}
	user, err := a.store.UserByUsername(r.PathValue("name"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "Profile not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the profile")
		return
	}
	writeJSON(w, http.StatusOK, a.profileResponse(user))
}

func (a *App) apiForgotPassword(w http.ResponseWriter, r *http.Request) {
	var request apiEmailRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.forgotLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many reset requests")
		return
	}
	if a.mailer == nil {
		apiV1Error(w, http.StatusServiceUnavailable, "email_unavailable", "Email delivery is not configured")
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err == nil && address.Address != "" {
		if user, lookupErr := a.store.UserByEmail(address.Address); lookupErr == nil &&
			!user.Disabled && user.EmailVerified {
			_ = a.enqueuePasswordResetEmail(user)
			_ = a.store.AddAudit(user.ID, "user.password_forgot", "", time.Now().Unix())
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "If the address exists, a password reset code was sent",
	})
}

type apiResetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (a *App) apiResetPassword(w http.ResponseWriter, r *http.Request) {
	var request apiResetPasswordRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	if !a.allowLimited(a.forgotLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiV1Error(w, http.StatusTooManyRequests, "too_many_requests", "Too many reset attempts")
		return
	}
	if err := validatePassword(request.NewPassword); err != nil {
		apiV1Error(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err != nil || address.Address == "" {
		apiV1Error(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address")
		return
	}
	userID, ok, err := a.store.VerifyPasswordResetCode(
		normalizeEmail(address.Address), hashToken(strings.TrimSpace(request.Code)), time.Now().Unix(),
	)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not reset the password")
		return
	}
	if !ok {
		apiV1Error(w, http.StatusBadRequest, "invalid_code", "The code is invalid or expired")
		return
	}
	passwordHash, err := hashPassword(request.NewPassword)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the password")
		return
	}
	if err := a.store.UpdateUserPassword(userID, passwordHash, time.Now().Unix()); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the password")
		return
	}
	_ = a.store.AddAudit(userID, "user.password_reset", "", time.Now().Unix())
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) apiAdminUsers(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIStaff(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 50, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 1_000_000)
	users, total, err := a.store.SearchUsers(r.URL.Query().Get("query"), limit, offset)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load users")
		return
	}
	result := make([]map[string]any, 0, len(users))
	for index := range users {
		result = append(result, apiUserJSON(&users[index]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":  result,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (a *App) apiAdminUser(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIStaff(w, r); !ok {
		return
	}
	user, err := a.store.UserByUUID(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": apiUserJSON(user)})
}

type apiAdminStatusRequest struct {
	Disabled bool `json:"disabled"`
}

func (a *App) apiAdminUserStatus(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	var request apiAdminStatusRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	target, err := a.store.UserByUUID(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the user")
		return
	}
	if target.ID == actor.ID {
		apiV1Error(w, http.StatusForbidden, "cannot_modify_self", "You cannot change your own status")
		return
	}
	if target.Role == "superadmin" && actor.Role != "superadmin" {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can modify this account")
		return
	}
	if (target.IsAdmin || isAdminRole(target.Role)) && actor.Role != "superadmin" {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can modify another administrator")
		return
	}
	now := time.Now().Unix()
	if err := a.store.SetUserDisabled(target.ID, request.Disabled, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the user")
		return
	}
	action := "user.enable"
	if request.Disabled {
		action = "user.disable"
	}
	_ = a.store.AddAudit(actor.ID, action, target.Username, now)
	updated, err := a.store.UserByID(target.ID)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not reload the user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": apiUserJSON(updated)})
}

type apiAdminRoleRequest struct {
	Role string `json:"role"`
}

func (a *App) apiAdminUserRole(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	var request apiAdminRoleRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	role := strings.ToLower(strings.TrimSpace(request.Role))
	if !validRole(role) {
		apiV1Error(w, http.StatusBadRequest, "invalid_role", "Role must be user, moderator, admin, or superadmin")
		return
	}
	target, err := a.store.UserByUUID(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the user")
		return
	}
	if target.ID == actor.ID {
		apiV1Error(w, http.StatusForbidden, "cannot_modify_self", "You cannot change your own role")
		return
	}
	if (target.Role == "superadmin" || role == "superadmin") && actor.Role != "superadmin" {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can manage superadmin roles")
		return
	}
	if (target.IsAdmin || isAdminRole(target.Role)) && actor.Role != "superadmin" {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can modify another administrator")
		return
	}
	if isAdminRole(role) && actor.Role != "superadmin" {
		apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can grant administrator roles")
		return
	}
	now := time.Now().Unix()
	if err := a.store.SetUserRole(target.ID, role, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the role")
		return
	}
	_ = a.store.AddAudit(actor.ID, "user.role_change",
		target.Username+": "+target.Role+" -> "+role, now)
	updated, err := a.store.UserByID(target.ID)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not reload the user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": apiUserJSON(updated)})
}

func (a *App) apiAdminVerifyEmail(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	target, err := a.store.UserByUUID(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the user")
		return
	}
	now := time.Now().Unix()
	if err := a.store.SetUserEmailVerified(target.ID, true, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not verify the email")
		return
	}
	_ = a.store.AddAudit(actor.ID, "user.verify_email_manual", target.Username, now)
	updated, err := a.store.UserByID(target.ID)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not reload the user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": apiUserJSON(updated)})
}

type apiAdminPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (a *App) apiAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	var request apiAdminPasswordRequest
	if r.ContentLength > 0 {
		if !decodeV1JSON(w, r, &request) {
			return
		}
	}
	target, err := a.store.UserByUUID(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the user")
		return
	}
	if target.ID != actor.ID {
		if target.Role == "superadmin" && actor.Role != "superadmin" {
			apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can reset this account")
			return
		}
		if (target.IsAdmin || isAdminRole(target.Role)) && actor.Role != "superadmin" {
			apiV1Error(w, http.StatusForbidden, "forbidden", "Only a superadmin can reset another administrator")
			return
		}
	}
	password := request.NewPassword
	generated := false
	if password == "" {
		password = newPassword(16)
		generated = true
	} else if err := validatePassword(password); err != nil {
		apiV1Error(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not hash the password")
		return
	}
	now := time.Now().Unix()
	if err := a.store.UpdateUserPassword(target.ID, passwordHash, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the password")
		return
	}
	_ = a.store.AddAudit(actor.ID, "user.password_reset_manual", target.Username, now)
	response := map[string]any{"user": apiUserJSON(target)}
	if generated {
		response["generated_password"] = password
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) apiAdminAudit(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIStaff(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100, 1, 500)
	entries, err := a.store.SearchAudit(r.URL.Query().Get("query"), limit)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the audit log")
		return
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		result = append(result, map[string]any{
			"id":         entry.ID,
			"actor":      entry.Actor,
			"action":     entry.Action,
			"detail":     entry.Detail,
			"created_at": entry.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": result})
}

func (a *App) apiAdminMail(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIAdmin(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100, 1, 500)
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	entries, err := a.store.ListMail(limit, status)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the mail log")
		return
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		var sentAt any
		if entry.SentAt.Valid {
			sentAt = entry.SentAt.Int64
		}
		result = append(result, map[string]any{
			"id":              entry.ID,
			"recipient":       entry.Recipient,
			"subject":         entry.Subject,
			"kind":            entry.Kind,
			"status":          entry.Status,
			"attempts":        entry.Attempts,
			"last_error":      entry.LastError,
			"next_attempt_at": entry.NextAttemptAt,
			"created_at":      entry.CreatedAt,
			"sent_at":         sentAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": result})
}

func (a *App) apiAdminRetryMail(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Invalid mail id")
		return
	}
	now := time.Now().Unix()
	retried, err := a.store.RetryMail(id, now)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not retry the message")
		return
	}
	if !retried {
		apiV1Error(w, http.StatusNotFound, "not_found", "Mail entry not found")
		return
	}
	_ = a.store.AddAudit(actor.ID, "mail.retry", strconv.FormatInt(id, 10), now)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) apiAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIAdmin(w, r); !ok {
		return
	}
	stats, err := a.store.AdminStats()
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load statistics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":          stats.Users,
		"active_users":   stats.ActiveUsers,
		"pending_users":  stats.PendingUsers,
		"disabled_users": stats.Disabled,
		"pending_mail":   stats.PendingMail,
	})
}

func queryInt(r *http.Request, name string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "UNIQUE") ||
		strings.Contains(message, "users_username_normalized") ||
		strings.Contains(message, "users_email_normalized") ||
		strings.Contains(message, "users_username_lower") ||
		strings.Contains(message, "users_email_lower")
}
