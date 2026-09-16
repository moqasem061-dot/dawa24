package commerce

import (
	"context"
	"errors"
	"testing"
)

// The per-branch purchase quota, at the gate.
//
// These tests pin the two things the feature must never get wrong: a branch
// that has taken its allowance is refused, and a branch that has room is
// offered a ceiling no larger than what the server will actually accept. The
// second is the one that produces a support ticket when it drifts — a number
// box that offers 40 and a checkout that refuses 4 is indistinguishable from
// the site being broken.

// quotaRepo is a repository whose only real behaviour is the quota sum. It
// embeds the shared mock so it satisfies Repository, and records what the rule
// asked for so a test can prove the branch, not just the variant, reached it.
type quotaRepo struct {
	*mockCommerceRepo
	used     map[int64]int // keyed by branch
	err      error
	askedFor []int64 // branches the rule summed, in order
	askedVar []int64 // variants it summed, in order
}

func newQuotaRepo(used map[int64]int) *quotaRepo {
	return &quotaRepo{mockCommerceRepo: newMockCommerceRepo(), used: used}
}

func (r *quotaRepo) BranchQuotaUsed(_ context.Context, variantID, branchID, _ int64) (int, error) {
	r.askedVar = append(r.askedVar, variantID)
	r.askedFor = append(r.askedFor, branchID)
	if r.err != nil {
		return 0, r.err
	}
	return r.used[branchID], nil
}

func (r *quotaRepo) ReleaseBranchQuota(context.Context, int64, int64, int64, int64, string) error {
	return nil
}
func (r *quotaRepo) UndoBranchQuotaRelease(context.Context, int64, int64, int64) error { return nil }
func (r *quotaRepo) ListBranchQuotaRows(context.Context, int64, QuotaFilter) ([]*BranchQuotaRow, int, error) {
	return nil, 0, nil
}
func (r *quotaRepo) ListQuotaVariantRows(context.Context, int64, QuotaFilter) ([]*QuotaVariantRow, int, error) {
	return nil, 0, nil
}
func (r *quotaRepo) QuotaSummaryForVendor(context.Context, int64) (QuotaSummary, error) {
	return QuotaSummary{}, nil
}
func (r *quotaRepo) QuotaVariantOptions(context.Context, int64) ([]QuotaOption, error) {
	return nil, nil
}
func (r *quotaRepo) QuotaCustomerOptions(context.Context, int64) ([]QuotaOption, error) {
	return nil, nil
}
func (r *quotaRepo) QuotaBranchOptions(context.Context, int64) ([]QuotaOption, error) {
	return nil, nil
}

// quotaService wires a healthy availability probe to a repository that knows
// what each branch has already taken.
func quotaService(repo Repository, probe AvailabilityProbe) *Service {
	s := cartService(repo)
	s.SetAvailabilityProbe(probe)
	return s
}

