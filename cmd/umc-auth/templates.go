package main

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

type BaseData struct {
	ServerName string
	Username   string
	LoggedIn   bool
	IsAdmin    bool
}

type LandingData struct {
	BaseData
	YggdrasilURL string
}

type AuthFormData struct {
	BaseData
	CSRF             string
	Error            string
	UsernameInput    string
	EmailInput       string
	InviteInput      string
	BootstrapInput   string
	RequireInvite    bool
	RequireEmail     bool
	RequireBootstrap bool
	CaptchaMode      string
	TurnstileSiteKey string
}

type DashboardData struct {
	BaseData
	UUID         string
	PrettyUUID   string
	CreatedAt    string
	YggdrasilURL string
	CSRF         string
	IsAdmin      bool
}

type ErrorData struct {
	BaseData
	Status  int
	Message string
}

type MessageData struct {
	BaseData
	Title    string
	Message  string
	Link     string
	LinkText string
}

type AdminData struct {
	BaseData
	CSRF              string
	RegistrationMode  string
	CaptchaMode       string
	EmailVerification string
	UserCount         int64
	Users             []User
	Invites           []Invite
	Audit             []AuditEntry
	NewCodes          []string
	Error             string
}

type Templates struct {
	pages map[string]*template.Template
}

func LoadTemplates() (*Templates, error) {
	pages := make(map[string]*template.Template)
	for _, name := range []string{
		"landing", "register", "login", "dashboard", "error", "message",
		"admin_dashboard", "admin_invites", "admin_users", "admin_audit",
	} {
		parsed, err := template.New("layout").Funcs(template.FuncMap{
			"unix": func(value int64) string {
				return time.Unix(value, 0).UTC().Format("2006-01-02 15:04")
			},
		}).ParseFS(
			templateFS,
			"templates/layout.html",
			fmt.Sprintf("templates/%s.html", name),
		)
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}
		pages[name] = parsed
	}
	return &Templates{pages: pages}, nil
}

func (t *Templates) render(w http.ResponseWriter, status int, name string, data any) {
	page, ok := t.pages[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := page.ExecuteTemplate(w, "layout", data); err != nil {
		// The status and part of the body may already be sent; log only.
		slog.Error("render template", "template", name, "error", err)
	}
}

func (t *Templates) errorPage(w http.ResponseWriter, status int, serverName, message string) {
	t.render(w, status, "error", ErrorData{
		BaseData: BaseData{ServerName: serverName},
		Status:   status,
		Message:  message,
	})
}
