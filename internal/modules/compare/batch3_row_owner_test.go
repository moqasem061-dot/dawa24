package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// Batch 3 (S13): a manual match may change only rows of the caller's own files.
// The fixture is an organization-less upload by user 1, which RLS shows to every
// tenant, so only the owner check stops another user.
func TestSaveManualCorrectionOnlyOwnRows(t *testing.T) {
	cases := []struct {
		name    string
		actor   *authctx.Actor
		wantErr bool
	}{
		{"no caller identity is refused", nil, true},
		{"another user's org-less file is refused", &authctx.Actor{UserID: 2, OrganizationID: 9}, true},
		{"the uploader may correct their own row", &authctx.Actor{UserID: 1}, false},
		{"staff may correct any file", &authctx.Actor{UserID: 2, IsStaff: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCompareRepo()
			repo.files[1] = &compare.CompareFile{ID: 1, UserID: 1}
			repo.fileRows[1] = []*compare.CompareFileRow{{ID: 10, FileID: 1}}
			svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

			ctx := context.Background()
			if tc.actor != nil {
				ctx = authctx.WithActor(ctx, *tc.actor)
			}
			err := svc.SaveManualCorrection(ctx, nil, 10, "بانادول", 5)
			if tc.wantErr {
				if apperr.KindOf(err) != apperr.KindNotFound {
					t.Fatalf("err=%v, want not found", err)
				}
				if repo.fileRows[1][0].MatchMethod == compare.MatchMethodManual {
					t.Fatal("row changed despite the refusal")
				}
				return
			}
			if err != nil {
				t.Fatalf("SaveManualCorrection: %v", err)
			}
			if repo.fileRows[1][0].MatchMethod != compare.MatchMethodManual {
				t.Fatal("row not changed for an allowed caller")
			}
		})
	}
}

// fileOwnedBy reports whether a file belongs to the owner by user or organization.
func (m *mockCompareRepo) fileOwnedBy(fileID int64, owner compare.RowOwner) bool {
	f, ok := m.files[fileID]
	if !ok {
		return false
	}
	if f.UserID == owner.UserID {
		return true
	}
	return owner.OrganizationID > 0 && f.OrganizationID != nil && *f.OrganizationID == owner.OrganizationID
}
