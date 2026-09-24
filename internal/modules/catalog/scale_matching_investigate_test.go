package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestProfileMatchingAtScale(t *testing.T) {
	const n = 19_996
	t.Logf("Creating synthetic catalogue of 20,000 products...")
	masters := make([]productmatch.MasterProduct, 20_000)
	for i := 0; i < 20_000; i++ {
		masters[i] = productmatch.MasterProduct{
			ID:          int64(i + 1),
			NameAR:      fmt.Sprintf("بانادول اكسترا اقراص %d", i),
			NameEN:      fmt.Sprintf("Panadol Extra Tablets %d", i),
			SKU:         fmt.Sprintf("SKU-%d", i),
			Barcode:     fmt.Sprintf("622123456%04d", i),
			PublicPrice: money.MustParse("50.00").String(),
		}
	}

	start := time.Now()
	idx := productmatch.NewIndex(masters)
	t.Logf("NewIndex took: %v", time.Since(start))

	t.Logf("Creating %d rows...", n)
	rows := make([]*productmatch.Row, n)
	for i := 0; i < n; i++ {
		rows[i] = &productmatch.Row{
			Name:    fmt.Sprintf("بانادول اكسترا %d", i),
			NameEN:  fmt.Sprintf("Panadol Extra %d", i),
			SKU:     fmt.Sprintf("PROD-%d", i),
			Barcode: fmt.Sprintf("622123456%04d", i),
		}
	}

	opts := productmatch.DefaultMatchOptions()
	start = time.Now()
	scored := productmatch.MatchAll(idx, rows, opts, 0)
	t.Logf("MatchAll(%d rows) took: %v", n, time.Since(start))
	_ = scored

	recall := productmatch.DefaultRecallOptions()
	recall.Limit = 5
	start = time.Now()
	retrieved := productmatch.RecallAll(idx, rows, recall, 0)
	t.Logf("RecallAll(%d rows) took: %v", n, time.Since(start))
	_ = retrieved
}
