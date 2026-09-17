package config

import (
	"testing"
)

func TestAuthVerificationConfig(t *testing.T) {
	t.Run("defaults to false when unset", func(t *testing.T) {
		t.Setenv("AUTH_REQUIRE_EMAIL_VERIFICATION", "")
		t.Setenv("AUTH_REQUIRE_PHONE_VERIFICATION", "")
		t.Setenv("REQUIRE_EMAIL_VERIFICATION", "")
		t.Setenv("REQUIRE_PHONE_VERIFICATION", "")

		auth := loadAuth()
		if auth.RequireEmailVerification {
			t.Errorf("expected RequireEmailVerification to default to false, got true")
		}
		if auth.RequirePhoneVerification {
			t.Errorf("expected RequirePhoneVerification to default to false, got true")
		}
	})

	t.Run("respects AUTH_REQUIRE_* variables", func(t *testing.T) {
		t.Setenv("AUTH_REQUIRE_EMAIL_VERIFICATION", "true")
		t.Setenv("AUTH_REQUIRE_PHONE_VERIFICATION", "1")

		auth := loadAuth()
		if !auth.RequireEmailVerification {
			t.Errorf("expected RequireEmailVerification to be true")
		}
		if !auth.RequirePhoneVerification {
			t.Errorf("expected RequirePhoneVerification to be true")
		}
	})

	t.Run("falls back to REQUIRE_* variables", func(t *testing.T) {
		t.Setenv("AUTH_REQUIRE_EMAIL_VERIFICATION", "")
		t.Setenv("AUTH_REQUIRE_PHONE_VERIFICATION", "")
		t.Setenv("REQUIRE_EMAIL_VERIFICATION", "true")
		t.Setenv("REQUIRE_PHONE_VERIFICATION", "true")

		auth := loadAuth()
		if !auth.RequireEmailVerification {
			t.Errorf("expected fallback RequireEmailVerification to be true")
		}
		if !auth.RequirePhoneVerification {
			t.Errorf("expected fallback RequirePhoneVerification to be true")
		}
	})
}
