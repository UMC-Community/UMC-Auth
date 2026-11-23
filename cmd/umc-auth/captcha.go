package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type Turnstile struct {
	secret string
	host   string
	client *http.Client
}

func NewTurnstile(secret, host string) *Turnstile {
	return &Turnstile{
		secret: secret,
		host:   host,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

type turnstileResponse struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	ErrorCodes []string `json:"error-codes"`
}

func (t *Turnstile) Verify(ctx context.Context, token, remoteIP, expectedAction string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("complete the Turnstile challenge")
	}
	form := url.Values{
		"secret":   {t.secret},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("create Turnstile request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := t.client.Do(request)
	if err != nil {
		return fmt.Errorf("Turnstile verification failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read Turnstile response: %w", err)
	}
	var result turnstileResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("invalid Turnstile response")
	}
	if !result.Success {
		return fmt.Errorf("Turnstile rejected the request: %s", strings.Join(result.ErrorCodes, ", "))
	}
	if result.Hostname != "" && !strings.EqualFold(result.Hostname, t.host) {
		return fmt.Errorf("Turnstile hostname mismatch")
	}
	if expectedAction != "" && result.Action != expectedAction {
		return fmt.Errorf("Turnstile action mismatch")
	}
	return nil
}
