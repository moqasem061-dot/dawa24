package telegramgateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

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
		return nil, fmt.Errorf("%w: %w", ErrInvalidPhone, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, request := range m.requests {
		if time.Now().After(request.expiresAt) {
			delete(m.requests, id)
		}
	}
	if len(m.requests) >= maxOTPEntries {
		return nil, ErrRateLimited
	}

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
		return &CheckResult{Valid: false, Status: "code_invalid", Error: "request not found or expired"}, nil
	}

	if time.Now().After(req.expiresAt) {
		delete(m.requests, requestID)
		return &CheckResult{Valid: false, Status: "expired", Error: "code expired"}, nil
	}

	if code == req.code {
		delete(m.requests, requestID)
		return &CheckResult{Valid: true, Status: "code_valid", PhoneNumber: req.phone}, nil
	}

	return &CheckResult{Valid: false, Status: "code_invalid"}, nil
}
