package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func TestConstructors(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		err := apperr.New(apperr.KindConflict, "stock.insufficient", "Not enough stock available")
		if err.Kind != apperr.KindConflict {
			t.Errorf("Kind = %v; want %v", err.Kind, apperr.KindConflict)
		}
		if err.Code != "stock.insufficient" {
			t.Errorf("Code = %q; want %q", err.Code, "stock.insufficient")
		}
		if err.Msg != "Not enough stock available" {
			t.Errorf("Msg = %q; want %q", err.Msg, "Not enough stock available")
		}
	})

	t.Run("Validation", func(t *testing.T) {
		fields := map[string]string{"phone": "must be valid Egyptian mobile"}
		err := apperr.Validation("validation.failed", "Invalid input", fields)
		if err.Kind != apperr.KindValidation {
			t.Errorf("Kind = %v; want %v", err.Kind, apperr.KindValidation)
		}
		if err.Fields["phone"] != "must be valid Egyptian mobile" {
			t.Errorf("Fields[phone] = %q", err.Fields["phone"])
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		err := apperr.NotFound("product")
		if err.Kind != apperr.KindNotFound {
			t.Errorf("Kind = %v; want %v", err.Kind, apperr.KindNotFound)
		}
		if err.Code != "product.not_found" {
			t.Errorf("Code = %q; want %q", err.Code, "product.not_found")
		}
		if err.Msg != "The requested product was not found." {
			t.Errorf("Msg = %q; want %q", err.Msg, "The requested product was not found.")
		}
	})

	t.Run("Conflict and Forbidden", func(t *testing.T) {
		conf := apperr.Conflict("org.duplicate_number", "Organization number already in use")
		if conf.Kind != apperr.KindConflict {
			t.Errorf("Conflict Kind = %v", conf.Kind)
		}

		forb := apperr.Forbidden("auth.insufficient_permissions", "Action not allowed")
		if forb.Kind != apperr.KindForbidden {
			t.Errorf("Forbidden Kind = %v", forb.Kind)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		unauth := apperr.Unauthorized()
		if unauth.Kind != apperr.KindUnauthorized {
			t.Errorf("Kind = %v; want %v", unauth.Kind, apperr.KindUnauthorized)
		}
		if unauth.Code != "auth.required" {
			t.Errorf("Code = %q", unauth.Code)
		}
	})

	t.Run("Internal", func(t *testing.T) {
		cause := errors.New("underlying disk error")
		err := apperr.Internal(cause)
		if err.Kind != apperr.KindInternal {
			t.Errorf("Kind = %v; want %v", err.Kind, apperr.KindInternal)
		}
		if !errors.Is(err, cause) {
			t.Errorf("expected err to unwrap to cause")
		}
	})

	t.Run("Unavailable", func(t *testing.T) {
		cause := errors.New("connection timeout")
		err := apperr.Unavailable("redis", cause)
		if err.Kind != apperr.KindUnavailable {
			t.Errorf("Kind = %v; want %v", err.Kind, apperr.KindUnavailable)
		}
		if err.Code != "redis.unavailable" {
			t.Errorf("Code = %q; want %q", err.Code, "redis.unavailable")
		}
		if !errors.Is(err, cause) {
			t.Errorf("expected err to unwrap to cause")
		}
	})
}

func TestFormattingAndDetail(t *testing.T) {
	err := apperr.New(apperr.KindValidation, "field.invalid", "Field is invalid")
	expectedNoDetail := "validation [field.invalid]: Field is invalid"
	if err.Error() != expectedNoDetail {
		t.Errorf("Error() = %q; want %q", err.Error(), expectedNoDetail)
	}

	withDetail := err.WithDetail("expected integer >= %d, got %d", 1, 0)
	expectedWithDetail := "validation [field.invalid]: Field is invalid (expected integer >= 1, got 0)"
	if withDetail.Error() != expectedWithDetail {
		t.Errorf("Error() with detail = %q; want %q", withDetail.Error(), expectedWithDetail)
	}

	// Verify WithDetail did not mutate original
	if err.Detail != "" {
		t.Errorf("original error Detail mutated")
	}

	// Verify Wrap
	cause := errors.New("database constraint failed")
	wrapped := err.Wrap(cause)
	if wrapped.Unwrap() != cause {
		t.Errorf("wrapped.Unwrap() = %v; want %v", wrapped.Unwrap(), cause)
	}
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected apperr.Kind
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: apperr.KindInternal,
		},
		{
			name:     "standard error",
			err:      errors.New("generic stdlib error"),
			expected: apperr.KindInternal,
		},
		{
			name:     "direct apperr",
			err:      apperr.NotFound("order"),
			expected: apperr.KindNotFound,
		},
		{
			name:     "wrapped apperr",
			err:      fmt.Errorf("outer handler: %w", apperr.Forbidden("auth.forbidden", "Not allowed")),
			expected: apperr.KindForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apperr.KindOf(tt.err)
			if got != tt.expected {
				t.Errorf("KindOf() = %v; want %v", got, tt.expected)
			}
		})
	}
}

func TestAs(t *testing.T) {
	appErr := apperr.Conflict("conflict.code", "Conflict detected")
	wrapped := fmt.Errorf("context wrapper: %w", appErr)

	extracted, ok := apperr.As(wrapped)
	if !ok || extracted == nil {
		t.Fatalf("As() failed to extract AppError")
	}
	if extracted.Code != "conflict.code" {
		t.Errorf("extracted.Code = %q; want %q", extracted.Code, "conflict.code")
	}

	_, ok = apperr.As(errors.New("regular error"))
	if ok {
		t.Errorf("As() returned true for regular non-apperr error")
	}
}
