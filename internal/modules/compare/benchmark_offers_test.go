package compare

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The modal must list exactly as many offers as the badge the reader clicked.
// They are computed by two different code paths over the same data; if those
// paths drift, a badge saying 3 opens a list of 4 and the screen is lying.
func TestBucketPartitionMatchesTheCounts(t *testing.T) {
	yourNet := money.FromMinor(10000) // 100.00

	offers := []MarketOffer{
		{SupplierName: "أرخص أ", NetPrice: money.FromMinor(8000)},
		{SupplierName: "أرخص ب", NetPrice: money.FromMinor(9000)},
		{SupplierName: "مماثل أ", NetPrice: money.FromMinor(10000)},
		{SupplierName: "أغلى أ", NetPrice: money.FromMinor(12000)},
		{SupplierName: "أغلى ب", NetPrice: money.FromMinor(15000)},
		{SupplierName: "أغلى ج", NetPrice: money.FromMinor(20000)},
		// A duplicate upload from a supplier already counted: one vote each.
		{SupplierName: "أرخص أ", NetPrice: money.FromMinor(7000)},
	}

	tolerance := int64(float64(yourNet.Minor()) * equalTolerancePercent / 100)
	counts := map[BenchmarkBucket]int{}
	seen := map[string]bool{}
	for _, o := range offers {
		if seen[o.SupplierName] {
			continue
		}
		seen[o.SupplierName] = true
		switch d := o.NetPrice.Minor() - yourNet.Minor(); {
		case d < -tolerance:
			counts[BucketBetter]++
		case d > tolerance:
			counts[BucketWorse]++
		default:
			counts[BucketEqual]++
		}
	}

	if counts[BucketBetter] != 2 {
		t.Errorf("cheaper offers counted %d, want 2", counts[BucketBetter])
	}
	if counts[BucketEqual] != 1 {
		t.Errorf("equal offers counted %d, want 1", counts[BucketEqual])
	}
	if counts[BucketWorse] != 3 {
		t.Errorf("dearer offers counted %d, want 3", counts[BucketWorse])
	}

	// Every offer lands in exactly one bucket: none lost, none double-counted.
	if total := counts[BucketBetter] + counts[BucketEqual] + counts[BucketWorse]; total != len(seen) {
		t.Errorf("buckets hold %d offers but %d distinct suppliers were seen", total, len(seen))
	}
}

// A bucket name arriving from a query string must be one of the three columns.
func TestValidBenchmarkBucket(t *testing.T) {
	for _, ok := range []string{"better", "equal", "worse"} {
		if !ValidBenchmarkBucket(ok) {
			t.Errorf("%q should be a valid bucket", ok)
		}
	}
	for _, bad := range []string{"", "all", "BETTER", "'; DROP TABLE", "../../etc"} {
		if ValidBenchmarkBucket(bad) {
			t.Errorf("%q was accepted as a bucket", bad)
		}
	}
}
