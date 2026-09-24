package org

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// A link is mutable only by the customer user who owns it or the vendor it
// points at; everyone else sees it as absent.
func TestAuthorizeUserOrgLink(t *testing.T) {
	ctx := context.Background()
	custOrgID := int64(10)
	uo := &UserOrganization{ID: 5, UserID: 42, CustomerOrgID: &custOrgID, VendorOrgID: 100}
	svc := NewService(linkRepo{newMockOrgRepo(), uo}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, tc := range []struct {
		name           string
		userID, vendor int64
		allowed        bool
	}{
		{"owning customer", 42, 0, true},
		{"target vendor", 0, 100, true},
		{"other customer", 43, 0, false},
		{"other vendor", 0, 101, false},
		{"nobody", 0, 0, false},
	} {
		err := svc.AuthorizeUserOrgLink(ctx, uo.ID, tc.userID, tc.vendor)
		if tc.allowed && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		}
		if !tc.allowed && apperr.KindOf(err) != apperr.KindNotFound {
			t.Errorf("%s: got %v; want NotFound", tc.name, err)
		}
	}
}

type linkRepo struct {
	*mockOrgRepo
	link *UserOrganization
}

func (r linkRepo) GetUserOrganizationByID(_ context.Context, id int64) (*UserOrganization, error) {
	if id != r.link.ID {
		return nil, apperr.NotFound("user_organization")
	}
	return r.link, nil
}
