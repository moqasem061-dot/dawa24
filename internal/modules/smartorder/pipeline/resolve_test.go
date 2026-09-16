package pipeline

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

type codeResolverRepo struct {
	smartorder.Repository
	skus     []string
	barcodes []string
}

func (r *codeResolverRepo) ResolveByCodes(_ context.Context, skus, barcodes []string) (map[string]int64, error) {
	r.skus = skus
	r.barcodes = barcodes
	return map[string]int64{"sku-1234": 42, "1234567890123": 43}, nil
}

func (r *codeResolverRepo) ResolveByLearned(context.Context, int64, []string) (map[string]int64, error) {
	return nil, nil
}

func (r *codeResolverRepo) ResolveByAlias(context.Context, []string) (map[string]int64, error) {
	return nil, nil
}

func TestResolverResolvesSKUsAndBarcodesInOneBulkTier(t *testing.T) {
	repo := &codeResolverRepo{}
	cfg, err := smartorder.NewConfig(1, 9, smartorder.Profile{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := []*smartorder.Line{
		{RawName: "دواء", RawSKU: "SKU-1234", RawBarcode: "1234567890123"},
	}
	Normalize(lines)

	if err := NewResolver(repo, cfg).Resolve(context.Background(), lines); err != nil {
		t.Fatal(err)
	}
	if len(repo.skus) != 1 || repo.skus[0] != "sku-1234" {
		t.Fatalf("SKU batch = %#v", repo.skus)
	}
	if len(repo.barcodes) != 1 || repo.barcodes[0] != "1234567890123" {
		t.Fatalf("barcode batch = %#v", repo.barcodes)
	}
	if lines[0].MatchedProductID == nil || *lines[0].MatchedProductID != 42 {
		t.Fatalf("match = %#v, want SKU product 42", lines[0].MatchedProductID)
	}
	if lines[0].MatchMethod != smartorder.MethodSKU {
		t.Fatalf("method = %q, want SKU", lines[0].MatchMethod)
	}
}
