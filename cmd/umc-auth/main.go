// Command umc-auth is an optional self-hosted account and Yggdrasil
// authentication service for Minecraft communities.
package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Bind                   string
	PublicOrigin           string
	OriginHost             string
	ServerName             string
	UploadDir              string
	BootstrapToken         string
	RegistrationMode       string
	EmailVerification      string
	CaptchaMode            string
	TurnstileSiteKey       string
	TurnstileSecretKey     string
	SMTP                   SMTPConfig
	SecureCookies          bool
	SessionDays            int
	TokenDays              int
	TrustProxyHeaders      bool
	AllowInsecureHTTP      bool
	PowDifficulty          int
	RegisterPerHour        int
	RegisterPerDay         int
	RegisterGlobalPerHour  int
	LoginPerMinute         int
	PowChallengePerMinute  int
	ProfileLookupPerMinute int
	AccessTokenMinutes     int
	RefreshTokenDays       int
	ForgotLimitPerMinute   int
	VerifyLimitPerMinute   int
	RefreshLimitPerMinute  int
}

type App struct {
	store          *Store
	cfg            Config
	pubKeyPEM      string
	signingKey     *rsa.PrivateKey
	powSecret      []byte
	poW            *PoW
	mailer         *Mailer
	turnstile      *Turnstile
	templates      *Templates
	registerHourly *slidingWindow
	registerDaily  *slidingWindow
	registerGlobal *slidingWindow
	loginLimiter   *slidingWindow
	powLimiter     *slidingWindow
	apiLimiter     *slidingWindow
	forgotLimiter  *slidingWindow
	verifyLimiter  *slidingWindow
	refreshLimiter *slidingWindow

	dummyPasswordHash string
}

func (a *App) yggdrasilURL() string {
	return a.cfg.PublicOrigin + "/api/yggdrasil"
}

