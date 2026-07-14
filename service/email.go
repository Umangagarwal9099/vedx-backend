package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/umangagarwal/vedx-backend/config"
)

const resendAPIURL = "https://api.resend.com/emails"

// sendTimeout bounds how long a single email send may take.
const sendTimeout = 10 * time.Second

// EmailService sends transactional HTML email via Resend's HTTP API. Plain
// net/http, no SDK — same pattern as ZoomService. HTTP (443) is used
// deliberately instead of raw SMTP: outbound SMTP ports are blocked or
// silently dropped on Render (and many other PaaS hosts), which made Gmail
// SMTP unusable in production even though it worked fine locally.
type EmailService struct {
	cfg    config.ResendConfig
	client *http.Client
}

func NewEmailService(cfg config.ResendConfig) *EmailService {
	return &EmailService{
		cfg:    cfg,
		client: &http.Client{Timeout: sendTimeout},
	}
}

// Configured reports whether Resend credentials have been set.
func (e *EmailService) Configured() bool {
	return e.cfg.Configured()
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type resendErrorResponse struct {
	Message string `json:"message"`
}

// Send delivers an HTML email to a single recipient, bounded by sendTimeout.
// It blocks the calling goroutine for up to that long, so request handlers
// should use SendAsync instead — this is exported mainly for one-off/test
// callers that want the result directly.
func (e *EmailService) Send(to, subject, htmlBody string) error {
	if !e.Configured() {
		return fmt.Errorf("resend not configured")
	}

	from := e.cfg.FromEmail
	if e.cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", e.cfg.FromName, e.cfg.FromEmail)
	}

	body, err := json.Marshal(resendEmailRequest{
		From:    from,
		To:      []string{to},
		Subject: subject,
		HTML:    htmlBody,
	})
	if err != nil {
		return fmt.Errorf("encode resend request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, resendAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend request to %s: %w", to, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp resendErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("resend API error (%d) sending to %s: %s", resp.StatusCode, to, errResp.Message)
		}
		return fmt.Errorf("resend API error (%d) sending to %s: %s", resp.StatusCode, to, string(respBody))
	}
	return nil
}

// SendAsync fires off Send in the background and logs failures. Use this
// from HTTP request handlers so a slow email provider can never stall the
// API response — email delivery is best-effort and unrelated to whether the
// request itself succeeded.
func (e *EmailService) SendAsync(to, subject, htmlBody string) {
	go func() {
		if err := e.Send(to, subject, htmlBody); err != nil {
			log.Printf("send email to %s: %v", to, err)
		}
	}()
}
