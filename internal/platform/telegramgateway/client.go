package telegramgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalidPhone       = errors.New("telegramgateway: invalid phone number")
	ErrRateLimited        = errors.New("telegramgateway: rate limited")
	ErrServiceUnavailable = errors.New("telegramgateway: service unavailable")
)

// Client defines the interface for interacting with Telegram Gateway or its mock.
type Client interface {
	SendVerification(ctx context.Context, phone string) (*SendResult, error)
	CheckVerification(ctx context.Context, requestID, code string) (*CheckResult, error)
	IsMock() bool
}

// SendResult holds the outcome of a sendVerificationMessage call.
type SendResult struct {
	RequestID   string `json:"request_id"`
	PhoneNumber string `json:"phone_number"`
	ExpiresIn   int    `json:"expires_in"` // seconds (default 300)
	MockCode    string `json:"mock_code,omitempty"`
}

// CheckResult holds the outcome of a checkVerificationStatus call.
type CheckResult struct {
	Valid       bool   `json:"valid"`
	Status      string `json:"status"` // "code_valid", "code_invalid", "expired", etc.
	Error       string `json:"error,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// Config configures the Telegram Gateway client.
type Config struct {
	Token      string
	BaseURL    string
	HTTPClient *http.Client
	MockMode   bool
	Log        *slog.Logger
}

// New creates a new Client. If Token is empty or "mock", or MockMode is true,
// it returns a mock client suitable for dev, staging, and tests.
func New(cfg Config) Client {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://gatewayapi.telegram.org"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	token := strings.TrimSpace(cfg.Token)
	if cfg.MockMode || token == "" || strings.EqualFold(token, "mock") || strings.EqualFold(token, "sandbox") {
		return &mockClient{
			log:      cfg.Log,
			requests: make(map[string]mockRequest),
		}
	}

	return &liveClient{
		token:      token,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
		log:        cfg.Log,
	}
}

// --- Live Telegram Gateway Client ---

type liveClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
	log        *slog.Logger
}

func (c *liveClient) IsMock() bool { return false }

type tgSendReq struct {
	PhoneNumber string `json:"phone_number"`
	CodeLength  int    `json:"code_length"`
	TTL         int    `json:"ttl"`
}

type tgSendResp struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
	Result      struct {
		RequestID      string `json:"request_id"`
		PhoneNumber    string `json:"phone_number"`
		DeliveryStatus struct {
			Status string `json:"status"`
		} `json:"delivery_status"`
	} `json:"result"`
}

func (c *liveClient) SendVerification(ctx context.Context, rawPhone string) (*SendResult, error) {
	normPhone, err := NormalizePhone(rawPhone)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPhone, err)
	}

	payload := tgSendReq{
		PhoneNumber: normPhone,
		CodeLength:  6,
		TTL:         300,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sendVerificationMessage", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.ErrorContext(ctx, "telegram gateway sendVerificationMessage request error", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	respBytes, err := readGatewayResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	var tgResp tgSendResp
	if err := json.Unmarshal(respBytes, &tgResp); err != nil {
		c.log.ErrorContext(ctx, "telegram gateway send parse error", "status", resp.StatusCode)
		return nil, fmt.Errorf("%w: failed to parse response", ErrServiceUnavailable)
	}

	if !tgResp.OK {
		c.log.WarnContext(ctx, "telegram gateway send returned error", "error", tgResp.Error, "desc", tgResp.Description)
		switch strings.ToUpper(tgResp.Error) {
		case "PHONE_NUMBER_INVALID":
			return nil, ErrInvalidPhone
		case "FLOOD_WAIT", "TOO_MANY_REQUESTS":
			return nil, ErrRateLimited
		default:
			return nil, fmt.Errorf("telegram gateway: %s (%s)", tgResp.Error, tgResp.Description)
		}
	}
	if tgResp.Result.RequestID == "" {
		return nil, ErrServiceUnavailable
	}

	return &SendResult{
		RequestID:   tgResp.Result.RequestID,
		PhoneNumber: normPhone,
		ExpiresIn:   300,
	}, nil
}

type tgCheckReq struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
}

type tgCheckResp struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
	Result      struct {
		RequestID   string `json:"request_id"`
		PhoneNumber string `json:"phone_number"`
		Status      struct {
			Status string `json:"status"`
		} `json:"verification_status"`
	} `json:"result"`
}

func (c *liveClient) CheckVerification(ctx context.Context, requestID, code string) (*CheckResult, error) {
	requestID = strings.TrimSpace(requestID)
	code = strings.TrimSpace(code)
	if requestID == "" || code == "" {
		return &CheckResult{Valid: false, Status: "code_invalid", Error: "missing request_id or code"}, nil
	}

	payload := tgCheckReq{
		RequestID: requestID,
		Code:      code,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/checkVerificationStatus", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.ErrorContext(ctx, "telegram gateway checkVerificationStatus request error", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	respBytes, err := readGatewayResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	var tgResp tgCheckResp
	if err := json.Unmarshal(respBytes, &tgResp); err != nil {
		c.log.ErrorContext(ctx, "telegram gateway check parse error", "status", resp.StatusCode)
		return nil, fmt.Errorf("%w: failed to parse response", ErrServiceUnavailable)
	}

	if !tgResp.OK {
		c.log.WarnContext(ctx, "telegram gateway check returned error", "error", tgResp.Error, "desc", tgResp.Description)
		return &CheckResult{
			Valid:  false,
			Status: strings.ToLower(tgResp.Error),
			Error:  tgResp.Description,
		}, nil
	}
	if tgResp.Result.RequestID != requestID {
		return nil, ErrServiceUnavailable
	}

	status := strings.ToLower(tgResp.Result.Status.Status)
	isValid := status == "code_valid"

	return &CheckResult{
		Valid:       isValid,
		Status:      status,
		PhoneNumber: tgResp.Result.PhoneNumber,
	}, nil
}