func loadConfig() (Config, error) {
	publicOrigin := strings.TrimRight(env("PUBLIC_ORIGIN", "http://localhost:8080"), "/")
	parsed, err := url.Parse(publicOrigin)
	if err != nil {
		return Config{}, errors.New("invalid PUBLIC_ORIGIN: " + err.Error())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Config{}, errors.New("PUBLIC_ORIGIN must use http or https")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, errors.New("PUBLIC_ORIGIN must not include a path, query, or fragment")
	}
	host := parsed.Hostname()
	if host == "" {
		return Config{}, errors.New("PUBLIC_ORIGIN must include a host")
	}
	sessionDays, err := envInt("SESSION_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	tokenDays, err := envInt("TOKEN_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	if sessionDays < 1 || sessionDays > 3650 || tokenDays < 1 || tokenDays > 3650 {
		return Config{}, errors.New("SESSION_DAYS and TOKEN_DAYS must be between 1 and 3650")
	}
	secureDefault := parsed.Scheme == "https"
	secureCookies, err := envBool("COOKIE_SECURE", secureDefault)
	if err != nil {
		return Config{}, err
	}
	trustProxy, err := envBool("TRUST_PROXY_HEADERS", false)
	if err != nil {
		return Config{}, err
	}
	allowInsecure, err := envBool("ALLOW_INSECURE_HTTP", false)
	if err != nil {
		return Config{}, err
	}
	if parsed.Scheme != "https" && !allowInsecure {
		return Config{}, errors.New("PUBLIC_ORIGIN must use https; set ALLOW_INSECURE_HTTP=true only for local development")
	}
	registrationMode := strings.ToLower(env("REGISTRATION_MODE", "invite"))
	switch registrationMode {
	case "invite", "open", "closed":
	default:
		return Config{}, errors.New("REGISTRATION_MODE must be invite, open, or closed")
	}
	emailVerification := strings.ToLower(env("EMAIL_VERIFICATION", "off"))
	switch emailVerification {
	case "off", "required":
	default:
		return Config{}, errors.New("EMAIL_VERIFICATION must be off or required")
	}
	smtpPort, err := envInt("SMTP_PORT", 587)
	if err != nil {
		return Config{}, err
	}
	smtp := SMTPConfig{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     smtpPort,
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
		TLS:      strings.ToLower(env("SMTP_TLS", "starttls")),
	}
	if emailVerification == "required" {
		if smtp.Host == "" || smtp.From == "" {
			return Config{}, errors.New("EMAIL_VERIFICATION=required needs SMTP_HOST and SMTP_FROM")
		}
	}
	switch smtp.TLS {
	case "starttls", "implicit", "none":
	default:
		return Config{}, errors.New("SMTP_TLS must be starttls, implicit, or none")
	}
	captchaMode := strings.ToLower(env("CAPTCHA_MODE", "none"))
	switch captchaMode {
	case "none", "pow", "turnstile":
	default:
		return Config{}, errors.New("CAPTCHA_MODE must be none, pow, or turnstile")
	}
	turnstileSiteKey := os.Getenv("TURNSTILE_SITE_KEY")
	turnstileSecretKey := os.Getenv("TURNSTILE_SECRET_KEY")
	if captchaMode == "turnstile" && (turnstileSiteKey == "" || turnstileSecretKey == "") {
		return Config{}, errors.New("CAPTCHA_MODE=turnstile needs TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY")
	}
	powDifficulty, err := envInt("POW_DIFFICULTY", 16)
	if err != nil {
		return Config{}, err
	}
	if powDifficulty < 12 || powDifficulty > 24 {
		return Config{}, errors.New("POW_DIFFICULTY must be between 12 and 24")
	}
	registerPerHour, err := envInt("REGISTER_LIMIT_PER_HOUR", 3)
	if err != nil {
		return Config{}, err
	}
	registerPerDay, err := envInt("REGISTER_LIMIT_PER_DAY", 10)
	if err != nil {
		return Config{}, err
	}
	registerGlobal, err := envInt("REGISTER_LIMIT_GLOBAL_PER_HOUR", 100)
	if err != nil {
		return Config{}, err
	}
	loginPerMinute, err := envInt("LOGIN_LIMIT_PER_MINUTE", 10)
	if err != nil {
		return Config{}, err
	}
	powChallengePerMinute, err := envInt("POW_CHALLENGE_LIMIT_PER_MINUTE", 30)
	if err != nil {
		return Config{}, err
	}
	profileLookupPerMinute, err := envInt("PROFILE_LOOKUP_LIMIT_PER_MINUTE", 60)
	if err != nil {
		return Config{}, err
	}
	accessTokenMinutes, err := envInt("ACCESS_TOKEN_MINUTES", 15)
	if err != nil {
		return Config{}, err
	}
	refreshTokenDays, err := envInt("REFRESH_TOKEN_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	forgotLimitPerMinute, err := envInt("FORGOT_LIMIT_PER_MINUTE", 5)
	if err != nil {
		return Config{}, err
	}
	verifyLimitPerMinute, err := envInt("VERIFY_LIMIT_PER_MINUTE", 10)
	if err != nil {
		return Config{}, err
	}
	refreshLimitPerMinute, err := envInt("REFRESH_LIMIT_PER_MINUTE", 60)
	if err != nil {
		return Config{}, err
	}
	if registerPerHour < 1 || registerPerDay < 1 || registerGlobal < 1 ||
		loginPerMinute < 1 || powChallengePerMinute < 1 || profileLookupPerMinute < 1 ||
		forgotLimitPerMinute < 1 || verifyLimitPerMinute < 1 || refreshLimitPerMinute < 1 {
		return Config{}, errors.New("rate limits must be positive")
	}
	if accessTokenMinutes < 1 || accessTokenMinutes > 1440 {
		return Config{}, errors.New("ACCESS_TOKEN_MINUTES must be between 1 and 1440")
	}
	if refreshTokenDays < 1 || refreshTokenDays > 3650 {
		return Config{}, errors.New("REFRESH_TOKEN_DAYS must be between 1 and 3650")
	}
	bind := env("BIND_ADDR", "0.0.0.0:8080")
	return Config{
		Bind:                   bind,
		PublicOrigin:           publicOrigin,
		OriginHost:             host,
		ServerName:             env("SERVER_NAME", "UMC Auth"),
		UploadDir:              env("UPLOAD_DIR", "data/uploads"),
		BootstrapToken:         os.Getenv("BOOTSTRAP_TOKEN"),
		RegistrationMode:       registrationMode,
		EmailVerification:      emailVerification,
		CaptchaMode:            captchaMode,
		TurnstileSiteKey:       turnstileSiteKey,
		TurnstileSecretKey:     turnstileSecretKey,
		SMTP:                   smtp,
		SecureCookies:          secureCookies,
		SessionDays:            sessionDays,
		TokenDays:              tokenDays,
		TrustProxyHeaders:      trustProxy,
		AllowInsecureHTTP:      allowInsecure,
		PowDifficulty:          powDifficulty,
		RegisterPerHour:        registerPerHour,
		RegisterPerDay:         registerPerDay,
		RegisterGlobalPerHour:  registerGlobal,
		LoginPerMinute:         loginPerMinute,
		PowChallengePerMinute:  powChallengePerMinute,
		ProfileLookupPerMinute: profileLookupPerMinute,
		AccessTokenMinutes:     accessTokenMinutes,
		RefreshTokenDays:       refreshTokenDays,
		ForgotLimitPerMinute:   forgotLimitPerMinute,
		VerifyLimitPerMinute:   verifyLimitPerMinute,
		RefreshLimitPerMinute:  refreshLimitPerMinute,
	}, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New(name + " must be true or false")
	}
}

func envInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return parsed, nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.apiRoot)
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/register/challenge", a.registrationChallenge)

	mux.HandleFunc("GET /api/yggdrasil", a.metadata)
	mux.HandleFunc("GET /api/yggdrasil/{$}", a.metadata)
	mux.HandleFunc("POST /api/yggdrasil/authserver/authenticate", a.authenticate)
	mux.HandleFunc("POST /api/yggdrasil/authserver/refresh", a.refresh)
	mux.HandleFunc("POST /api/yggdrasil/authserver/validate", a.validate)
	mux.HandleFunc("POST /api/yggdrasil/authserver/invalidate", a.invalidate)
	mux.HandleFunc("POST /api/yggdrasil/authserver/signout", a.signout)
	mux.HandleFunc("POST /api/yggdrasil/sessionserver/session/minecraft/join", a.join)
	mux.HandleFunc("GET /api/yggdrasil/sessionserver/session/minecraft/hasJoined", a.hasJoined)
	mux.HandleFunc("GET /api/yggdrasil/sessionserver/session/minecraft/profile/{uuid}", a.profile)
	mux.HandleFunc("GET /api/yggdrasil/api/user/profile/{uuid}", a.profile)
	mux.HandleFunc("POST /api/yggdrasil/api/profiles/minecraft", a.profilesByName)

	mux.HandleFunc("POST /api/v1/auth/register", a.apiRegister)
	mux.HandleFunc("GET /api/v1/meta", a.apiMeta)
	mux.HandleFunc("POST /api/v1/auth/verify-email", a.apiVerifyEmail)
	mux.HandleFunc("POST /api/v1/auth/verify-email/resend", a.apiResendVerification)
	mux.HandleFunc("POST /api/v1/auth/login", a.apiLogin)
	mux.HandleFunc("POST /api/v1/auth/refresh", a.apiRefresh)
	mux.HandleFunc("POST /api/v1/auth/logout", a.apiLogout)
	mux.HandleFunc("POST /api/v1/auth/password/forgot", a.apiForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/password/reset", a.apiResetPassword)
	mux.HandleFunc("GET /api/v1/users/me", a.apiMe)
	mux.HandleFunc("GET /api/v1/users/me/minecraft", a.apiMinecraftProfile)
	mux.HandleFunc("POST /api/v1/users/me/skin", a.apiUploadSkin)
	mux.HandleFunc("DELETE /api/v1/users/me/skin", a.apiDeleteSkin)
	mux.HandleFunc("POST /api/v1/users/me/cape", a.apiUploadCape)
	mux.HandleFunc("DELETE /api/v1/users/me/cape", a.apiDeleteCape)
	mux.HandleFunc("GET /api/v1/users/me/sessions", a.apiListSessions)
	mux.HandleFunc("DELETE /api/v1/users/me/sessions/{id}", a.apiRevokeSession)
	mux.HandleFunc("GET /api/v1/profiles/by-username/{name}", a.apiProfileByName)
	mux.HandleFunc("GET /api/v1/profiles/{uuid}", a.apiProfile)

	mux.HandleFunc("GET /api/v1/admin/users", a.apiAdminUsers)
	mux.HandleFunc("GET /api/v1/admin/users/{id}", a.apiAdminUser)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/status", a.apiAdminUserStatus)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/role", a.apiAdminUserRole)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/verify-email", a.apiAdminVerifyEmail)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/reset-password", a.apiAdminResetPassword)
	mux.HandleFunc("GET /api/v1/admin/audit", a.apiAdminAudit)
	mux.HandleFunc("GET /api/v1/admin/mail", a.apiAdminMail)
	mux.HandleFunc("POST /api/v1/admin/mail/{id}/retry", a.apiAdminRetryMail)
	mux.HandleFunc("GET /api/v1/admin/stats", a.apiAdminStats)
	mux.HandleFunc("GET /api/v1/admin/invites", a.apiAdminInvites)
	mux.HandleFunc("POST /api/v1/admin/invites", a.apiAdminCreateInvites)
	mux.HandleFunc("POST /api/v1/admin/invites/{id}/revoke", a.apiAdminRevokeInvite)

	mux.HandleFunc("GET /api/v1/admin/servers", a.apiAdminServers)
	mux.HandleFunc("POST /api/v1/admin/servers", a.apiAdminCreateServer)
	mux.HandleFunc("POST /api/v1/admin/servers/{id}/status", a.apiAdminServerStatus)
	mux.HandleFunc("DELETE /api/v1/admin/servers/{id}", a.apiAdminDeleteServer)
	mux.HandleFunc("GET /api/v1/admin/servers/joins", a.apiAdminServerJoins)

	mux.HandleFunc("GET /textures/{kind}/{uuid}/{hash}", a.texture)

	// This host is JSON-only. Any unmatched page or API path gets a JSON 404
	// instead of an HTML response.
	mux.HandleFunc("/", a.apiNotFound)

	return limitBody(a.securityHeaders(logRequests(mux)))
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// JSON requests are small; the larger limit accommodates 1 MB PNG
		// skin and cape uploads plus multipart overhead.
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next.ServeHTTP(w, r)
	})
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	csp := "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
		"form-action 'self'; frame-ancestors 'none'; base-uri 'self'"
	if a.cfg.CaptchaMode == "turnstile" {
		csp += "; script-src 'self' https://challenges.cloudflare.com" +
			"; connect-src 'self' https://challenges.cloudflare.com" +
			"; frame-src https://challenges.cloudflare.com"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			for index := len(parts) - 1; index >= 0; index-- {
				if value := strings.TrimSpace(parts[index]); value != "" {
					return value
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	level := slog.LevelInfo
	switch strings.ToLower(env("LOG_LEVEL", "info")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	databaseURL := env("DATABASE_URL", "postgres://umc_auth:umc_auth@localhost:5432/umc_auth?sslmode=disable")
	store, err := OpenStore(databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	signingKey, pubKeyPEM, err := store.LoadOrCreateSigningKeyPair()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.UploadDir, "skins"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.UploadDir, "capes"), 0o755); err != nil {
		return err
	}
	powSecret, err := store.LoadOrCreateSecret("pow_secret", func() (string, error) {
		return randomToken(), nil
	})
	if err != nil {
		return err
	}
	var mailer *Mailer
	if cfg.SMTP.Host != "" {
		mailer, err = NewMailer(cfg.SMTP)
		if err != nil {
			return err
		}
	}
	if cfg.EmailVerification == "required" && mailer == nil {
		return errors.New("EMAIL_VERIFICATION=required needs a configured SMTP server")
	}
	var turnstile *Turnstile
	if cfg.CaptchaMode == "turnstile" {
		turnstile = NewTurnstile(cfg.TurnstileSecretKey, cfg.OriginHost)
	}
	templates, err := LoadTemplates()
	if err != nil {
		return err
	}
	dummyPasswordHash, err := hashPassword(randomToken())
	if err != nil {
		return err
	}
	app := &App{
		store: store, cfg: cfg, pubKeyPEM: pubKeyPEM, signingKey: signingKey,
		powSecret: []byte(powSecret), poW: NewPoW(store, []byte(powSecret), cfg.PowDifficulty),
		mailer: mailer, turnstile: turnstile, templates: templates,
		registerHourly: newSlidingWindow(cfg.RegisterPerHour, time.Hour),
		registerDaily:  newSlidingWindow(cfg.RegisterPerDay, 24*time.Hour),
		registerGlobal: newSlidingWindow(cfg.RegisterGlobalPerHour, time.Hour),
		loginLimiter:   newSlidingWindow(cfg.LoginPerMinute, time.Minute),
		powLimiter:     newSlidingWindow(cfg.PowChallengePerMinute, time.Minute),
		apiLimiter:     newSlidingWindow(cfg.ProfileLookupPerMinute, time.Minute),
		forgotLimiter:  newSlidingWindow(cfg.ForgotLimitPerMinute, time.Minute),
		verifyLimiter:  newSlidingWindow(cfg.VerifyLimitPerMinute, time.Minute),
		refreshLimiter: newSlidingWindow(cfg.RefreshLimitPerMinute, time.Minute),

		dummyPasswordHash: dummyPasswordHash,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if app.mailer != nil {
		go app.mailWorker(ctx)
	}

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.Cleanup(time.Now().Unix()); err != nil {
					slog.Warn("database cleanup failed", "error", err)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Bind,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("UMC Auth listening", "address", cfg.Bind, "public_origin", cfg.PublicOrigin)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
