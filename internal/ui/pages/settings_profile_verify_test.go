package pages

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
)

// Verification is optional at sign-up, so the profile is the only place an
// unverified account can verify later. The UI for it was dropped once in an
// unrelated commit, and before that it loaded its script from /js/, a path
// nothing serves. This pins both: the controls render for an unverified user,
// and the script comes from /static/js/.
func TestProfileVerificationRendersForUnverifiedUser(t *testing.T) {
	u := &identity.User{Email: "a@example.com"}

	var sb strings.Builder
	ctx := context.Background()
	if err := ProfileEmailField(u, "ar").Render(ctx, &sb); err != nil {
		t.Fatalf("render email: %v", err)
	}
	if err := ProfilePhoneField(u, "ar").Render(ctx, &sb); err != nil {
		t.Fatalf("render phone: %v", err)
	}
	if err := ProfileVerifyScript().Render(ctx, &sb); err != nil {
		t.Fatalf("render script: %v", err)
	}
	html := sb.String()

	for _, id := range []string{
		"profile-verify-email-open-btn", "profile-email-otp-box", "profile-email-otp-code",
		"profile-verify-phone-open-btn", "profile-phone-otp-box", "profile-phone-otp-code",
	} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("unverified profile is missing #%s", id)
		}
	}
	if !strings.Contains(html, "/static/js/profile_verification.js") {
		t.Error("the verification script is not loaded from /static/js/")
	}
}

func TestProfileVerificationHidesControlsOnceVerified(t *testing.T) {
	now := time.Now()
	u := &identity.User{Email: "a@example.com", EmailVerifiedAt: &now, PhoneVerifiedAt: &now}

	var sb strings.Builder
	if err := ProfileEmailField(u, "ar").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if err := ProfilePhoneField(u, "ar").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	html := sb.String()
	if strings.Contains(html, "profile-verify-email-open-btn") || strings.Contains(html, "profile-verify-phone-open-btn") {
		t.Error("a verified account is offered verification again")
	}
	if !strings.Contains(html, `id="profile-email-badge-verified"`) || !strings.Contains(html, `id="profile-phone-badge-verified"`) {
		t.Error("a verified account does not show its verified badges")
	}
}
