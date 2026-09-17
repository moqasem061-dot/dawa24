package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestVerifyUserEmailAndPhone(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, log)

	u := &User{
		Email:    "pharmacist@dawa24.eg",
		Phone:    "01012345678",
		Name:     i18n.Text{i18n.AR: "د. أحمد", i18n.EN: "Dr. Ahmed"},
		Role:     "pharmacy",
		Status:   StatusActive,
		Language: i18n.AR,
		Timezone: "Africa/Cairo",
	}
	if err := repo.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if u.EmailVerifiedAt != nil {
		t.Fatalf("expected initial EmailVerifiedAt to be nil")
	}
	if u.PhoneVerifiedAt != nil {
		t.Fatalf("expected initial PhoneVerifiedAt to be nil")
	}

	// Verify email
	newEmail := "verified.pharmacist@dawa24.eg"
	if err := svc.VerifyUserEmail(ctx, u.ID, newEmail); err != nil {
		t.Fatalf("VerifyUserEmail failed: %v", err)
	}

	updated, err := repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.EmailVerifiedAt == nil {
		t.Fatalf("expected EmailVerifiedAt to be non-nil")
	}
	if updated.Email != newEmail {
		t.Fatalf("expected Email = %s, got %s", newEmail, updated.Email)
	}

	// Verify phone
	newPhone := "+201098765432"
	if err := svc.VerifyUserPhone(ctx, u.ID, newPhone); err != nil {
		t.Fatalf("VerifyUserPhone failed: %v", err)
	}

	updated, err = repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.PhoneVerifiedAt == nil {
		t.Fatalf("expected PhoneVerifiedAt to be non-nil")
	}
	if updated.Phone != newPhone {
		t.Fatalf("expected Phone = %s, got %s", newPhone, updated.Phone)
	}
}
