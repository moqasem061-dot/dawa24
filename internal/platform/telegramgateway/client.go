package telegramgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidPhone        = errors.New("telegramgateway: invalid phone number")
	ErrInvalidCode         = errors.New("telegramgateway: verification code is invalid")
	ErrExpiredCode         = errors.New("telegramgateway: verification code has expired")
	ErrMaxAttemptsExceeded = errors.New("telegramgateway: maximum verification attempts exceeded")
	ErrRateLimited         = errors.New("telegramgateway: rate limited")
	ErrServiceUnavailable  = errors.New("telegramgateway: service unavailable")
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
	Valid  bool   `json:"valid"`
	Status string `json:"status"` // "code_valid", "code_invalid", "expired", etc.
	Error  string `json:"error,omitempty"`
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
		RequestID      string  `json:"request_id"`
		PhoneNumber    string  `json:"phone_number"`
		RequestCost    float64 `json:"request_cost"`
		RemainingBal   float64 `json:"remaining_balance"`
		DeliveryStatus struct {
			Status string `json:"status"`
		} `json:"delivery_status"`
	} `json:"result"`
}

func (c *liveClient) SendVerification(ctx context.Context, rawPhone string) (*SendResult, error) {
	normPhone, err := NormalizePhone(rawPhone)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPhone, err)
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
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tgResp tgSendResp
	if err := json.Unmarshal(respBytes, &tgResp); err != nil {
		c.log.ErrorContext(ctx, "telegram gateway send parse error", "status", resp.StatusCode, "body", string(respBytes))
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
		RequestID string `json:"request_id"`
		Status    struct {
			Status string `json:"status"`
		} `json:"status"`
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
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tgResp tgCheckResp
	if err := json.Unmarshal(respBytes, &tgResp); err != nil {
		c.log.ErrorContext(ctx, "telegram gateway check parse error", "status", resp.StatusCode, "body", string(respBytes))
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

	status := strings.ToLower(tgResp.Result.Status.Status)
	isValid := status == "code_valid"

	return &CheckResult{
		Valid:  isValid,
		Status: status,
	}, nil
}

// --- Mock Telegram Gateway Client ---

type mockRequest struct {
	phone     string
	code      string
	expiresAt time.Time
}

type mockClient struct {
	log      *slog.Logger
	mu       sync.Mutex
	requests map[string]mockRequest
}

func (m *mockClient) IsMock() bool { return true }

func (m *mockClient) SendVerification(ctx context.Context, rawPhone string) (*SendResult, error) {
	normPhone, err := NormalizePhone(rawPhone)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPhone, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reqID := fmt.Sprintf("mock_req_%d", time.Now().UnixNano())
	code := "123456"

	m.requests[reqID] = mockRequest{
		phone:     normPhone,
		code:      code,
		expiresAt: time.Now().Add(5 * time.Minute),
	}

	m.log.InfoContext(ctx, "[TELEGRAM GATEWAY MOCK] Verification OTP issued",
		"phone", normPhone,
		"code", code,
		"request_id", reqID,
	)

	return &SendResult{
		RequestID:   reqID,
		PhoneNumber: normPhone,
		ExpiresIn:   300,
		MockCode:    code,
	}, nil
}

func (m *mockClient) CheckVerification(ctx context.Context, requestID, code string) (*CheckResult, error) {
	requestID = strings.TrimSpace(requestID)
	code = strings.TrimSpace(code)

	if requestID == "" || code == "" {
		return &CheckResult{Valid: false, Status: "code_invalid", Error: "missing code or request_id"}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[requestID]
	if !ok {
		// In mock mode, also accept default mock code 123456 for any mock prefix
		if strings.HasPrefix(requestID, "mock_") && code == "123456" {
			return &CheckResult{Valid: true, Status: "code_valid"}, nil
		}
		return &CheckResult{Valid: false, Status: "code_invalid", Error: "request not found or expired"}, nil
	}

	if time.Now().After(req.expiresAt) {
		delete(m.requests, requestID)
		return &CheckResult{Valid: false, Status: "expired", Error: "code expired"}, nil
	}

	if code == req.code || code == "123456" {
		delete(m.requests, requestID)
		return &CheckResult{Valid: true, Status: "code_valid"}, nil
	}

	return &CheckResult{Valid: false, Status: "code_invalid"}, nil
}

// --- Phone Number Normalization ---

// NormalizePhone converts Egyptian and international numbers into canonical E.164 format (+201...).
func NormalizePhone(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty phone number")
	}

	// 1. Convert Arabic-Indic digits (٠١٢٣٤٥٦٧٨٩) to standard ASCII digits (0-9)
	var sb strings.Builder
	for _, r := range input {
		switch {
		case r >= '٠' && r <= '٩':
			sb.WriteRune('0' + (r - '٠'))
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '+':
			if sb.Len() == 0 {
				sb.WriteRune('+')
			}
		}
	}
	clean := sb.String()

	// 2. Remove leading "00" and replace with "+"
	if strings.HasPrefix(clean, "00") {
		clean = "+" + clean[2:]
	}

	// 3. Handle Egyptian national format: e.g. 01012345678, 011..., 012..., 015... (11 digits)
	egyptianNational := regexp.MustCompile(`^0(1[0125]\d{8})$`)
	if m := egyptianNational.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 4. Handle Egyptian with country code without plus: 201012345678 (12 digits)
	egyptianNoPlus := regexp.MustCompile(`^20(1[0125]\d{8})$`)
	if m := egyptianNoPlus.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 5. Handle Egyptian with country code with plus: +201012345678 (13 chars)
	egyptianPlus := regexp.MustCompile(`^\+20(1[0125]\d{8})$`)
	if m := egyptianPlus.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 6. Generic international E.164: + followed by 8 to 15 digits
	e164Regex := regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
	if e164Regex.MatchString(clean) {
		return clean, nil
	}

	return "", fmt.Errorf("unsupported phone format: %q", input)
}
