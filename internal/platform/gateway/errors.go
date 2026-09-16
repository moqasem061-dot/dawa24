package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// The closed error set. Callers branch on exactly these; no raw HTTP status or
// upstream provider message ever escapes this package. Every one of them means
// the same thing to a caller: use your fallback.
var (
	// ErrDisabled means the Gateway is switched off by configuration. Expected
	// in local development and during early phases before a virtual key exists.
	ErrDisabled = errors.New("gateway: disabled")

	// ErrUnavailable covers transport failure and 5xx. Retrying later may work.
	ErrUnavailable = errors.New("gateway: unavailable")

	// ErrCircuitOpen means we stopped calling after repeated failures.
	ErrCircuitOpen = errors.New("gateway: circuit open")

	// ErrTimeout means the capability's latency budget was exhausted.
	ErrTimeout = errors.New("gateway: timeout")

	// ErrQuotaExceeded means this organisation is out of AI budget. This is a
	// business condition, not an outage: surface it to the user as a limit.
	ErrQuotaExceeded = errors.New("gateway: quota exceeded")

	// ErrRateLimited means slow down; the request itself was fine.
	ErrRateLimited = errors.New("gateway: rate limited")

	// ErrUnauthorized means the virtual key is missing, wrong, or revoked. This
	// is a deployment fault and should page someone.
	ErrUnauthorized = errors.New("gateway: unauthorized")

	// ErrBadRequest means we sent something invalid. Our bug; do not retry.
	ErrBadRequest = errors.New("gateway: bad request")

	// ErrUnknownCapability means a capability has no configured budget.
	ErrUnknownCapability = errors.New("gateway: unknown capability")
)

// ShouldFallback reports whether a caller should serve its deterministic path.
//
// This is the single decision point every AI-touching feature consults, which is
// why it is one function rather than a switch repeated in six modules. Note that
// ErrBadRequest is included: our own malformed request must still not break a
// vendor's import.
func ShouldFallback(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrDisabled),
		errors.Is(err, ErrUnavailable),
		errors.Is(err, ErrCircuitOpen),
		errors.Is(err, ErrTimeout),
		errors.Is(err, ErrRateLimited),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrBadRequest),
		errors.Is(err, ErrUnknownCapability):
		return true
	case errors.Is(err, ErrQuotaExceeded):
		// Budget exhaustion is deliberate. Fall back so the feature keeps
		// working, and let the billing module warn the organisation.
		return true
	default:
		return true
	}
}

func isRetryable(err error) bool {
	switch {
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrTimeout), errors.Is(err, ErrRateLimited):
		return true
	default:
		return false
	}
}

// classifyStatus maps a Gateway HTTP response onto the closed error set.
func classifyStatus(status int, body []byte) error {
	detail := extractMessage(body)

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrUnauthorized, detail)
	case http.StatusPaymentRequired:
		return fmt.Errorf("%w: %s", ErrQuotaExceeded, detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, detail)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s", ErrTimeout, detail)
	}

	switch {
	case status >= 500:
		return fmt.Errorf("%w: upstream %d: %s", ErrUnavailable, status, detail)
	case status >= 400:
		return fmt.Errorf("%w: upstream %d: %s", ErrBadRequest, status, detail)
	default:
		return fmt.Errorf("%w: unexpected status %d", ErrUnavailable, status)
	}
}

// extractMessage pulls a human-readable reason out of an error body without
// letting arbitrary upstream text grow unbounded in our logs.
func extractMessage(body []byte) string {
	if len(body) == 0 {
		return "no body"
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if envelope.Error.Message != "" {
			return truncate(envelope.Error.Message, 300)
		}
		if envelope.Message != "" {
			return truncate(envelope.Message, 300)
		}
	}
	return truncate(string(body), 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
