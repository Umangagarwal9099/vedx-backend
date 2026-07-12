package service

import (
	"fmt"
	"log"
	"net/smtp"
	"time"

	"github.com/umangagarwal/vedx-backend/config"
)

// sendTimeout bounds how long a single SMTP send may take. Some hosting
// providers silently drop outbound SMTP traffic instead of refusing it,
// which would otherwise hang on the OS's default TCP timeout (often a
// minute or more) since net/smtp.SendMail has no built-in deadline.
const sendTimeout = 10 * time.Second

// EmailService sends transactional HTML email over SMTP. Same pattern as
// ZoomService: plain stdlib, no external SDK. net/smtp.SendMail negotiates
// STARTTLS automatically when the server advertises it, which Gmail does on
// port 587.
type EmailService struct {
	cfg config.SMTPConfig
}

func NewEmailService(cfg config.SMTPConfig) *EmailService {
	return &EmailService{cfg: cfg}
}

// Configured reports whether SMTP credentials have been set.
func (e *EmailService) Configured() bool {
	return e.cfg.Configured()
}

// Send delivers an HTML email to a single recipient, bounded by sendTimeout.
// It still blocks the calling goroutine for up to that long, so request
// handlers should use SendAsync instead — this is exported mainly for
// one-off/test callers that want the result directly.
func (e *EmailService) Send(to, subject, htmlBody string) error {
	if !e.Configured() {
		return fmt.Errorf("smtp not configured")
	}

	done := make(chan error, 1)
	go func() { done <- e.send(to, subject, htmlBody) }()

	select {
	case err := <-done:
		return err
	case <-time.After(sendTimeout):
		return fmt.Errorf("smtp send to %s timed out after %s", to, sendTimeout)
	}
}

// SendAsync fires off Send in the background and logs failures. Use this
// from HTTP request handlers so a slow or blocked mail server can never
// stall the API response — email delivery is best-effort and unrelated to
// whether the request itself succeeded.
func (e *EmailService) SendAsync(to, subject, htmlBody string) {
	go func() {
		if err := e.Send(to, subject, htmlBody); err != nil {
			log.Printf("send email to %s: %v", to, err)
		}
	}()
}

func (e *EmailService) send(to, subject, htmlBody string) error {
	fromHeader := e.cfg.FromEmail
	if e.cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", e.cfg.FromName, e.cfg.FromEmail)
	}

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s",
		fromHeader, to, subject, htmlBody,
	)

	auth := smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.Host)
	addr := fmt.Sprintf("%s:%d", e.cfg.Host, e.cfg.Port)
	return smtp.SendMail(addr, auth, e.cfg.FromEmail, []string{to}, []byte(msg))
}
