package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      string
}

type Mailer struct {
	cfg  SMTPConfig
	from *mail.Address
}

func NewMailer(cfg SMTPConfig) (*Mailer, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_FROM: %w", err)
	}
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid SMTP_HOST or SMTP_PORT")
	}
	switch cfg.TLS {
	case "starttls", "implicit", "none":
	default:
		return nil, fmt.Errorf("SMTP_TLS must be starttls, implicit, or none")
	}
	return &Mailer{cfg: cfg, from: from}, nil
}

func (m *Mailer) SendVerification(to, username, link string, validHours int) error {
	subject := "Verify your UMC Auth account"
	body := fmt.Sprintf(
		"Hello %s,\r\n\r\n"+
			"Verify your account by opening this link within %d hours:\r\n\r\n%s\r\n\r\n"+
			"If you did not request this account, ignore this message.\r\n",
		username, validHours, link,
	)
	return m.Send(to, subject, body)
}

func (m *Mailer) Send(to, subject, body string) error {
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	message := strings.Join([]string{
		"From: " + m.from.String(),
		"To: " + recipient.String(),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")

	address := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if m.cfg.TLS == "implicit" {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName: m.cfg.Host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(25 * time.Second))

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer client.Close()
	if m.cfg.TLS == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not offer STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: m.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if m.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(m.from.Address); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("close SMTP session: %w", err)
	}
	return nil
}
