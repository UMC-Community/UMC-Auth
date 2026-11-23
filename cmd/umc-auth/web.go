package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const (
	sessionCookie = "umc_auth_session"
	csrfCookie    = "umc_auth_csrf"
)

func (a *App) baseData(r *http.Request) BaseData {
	data := BaseData{ServerName: a.cfg.ServerName}
	if user, _, err := a.currentSession(r); err == nil && user != nil {
		data.LoggedIn = true
		data.Username = user.Username
		data.IsAdmin = user.IsAdmin
	}
	return data
}

func (a *App) landing(w http.ResponseWriter, r *http.Request) {
	a.templates.render(w, http.StatusOK, "landing", LandingData{
		BaseData:     a.baseData(r),
		YggdrasilURL: a.yggdrasilURL(),
	})
}

func (a *App) registerPage(w http.ResponseWriter, r *http.Request) {
	userCount, err := a.store.CountUsers()
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load registration settings")
		return
	}
	firstUser := userCount == 0
	if a.cfg.RegistrationMode == "closed" && !firstUser {
		a.webError(w, r, http.StatusForbidden, "Registration is closed")
		return
	}
	csrf := a.freshCSRF(w, r)
	captcha := "none"
	if !firstUser {
		captcha = a.cfg.CaptchaMode
	}
	a.templates.render(w, http.StatusOK, "register", AuthFormData{
		BaseData:         a.baseData(r),
		CSRF:             csrf,
		InviteInput:      r.URL.Query().Get("invite"),
		RequireBootstrap: firstUser && a.cfg.BootstrapToken != "",
		RequireInvite:    !firstUser && a.cfg.RegistrationMode == "invite",
		RequireEmail:     !firstUser && a.cfg.EmailVerification == "required",
		CaptchaMode:      captcha,
		TurnstileSiteKey: a.cfg.TurnstileSiteKey,
	})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	userCount, err := a.store.CountUsers()
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load registration settings")
		return
	}
	firstUser := userCount == 0
	if a.cfg.RegistrationMode == "closed" && !firstUser {
		a.webError(w, r, http.StatusForbidden, "Registration is closed")
		return
	}
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed registration form")
		return
	}
	csrf := r.PostFormValue("csrf")
	if !a.csrfMatches(r, csrf) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token; reload the form")
		return
	}
	if !a.registrationAllowed(r) {
		w.Header().Set("Retry-After", "3600")
		a.webError(w, r, http.StatusTooManyRequests, "Too many registration attempts; try again later")
		return
	}

	username := strings.TrimSpace(r.PostFormValue("username"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	invite := strings.ToUpper(strings.TrimSpace(r.PostFormValue("invite")))
	bootstrapToken := r.PostFormValue("bootstrap_token")
	password := r.PostFormValue("password")
	confirm := r.PostFormValue("confirm")
	requireInvite := !firstUser && a.cfg.RegistrationMode == "invite"
	requireEmail := !firstUser && a.cfg.EmailVerification == "required"

	renderError := func(message string) {
		captcha := "none"
		if !firstUser {
			captcha = a.cfg.CaptchaMode
		}
		a.templates.render(w, http.StatusOK, "register", AuthFormData{
			BaseData:         a.baseData(r),
			CSRF:             csrf,
			Error:            message,
			UsernameInput:    username,
			EmailInput:       email,
			InviteInput:      invite,
			BootstrapInput:   bootstrapToken,
			RequireBootstrap: firstUser && a.cfg.BootstrapToken != "",
			RequireInvite:    requireInvite,
			RequireEmail:     requireEmail,
			CaptchaMode:      captcha,
			TurnstileSiteKey: a.cfg.TurnstileSiteKey,
		})
	}

	if firstUser && a.cfg.BootstrapToken != "" &&
		!constantTimeEqual(a.cfg.BootstrapToken, bootstrapToken) {
		renderError("Invalid bootstrap token")
		return
	}
	if !firstUser {
		switch a.cfg.CaptchaMode {
		case "turnstile":
			if a.turnstile == nil {
				renderError("Turnstile is not configured correctly")
				return
			}
			if err := a.turnstile.Verify(
				r.Context(), r.PostFormValue("cf-turnstile-response"),
				clientIP(r, a.cfg.TrustProxyHeaders), "register",
			); err != nil {
				renderError(err.Error())
				return
			}
		case "pow":
			if a.poW == nil {
				renderError("Proof-of-work is not configured correctly")
				return
			}
			if err := a.poW.Verify(
				r.PostFormValue("pow_token"), r.PostFormValue("pow_nonce"),
				clientIP(r, a.cfg.TrustProxyHeaders), time.Now(),
			); err != nil {
				renderError(err.Error())
				return
			}
		}
	}

	now := time.Now()
	var inviteID int64
	inviteUsed := false
	releaseInvite := func() {
		if inviteUsed {
			a.store.ReleaseInvite(inviteID)
			inviteUsed = false
		}
	}
	if requireInvite {
		if invite == "" {
			renderError("Invite code is required")
			return
		}
		id, ok, err := a.store.ConsumeInvite(hashToken(invite), now.Unix())
		if err != nil {
			renderError("Could not validate the invite code")
			return
		}
		if !ok {
			renderError("Invite code is invalid, expired, or already used")
			return
		}
		inviteID = id
		inviteUsed = true
	}
	if requireEmail {
		address, err := mail.ParseAddress(email)
		if err != nil {
			releaseInvite()
			renderError("Enter a valid email address")
			return
		}
		email = address.Address
	} else {
		email = ""
	}

	if err := validateUsername(username); err != nil {
		releaseInvite()
		renderError(err.Error())
		return
	}
	if err := validatePassword(password); err != nil {
		releaseInvite()
		renderError(err.Error())
		return
	}
	if password != confirm {
		releaseInvite()
		renderError("Passwords do not match")
		return
	}
	if _, err := a.store.UserByUsername(username); err == nil {
		releaseInvite()
		renderError("That username is already registered")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		releaseInvite()
		a.webError(w, r, http.StatusInternalServerError, "Could not check the account")
		return
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		releaseInvite()
		a.webError(w, r, http.StatusInternalServerError, "Could not hash the password")
		return
	}
	uuid := newUserUUID()
	userID, err := a.store.CreateUser(
		uuid, username, passwordHash, email, !requireEmail, firstUser, now.Unix(),
	)
	if err != nil {
		releaseInvite()
		if strings.Contains(err.Error(), "users_email_lower") {
			renderError("That email address is already registered")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			renderError("That username is already registered")
			return
		}
		a.webError(w, r, http.StatusInternalServerError, "Could not create the account")
		return
	}
	if firstUser {
		if err := a.store.SetUserRole(userID, "superadmin", now.Unix()); err != nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			a.webError(w, r, http.StatusInternalServerError, "Could not initialize the administrator")
			return
		}
	}

	if requireEmail {
		token := randomToken()
		if err := a.store.CreateEmailVerification(
			userID, hashToken(token), now.Unix(), now.Add(24*time.Hour).Unix(),
		); err != nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			a.webError(w, r, http.StatusInternalServerError, "Could not create the verification email")
			return
		}
		link := a.cfg.PublicOrigin + "/verify-email?token=" + url.QueryEscape(token)
		if a.mailer == nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			a.webError(w, r, http.StatusInternalServerError, "Email delivery is not configured")
			return
		}
		if err := a.mailer.SendVerification(email, username, link, 24); err != nil {
			_ = a.store.DeleteUser(userID)
			releaseInvite()
			a.webError(w, r, http.StatusBadGateway, "Could not send the verification email")
			return
		}
		_ = a.store.AddAudit(userID, "user.register", username, now.Unix())
		a.templates.render(w, http.StatusOK, "message", MessageData{
			BaseData: BaseData{ServerName: a.cfg.ServerName},
			Title:    "Check your email",
			Message:  "We sent a verification link to " + email + ". It expires in 24 hours.",
			Link:     "/login",
			LinkText: "Continue to sign in",
		})
		return
	}

	if err := a.issueSession(w, userID); err != nil {
		releaseInvite()
		a.webError(w, r, http.StatusInternalServerError, "Could not start the session")
		return
	}
	_ = a.store.AddAudit(userID, "user.register", username, now.Unix())
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	csrf := a.freshCSRF(w, r)
	a.templates.render(w, http.StatusOK, "login", AuthFormData{
		BaseData: a.baseData(r),
		CSRF:     csrf,
	})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed sign-in form")
		return
	}
	csrf := r.PostFormValue("csrf")
	if !a.csrfMatches(r, csrf) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token; reload the form")
		return
	}
	if !a.loginAllowed(r) {
		w.Header().Set("Retry-After", "60")
		a.webError(w, r, http.StatusTooManyRequests, "Too many sign-in attempts; try again later")
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")
	renderError := func(message string) {
		a.templates.render(w, http.StatusOK, "login", AuthFormData{
			BaseData:      a.baseData(r),
			CSRF:          csrf,
			Error:         message,
			UsernameInput: username,
		})
	}

	user, err := a.store.UserByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		renderError("Invalid username or password")
		return
	}
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not sign in")
		return
	}
	if !verifyPassword(user.PasswordHash, password) {
		renderError("Invalid username or password")
		return
	}
	if user.Disabled {
		renderError("This account is disabled")
		return
	}
	if !user.EmailVerified {
		renderError("Verify your email address before signing in")
		return
	}
	if err := a.issueSession(w, user.ID); err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not start the session")
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.webError(w, r, http.StatusBadRequest, "Malformed sign-out form")
		return
	}
	user, sessionCSRF, err := a.currentSession(r)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load the session")
		return
	}
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !constantTimeEqual(sessionCSRF, r.PostFormValue("csrf")) {
		a.webError(w, r, http.StatusForbidden, "Invalid CSRF token")
		return
	}
	if token, ok := sessionToken(r); ok {
		if err := a.store.DeleteSession(hashToken(token)); err != nil {
			a.webError(w, r, http.StatusInternalServerError, "Could not sign out")
			return
		}
	}
	a.clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	user, csrf, err := a.currentSession(r)
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not load the session")
		return
	}
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	a.templates.render(w, http.StatusOK, "dashboard", DashboardData{
		BaseData: BaseData{
			ServerName: a.cfg.ServerName,
			Username:   user.Username,
			LoggedIn:   true,
			IsAdmin:    user.IsAdmin,
		},
		UUID:         user.UUID,
		PrettyUUID:   prettyUUID(user.UUID),
		CreatedAt:    time.Unix(user.CreatedAt, 0).UTC().Format("2006-01-02"),
		YggdrasilURL: a.yggdrasilURL(),
		CSRF:         csrf,
		IsAdmin:      user.IsAdmin,
	})
}

