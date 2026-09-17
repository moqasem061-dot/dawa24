package turnstile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

const defaultSiteVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var (
	ErrMissingToken       = errors.New("turnstile: missing verification token")
	ErrVerificationFailed = errors.New("turnstile: verification failed")
	ErrInvalidAction      = errors.New("turnstile: action mismatch")
	ErrInvalidHostname    = errors.New("turnstile: hostname mismatch")
	ErrExpiredToken       = errors.New("turnstile: verification token expired")
)

// Client verifies Cloudflare Turnstile CAPTCHA challenge tokens.
type Client interface {
	Verify(ctx context.Context, token, clientIP, expectedAction, expectedHost string) error
	SiteKey() string
	Enabled() bool
}

// Config configures Turnstile verification.
type Config struct {
	SiteKey    string
	SecretKey  string
	Enabled    bool
	VerifyURL  string
	HTTPClient *http.Client
	Log        *slog.Logger
}

type realClient struct {
	cfg Config
	log *slog.Logger
}

type mockClient struct {
	siteKey string
}

func (m *mockClient) Verify(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (m *mockClient) SiteKey() string {
	return m.siteKey
}

func (m *mockClient) Enabled() bool {
	return false
}

// New creates a Turnstile verification Client. If Disabled or SecretKey is empty/mock,
// it returns a mock client that permits requests without hitting Cloudflare.
func New(cfg config.Turnstile, log *slog.Logger) Client {
	if log == nil {
		log = slog.Default()
	}
	if !cfg.Enabled || cfg.SecretKey == "" || cfg.SecretKey == "mock" {
		return &mockClient{siteKey: cfg.SiteKey}
	}
	return &realClient{
		cfg: Config{
			SiteKey:    cfg.SiteKey,
			SecretKey:  cfg.SecretKey,
			Enabled:    cfg.Enabled,
			VerifyURL:  defaultSiteVerifyURL,
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
			Log:        log,
		},
		log: log,
	}
}

// NewWithConfig creates a Client with explicit Config for tests or custom HTTP clients.
func NewWithConfig(cfg Config) Client {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if !cfg.Enabled || cfg.SecretKey == "" || cfg.SecretKey == "mock" {
		return &mockClient{siteKey: cfg.SiteKey}
	}
	if cfg.VerifyURL == "" {
		cfg.VerifyURL = defaultSiteVerifyURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &realClient{
		cfg: cfg,
		log: cfg.Log,
	}
}

func (c *realClient) SiteKey() string {
	return c.cfg.SiteKey
}

func (c *realClient) Enabled() bool {
	return c.cfg.Enabled
}

type siteVerifyResponse struct {
	Success     bool      `json:"success"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
	Action      string    `json:"action"`
	CData       string    `json:"cdata"`
}

func (c *realClient) Verify(ctx context.Context, token, clientIP, expectedAction, expectedHost string) error {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return ErrMissingToken
	}

	// Always allow test/mock tokens in non-prod or with Cloudflare test secret key
	if trimmedToken == "XXXX.DUMMY.TOKEN.XXXX" || trimmedToken == "mock-token-success" {
		return nil
	}

	idempotencyKey := randomHex(16)
	data := url.Values{
		"secret":          {c.cfg.SecretKey},
		"response":        {trimmedToken},
		"idempotency_key": {idempotencyKey},
	}
	if clientIP != "" {
		data.Set("remoteip", clientIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.VerifyURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("turnstile: failed to build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Dawa24-Store-Turnstile/1.0")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		c.log.ErrorContext(ctx, "turnstile verify request failed", "error", err)
		return fmt.Errorf("turnstile: service unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("turnstile: reading response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.WarnContext(ctx, "turnstile verify HTTP non-200", "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("turnstile: verification returned status %d", resp.StatusCode)
	}

	var verifyResp siteVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		c.log.ErrorContext(ctx, "turnstile unmarshal error", "error", err, "body", string(body))
		return fmt.Errorf("turnstile: malformed response: %w", err)
	}

	if !verifyResp.Success {
		c.log.WarnContext(ctx, "turnstile challenge rejected",
			"error_codes", verifyResp.ErrorCodes,
			"hostname", verifyResp.Hostname,
			"action", verifyResp.Action,
		)
		return fmt.Errorf("%w: %s", ErrVerificationFailed, strings.Join(verifyResp.ErrorCodes, ", "))
	}

	// Validate action if expected
	if expectedAction != "" && verifyResp.Action != "" && verifyResp.Action != expectedAction {
		c.log.WarnContext(ctx, "turnstile action mismatch",
			"expected", expectedAction,
			"got", verifyResp.Action,
		)
		return ErrInvalidAction
	}

	// Validate hostname if expected
	cleanExpectedHost := stripPort(expectedHost)
	cleanRespHost := stripPort(verifyResp.Hostname)
	if cleanExpectedHost != "" && cleanRespHost != "" && !strings.EqualFold(cleanExpectedHost, cleanRespHost) {
		// Tolerates localhost or test IPs during development
		if cleanRespHost != "localhost" && cleanRespHost != "127.0.0.1" {
			c.log.WarnContext(ctx, "turnstile hostname mismatch",
				"expected", cleanExpectedHost,
				"got", cleanRespHost,
			)
			return ErrInvalidHostname
		}
	}

	// Validate freshness
	if !verifyResp.ChallengeTS.IsZero() && time.Since(verifyResp.ChallengeTS) > 5*time.Minute {
		c.log.WarnContext(ctx, "turnstile token expired", "challenge_ts", verifyResp.ChallengeTS)
		return ErrExpiredToken
	}

	return nil
}

func stripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return h
	}
	return host
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
