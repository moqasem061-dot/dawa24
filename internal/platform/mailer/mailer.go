package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"encoding/base64"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// Message encapsulates an outbound email.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Mailer abstracts sending emails across the platform.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
	SendOTP(ctx context.Context, to, code, purpose, lang string) error
	SendNotification(ctx context.Context, to, title, body, actionURL, lang string) error
	IsMock() bool
}

// New constructs a Mailer from configuration. If host is blank or disabled,
// a mock mailer is returned that logs outgoing messages for zero-downtime testing.
func New(cfg config.SMTP, log *slog.Logger) Mailer {
	if cfg.Host == "" || !cfg.Enabled {
		return &mockMailer{log: log, cfg: cfg}
	}
	return &smtpMailer{cfg: cfg, log: log}
}

// NewMockMailer creates a mock Mailer for tests and fallback without an SMTP connection.
func NewMockMailer(log *slog.Logger) Mailer {
	return &mockMailer{log: log, cfg: config.SMTP{}}
}

// mockMailer logs messages to slog without connecting to an SMTP server.
type mockMailer struct {
	log *slog.Logger
	cfg config.SMTP
}

func (m *mockMailer) IsMock() bool { return true }

func (m *mockMailer) Send(ctx context.Context, msg Message) error {
	m.log.InfoContext(ctx, "[SMTP MOCK] email sent",
		"to", msg.To,
		"subject", msg.Subject,
		"text", msg.Text)
	return nil
}

func (m *mockMailer) SendOTP(ctx context.Context, to, code, purpose, lang string) error {
	subj, html, text := RenderOTPEmail(code, purpose, lang)
	m.log.InfoContext(ctx, "[SMTP MOCK] OTP email sent",
		"to", to,
		"code", code,
		"purpose", purpose,
		"subject", subj)
	return m.Send(ctx, Message{
		To:      to,
		Subject: subj,
		HTML:    html,
		Text:    text,
	})
}

func (m *mockMailer) SendNotification(ctx context.Context, to, title, body, actionURL, lang string) error {
	subj, html, text := RenderNotificationEmail(title, body, actionURL, lang)
	return m.Send(ctx, Message{
		To:      to,
		Subject: subj,
		HTML:    html,
		Text:    text,
	})
}

// smtpMailer delivers emails using standard SMTP over STARTTLS or TLS.
type smtpMailer struct {
	cfg config.SMTP
	log *slog.Logger
}

func (s *smtpMailer) IsMock() bool { return false }

func (s *smtpMailer) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return fmt.Errorf("mailer: recipient address is empty")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	from := s.cfg.FromEmail
	if from == "" {
		from = "no-reply@dawa24.net"
	}

	// Prepare MIME message
	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.cfg.FromName, from))
	body.WriteString(fmt.Sprintf("To: %s\r\n", msg.To))
	body.WriteString(fmt.Sprintf("Subject: =?UTF-8?B?%s?=\r\n", encodeRFC2047(msg.Subject)))
	body.WriteString("MIME-Version: 1.0\r\n")

	boundary := fmt.Sprintf("=_dawa24_%d", time.Now().UnixNano())
	body.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

	// Plaintext part
	if msg.Text != "" {
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		body.WriteString(msg.Text)
		body.WriteString("\r\n\r\n")
	}

	// HTML part
	if msg.HTML != "" {
		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		body.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		body.WriteString(msg.HTML)
		body.WriteString("\r\n\r\n")
	}
	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	rawMsg := []byte(body.String())

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	timeout := s.cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	// Handle direct TLS (Port 465)
	if s.cfg.Port == 465 || strings.EqualFold(s.cfg.Encryption, "tls") {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("mailer: tls dial error: %w", err)
		}
		defer conn.Close()

		c, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return fmt.Errorf("mailer: smtp client error: %w", err)
		}
		defer c.Quit()

		if auth != nil {
			if err = c.Auth(auth); err != nil {
				return fmt.Errorf("mailer: smtp auth error: %w", err)
			}
		}
		if err = c.Mail(from); err != nil {
			return err
		}
		if err = c.Rcpt(msg.To); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err = w.Write(rawMsg); err != nil {
			return err
		}
		return w.Close()
	}

	// Standard STARTTLS (Port 587 or 25)
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mailer: dial error: %w", err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: smtp client error: %w", err)
	}
	defer c.Quit()

	if ok, _ := c.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		if err = c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mailer: starttls error: %w", err)
		}
	}

	if auth != nil {
		if err = c.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth error: %w", err)
		}
	}

	if err = c.Mail(from); err != nil {
		return err
	}
	if err = c.Rcpt(msg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(rawMsg); err != nil {
		return err
	}
	return w.Close()
}

func (s *smtpMailer) SendOTP(ctx context.Context, to, code, purpose, lang string) error {
	subj, html, text := RenderOTPEmail(code, purpose, lang)
	return s.Send(ctx, Message{
		To:      to,
		Subject: subj,
		HTML:    html,
		Text:    text,
	})
}

func (s *smtpMailer) SendNotification(ctx context.Context, to, title, body, actionURL, lang string) error {
	subj, html, text := RenderNotificationEmail(title, body, actionURL, lang)
	return s.Send(ctx, Message{
		To:      to,
		Subject: subj,
		HTML:    html,
		Text:    text,
	})
}

// encodeRFC2047 encodes subject strings using base64 RFC 2047 format.
func encodeRFC2047(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