func (a *App) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		a.templates.render(w, http.StatusBadRequest, "message", MessageData{
			BaseData: BaseData{ServerName: a.cfg.ServerName},
			Title:    "Invalid verification link",
			Message:  "The verification token is missing.",
			Link:     "/login",
			LinkText: "Return to sign in",
		})
		return
	}
	userID, ok, err := a.store.ConsumeEmailVerification(hashToken(token), time.Now().Unix())
	if err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not verify the email address")
		return
	}
	if !ok {
		a.templates.render(w, http.StatusBadRequest, "message", MessageData{
			BaseData: BaseData{ServerName: a.cfg.ServerName},
			Title:    "Verification link expired",
			Message:  "This link is invalid, expired, or was already used.",
			Link:     "/login",
			LinkText: "Return to sign in",
		})
		return
	}
	if err := a.issueSession(w, userID); err != nil {
		a.webError(w, r, http.StatusInternalServerError, "Could not start the session")
		return
	}
	_ = a.store.AddAudit(userID, "user.verify_email", "", time.Now().Unix())
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (a *App) registrationChallenge(w http.ResponseWriter, r *http.Request) {
	if a.cfg.CaptchaMode != "pow" || a.poW == nil {
		apiError(w, http.StatusNotFound, "NotFound", "Proof-of-work is not enabled")
		return
	}
	if !a.allowLimited(a.powLimiter, r) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "TooManyRequests", "Too many challenge requests")
		return
	}
	token, challenge, difficulty, expiresAt, err := a.poW.Issue(
		clientIP(r, a.cfg.TrustProxyHeaders), time.Now(),
	)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "InternalServerError", "Could not issue a challenge")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"challenge":  challenge,
		"difficulty": difficulty,
		"expires_at": expiresAt,
	})
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (a *App) currentSession(r *http.Request) (*User, string, error) {
	token, ok := sessionToken(r)
	if !ok {
		return nil, "", nil
	}
	user, csrf, err := a.store.SessionUser(hashToken(token), time.Now().Unix())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return user, csrf, nil
}

