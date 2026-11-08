package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// The ERP sync writes quotas in bulk: one call per chunk, a map of variant id
// to the new per-branch cap. A nil (or normalised-to-nil) cap clears the
// column; an empty map must not reach the repository at all.

type quotaWriterRepo struct {
	catalog.Repository
	calls []map[int64]*int
	evict func()
}

func (r *quotaWriterRepo) SetVariantQuotaLimits(_ context.Context, _ int64, limits map[int64]*int) error {
	r.calls = append(r.calls, limits)
	if r.evict != nil {
		r.evict()
	}
	return nil
}

type quotaPlainRepo struct {
	catalog.Repository
}

func TestSetVariantQuotaLimits(t *testing.T) {
	ctx := context.Background()
	intPtr := func(v int) *int { return &v }

	t.Run("values pass through NormalizeQuotaLimit", func(t *testing.T) {
		repo := &quotaWriterRepo{}
		svc := catalog.NewService(repo, nil)
		err := svc.SetVariantQuotaLimits(ctx, 7, map[int64]*int{1: intPtr(5), 2: intPtr(0), 3: intPtr(-3)})
		if err != nil {
			t.Fatal(err)
		}
		if len(repo.calls) != 1 {
			t.Fatalf("repository calls = %d, want 1", len(repo.calls))
		}
		got := repo.calls[0]
		if got[1] == nil || *got[1] != 5 {
			t.Errorf("variant 1 limit = %v, want 5 untouched", got[1])
		}
		if got[2] != nil {
			t.Errorf("variant 2 limit = %v, want nil (0 means no quota)", got[2])
		}
		if got[3] != nil {
			t.Errorf("variant 3 limit = %v, want nil (negative means no quota)", got[3])
		}
	})

	t.Run("nil clears to null", func(t *testing.T) {
		repo := &quotaWriterRepo{}
		svc := catalog.NewService(repo, nil)
		if err := svc.SetVariantQuotaLimits(ctx, 7, map[int64]*int{9: nil}); err != nil {
			t.Fatal(err)
		}
		if repo.calls[0][9] != nil {
			t.Errorf("nil limit = %v, want nil (clears the column)", repo.calls[0][9])
		}
	})

	t.Run("empty map is a no-op", func(t *testing.T) {
		repo := &quotaWriterRepo{}
		svc := catalog.NewService(repo, nil)
		if err := svc.SetVariantQuotaLimits(ctx, 7, map[int64]*int{}); err != nil {
			t.Fatal(err)
		}
		if len(repo.calls) != 0 {
			t.Errorf("repository called %d times for an empty map, want 0", len(repo.calls))
		}
	})

	t.Run("repository without the optional writer", func(t *testing.T) {
		svc := catalog.NewService(&quotaPlainRepo{}, nil)
		err := svc.SetVariantQuotaLimits(ctx, 7, map[int64]*int{1: intPtr(5)})
		if !errors.Is(err, catalog.ErrBulkVariantsUnavailable) {
			t.Fatalf("err = %v, want ErrBulkVariantsUnavailable", err)
		}
	})
}