func TestCheckAvailabilityAppliesTheBranchQuota(t *testing.T) {
	cases := []struct {
		name       string
		limit      int
		stock      int
		usedByThis int
		request    int
		wantAllow  bool
		wantReason Reason
		wantMax    int
	}{
		{
			name: "no quota leaves the line alone", limit: 0, stock: 40, request: 8,
			wantAllow: true, wantMax: 40,
		},
		{
			name: "an untouched branch may take up to the cap", limit: 10, stock: 40, request: 10,
			wantAllow: true, wantMax: 10,
		},
		{
			// The ceiling reported on an allowed line is what the quantity box
			// offers. Reporting the stock here is the drift that lets a
			// pharmacy type a number checkout will refuse.
			name:  "the ceiling is the remaining quota, not the stock",
			limit: 10, stock: 40, usedByThis: 7, request: 3,
			wantAllow: true, wantMax: 3,
		},
		{
			// And the other way round: a quota larger than the shelf must not
			// raise the ceiling above what is actually there.
			name:  "the ceiling is still the stock when the stock is smaller",
			limit: 100, stock: 4, request: 4,
			wantAllow: true, wantMax: 4,
		},
		{
			name:  "asking for more than remains is refused with the remainder",
			limit: 10, stock: 40, usedByThis: 7, request: 8,
			wantAllow: false, wantReason: ReasonQuotaExceeded, wantMax: 3,
		},
		{
			name:  "a branch that has taken the whole allowance is finished",
			limit: 10, stock: 40, usedByThis: 10, request: 1,
			wantAllow: false, wantReason: ReasonQuotaExhausted, wantMax: 0,
		},
		{
			// A cap lowered after the branch had already bought more than it.
			// The branch is finished, not owed a negative remainder.
			name:  "consumption beyond a lowered cap does not go negative",
			limit: 5, stock: 40, usedByThis: 9, request: 1,
			wantAllow: false, wantReason: ReasonQuotaExhausted, wantMax: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := healthyProbe()
			probe.variant.StockQty = tc.stock
			probe.variant.QuotaLimit = tc.limit

			repo := newQuotaRepo(map[int64]int{3: tc.usedByThis})
			req := healthyRequest()
			req.Quantity = tc.request

			got, err := quotaService(repo, probe).CheckAvailability(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Allowed != tc.wantAllow {
				t.Fatalf("Allowed = %v (reason %q), want %v", got.Allowed, got.Reason, tc.wantAllow)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if got.MaxQuantity != tc.wantMax {
				t.Errorf("MaxQuantity = %d, want %d", got.MaxQuantity, tc.wantMax)
			}
			if !got.Allowed && got.MessageAr == "" {
				t.Error("a refusal must carry an Arabic message the pharmacy can read")
			}
		})
	}
}

// The whole point of the feature: the quota is charged to a branch, so a second
// branch of the same company starts from its own zero.
func TestBranchQuotaIsPerBranchNotPerCompany(t *testing.T) {
	probe := healthyProbe()
	probe.variant.StockQty = 40
	probe.variant.QuotaLimit = 10

	// Branch 3 has taken everything; branch 4 has taken nothing. Both belong to
	// the same buying organisation.
	repo := newQuotaRepo(map[int64]int{3: 10, 4: 0})
	svc := quotaService(repo, probe)

	exhausted := healthyRequest()
	exhausted.Quantity = 1
	got, err := svc.CheckAvailability(context.Background(), exhausted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Allowed {
		t.Fatal("the branch that used its whole allowance must be refused")
	}

	fresh := healthyRequest()
	fresh.Quantity = 10
	fresh.CustomerBranchID = 4
	probe.branch.ID = 4
	got, err = svc.CheckAvailability(context.Background(), fresh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Allowed {
		t.Fatalf("a sibling branch has its own allowance; got refusal %q", got.Reason)
	}
	if got.MaxQuantity != 10 {
		t.Errorf("MaxQuantity = %d, want the full cap of 10", got.MaxQuantity)
	}

	if len(repo.askedFor) != 2 || repo.askedFor[0] != 3 || repo.askedFor[1] != 4 {
		t.Errorf("the rule must sum per branch; asked for %v", repo.askedFor)
	}
}

// A quota sum that fails is not permission to buy. The stock check has this
// property already and the quota must not be the one place where a database
// error reads as "unlimited".
func TestBranchQuotaErrorSurfacesRatherThanAllowing(t *testing.T) {
	probe := healthyProbe()
	probe.variant.StockQty = 40
	probe.variant.QuotaLimit = 10

	repo := newQuotaRepo(nil)
	repo.err = errors.New("database unavailable")

	got, err := quotaService(repo, probe).CheckAvailability(context.Background(), healthyRequest())
	if err == nil {
		t.Fatalf("expected an error, got result %+v", got)
	}
	if got.Allowed {
		t.Fatal("a failed quota sum must never report the line as allowed")
	}
}

// A repository with no quota support must behave as it did before quotas
// existed, rather than refusing everything. The in-memory repositories the rest
// of these tests use are exactly that case.
func TestQuotaIsInertWithoutABackend(t *testing.T) {
	probe := healthyProbe()
	probe.variant.StockQty = 40
	probe.variant.QuotaLimit = 10

	svc := quotaService(newMockCommerceRepo(), probe)
	got, err := svc.CheckAvailability(context.Background(), healthyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Allowed {
		t.Fatalf("without a quota backend the line must pass; got %q", got.Reason)
	}
}

func TestBranchQuotaUsageArithmetic(t *testing.T) {
	cases := []struct {
		name          string
		usage         BranchQuotaUsage
		wantRemaining int
		wantExhausted bool
		wantPercent   int
	}{
		{"no cap means nothing to spend", BranchQuotaUsage{}, 0, false, 0},
		{"fresh", BranchQuotaUsage{Limit: 10}, 10, false, 0},
		{"partly used", BranchQuotaUsage{Limit: 10, Used: 3}, 7, false, 30},
		{"exactly used up", BranchQuotaUsage{Limit: 10, Used: 10}, 0, true, 100},
		{"over-consumed after a lowered cap", BranchQuotaUsage{Limit: 5, Used: 9}, 0, true, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.usage.Remaining(); got != tc.wantRemaining {
				t.Errorf("Remaining() = %d, want %d", got, tc.wantRemaining)
			}
			if got := tc.usage.Exhausted(); got != tc.wantExhausted {
				t.Errorf("Exhausted() = %v, want %v", got, tc.wantExhausted)
			}
			if got := tc.usage.PercentUsed(); got != tc.wantPercent {
				t.Errorf("PercentUsed() = %d, want %d", got, tc.wantPercent)
			}
		})
	}
}

// Checkout measures a variant's whole demand, not each line separately.
//
// A basket can name one variant twice — the plain listing and an offer both add
// it — and checking 3 and 3 against a remaining allowance of 4 would pass both.
func TestCheckoutMeasuresRepeatedVariantsTogether(t *testing.T) {
	probe := healthyProbe()
	probe.variant.StockQty = 40
	probe.variant.QuotaLimit = 4

	repo := newQuotaRepo(map[int64]int{3: 0})
	svc := quotaService(repo, probe)

	variantID := int64(10)
	branchID := int64(3)
	err := svc.revalidateCheckoutLines(context.Background(), CheckoutInput{
		CustomerOrgID: 99,
		BranchID:      &branchID,
		Items: []CheckoutLineItem{
			{VendorOrgID: 7, ProductVariantID: &variantID, Quantity: 3},
			{VendorOrgID: 7, ProductVariantID: &variantID, Quantity: 3},
		},
	})
	if err == nil {
		t.Fatal("two lines of 3 against a quota of 4 must be refused, not passed line by line")
	}
	if len(repo.askedVar) != 1 {
		t.Errorf("the variant should be summed once for the whole order, asked %d times", len(repo.askedVar))
	}
}