func (a *App) issueSession(w http.ResponseWriter, userID int64) error {
	token := randomToken()
	csrf := randomToken()
	now := time.Now().Unix()
	expiresAt := now + int64(a.cfg.SessionDays)*86_400
	if err := a.store.CreateSession(userID, hashToken(token), csrf, now, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   a.cfg.SessionDays * 86_400,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) freshCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   1800,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func (a *App) csrfMatches(r *http.Request, submitted string) bool {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" || submitted == "" {
		return false
	}
	return constantTimeEqual(cookie.Value, submitted)
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (a *App) webError(w http.ResponseWriter, r *http.Request, status int, message string) {
	a.templates.errorPage(w, status, a.cfg.ServerName, message)
}

func (a *App) registrationAllowed(r *http.Request) bool {
	key := rateKey(clientIP(r, a.cfg.TrustProxyHeaders))
	now := time.Now()
	if !a.registerHourly.Check(key, now) ||
		!a.registerDaily.Check(key, now) ||
		!a.registerGlobal.Check("global", now) {
		return false
	}
	a.registerHourly.Record(key, now)
	a.registerDaily.Record(key, now)
	a.registerGlobal.Record("global", now)
	return true
}

func (a *App) loginAllowed(r *http.Request) bool {
	key := rateKey(clientIP(r, a.cfg.TrustProxyHeaders))
	now := time.Now()
	if !a.loginLimiter.Check(key, now) {
		return false
	}
	a.loginLimiter.Record(key, now)
	return true
}
