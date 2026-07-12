package service

import (
	"fmt"
	"net/smtp"

	"github.com/umangagarwal/vedx-backend/config"
)

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

// Send delivers an HTML email to a single recipient. Callers should log
// failures rather than fail the parent operation — a bad address or a
// transient SMTP error shouldn't block session/batch creation.
func (e *EmailService) Send(to, subject, htmlBody string) error {
	if !e.Configured() {
		return fmt.Errorf("smtp not configured")
	}

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
