package catalog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// A product carries both category_id and brand_id, and nothing used to
// constrain them to agree — a cosmetics product could carry a brand that only
// makes medical supplies. The product form filters the brand list client-side;
// these cases cover the server-side rule behind it, which is what a crafted or
// stale form actually meets.

type brandCategoryRepo struct {
	Repository
	pairAllowed bool
	pairErr     error
	created     bool
}

func (r *brandCategoryRepo) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return r.pairAllowed, r.pairErr
}

func (r *brandCategoryRepo) CreateProduct(_ context.Context, _ *Product) error {
	r.created = true
	return nil
}

func (r *brandCategoryRepo) UpdateProduct(_ context.Context, _ *Product) error {
	r.created = true
	return nil
}

func productWith(categoryID, brandID *int64) *Product {
	return &Product{
		OrganizationID: 1,
		Name:           i18n.Text{i18n.AR: "بانادول", i18n.EN: "Panadol"},
		CategoryID:     categoryID,
		BrandID:        brandID,
	}
}

func id(v int64) *int64 { return &v }

func TestAssertBrandInCategory(t *testing.T) {
	cases := []struct {
		name        string
		categoryID  *int64
		brandID     *int64
		pairAllowed bool
		wantErr     bool
		wantWritten bool
	}{
		{
			name:       "a brand that operates in the category is accepted",
			categoryID: id(7), brandID: id(3), pairAllowed: true,
			wantWritten: true,
		},
		{
			name:       "a brand outside the category is refused",
			categoryID: id(7), brandID: id(3), pairAllowed: false,
			wantErr: true,
		},
		{
			// Both columns are nullable and legacy rows often carry only one.
			name:       "a product with no brand is left alone",
			categoryID: id(7), brandID: nil, pairAllowed: false,
			wantWritten: true,
		},
		{
			name:       "a product with no category is left alone",
			categoryID: nil, brandID: id(3), pairAllowed: false,
			wantWritten: true,
		},
	}

	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			repo := &brandCategoryRepo{pairAllowed: tc.pairAllowed}
			svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

			_, err := svc.CreateProduct(context.Background(), productWith(tc.categoryID, tc.brandID))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected the mismatched pair to be refused")
				}
				var ae *apperr.Error
				if !errors.As(err, &ae) || ae.Kind != apperr.KindValidation {
					t.Fatalf("want a validation error, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.created != tc.wantWritten {
				t.Errorf("repository write = %v, want %v", repo.created, tc.wantWritten)
			}
		})

		t.Run("update/"+tc.name, func(t *testing.T) {
			repo := &brandCategoryRepo{pairAllowed: tc.pairAllowed}
			svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

			p := productWith(tc.categoryID, tc.brandID)
			p.ID = 42
			err := svc.UpdateProduct(context.Background(), p)
			if tc.wantErr && err == nil {
				t.Fatal("expected the mismatched pair to be refused on update too")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.created != tc.wantWritten {
				t.Errorf("repository write = %v, want %v", repo.created, tc.wantWritten)
			}
		})
	}
}

// A failing lookup must not be read as permission to save.
func TestAssertBrandInCategory_LookupErrorBlocksTheWrite(t *testing.T) {
	repo := &brandCategoryRepo{pairErr: errors.New("database unavailable")}
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := svc.CreateProduct(context.Background(), productWith(id(7), id(3))); err == nil {
		t.Fatal("expected the lookup failure to surface")
	}
	if repo.created {
		t.Fatal("the product must not be written when the pair could not be checked")
	}
}
