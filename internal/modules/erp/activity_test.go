package erp

import (
	"context"
	"testing"
	"time"
)

// activityFixture is a service over the in-memory fake, with the activity fake
// holding the rows the screen reads.
func activityFixture(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	return NewService(repo, Ports{}, Config{Provider: "testprovider"}, nil), repo
}

func TestListCallsClampsLimitAndNormalisesResult(t *testing.T) {
	svc, repo := activityFixture(t)
	now := time.Now()
	for id := int64(1); id <= 300; id++ {
		repo.actCalls = append(repo.actCalls, fakeCall{org: 7, row: CallRow{ID: id, Status: 200, CreatedAt: now}})
	}
	cases := []struct {
		name      string
		in        CallFilter
		wantLimit int
		wantRes   string
	}{
		{"zero limit uses the default", CallFilter{}, CallsDefaultLimit, "all"},
		{"negative limit uses the default", CallFilter{Limit: -4}, CallsDefaultLimit, "all"},
		{"large limit is capped", CallFilter{Limit: 5000}, CallsMaxLimit, "all"},
		{"ok passes through", CallFilter{Result: "ok", Limit: 10}, 10, "ok"},
		{"error passes through", CallFilter{Result: "error", Limit: 10}, 10, "error"},
		{"unknown result reads as all", CallFilter{Result: "bogus", Limit: 10}, 10, "all"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.ListCalls(context.Background(), 7, c.in); err != nil {
				t.Fatalf("ListCalls: %v", err)
			}
			if repo.actLastFilter.Limit != c.wantLimit || repo.actLastFilter.Result != c.wantRes {
				t.Fatalf("repository got limit %d result %q, want %d %q", repo.actLastFilter.Limit, repo.actLastFilter.Result, c.wantLimit, c.wantRes)
			}
		})
	}
}

func TestListCallsIsTenantScoped(t *testing.T) {
	svc, repo := activityFixture(t)
	now := time.Now()
	repo.actCalls = []fakeCall{
		{org: 7, row: CallRow{ID: 1, CreatedAt: now}},
		{org: 8, row: CallRow{ID: 2, CreatedAt: now}},
	}
	rows, err := svc.ListCalls(context.Background(), 8, CallFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListCalls: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 2 {
		t.Fatalf("organization 8 read %+v, want only call 2", rows)
	}
	none, err := svc.ListCalls(context.Background(), 99, CallFilter{Limit: 10})
	if err != nil || len(none) != 0 {
		t.Fatalf("unknown organization read %+v (err %v), want nothing", none, err)
	}
}

func TestListBatchesClampsLimit(t *testing.T) {
	svc, _ := activityFixture(t)
	if _, err := svc.ListBatches(context.Background(), 7, -1, 0); err != nil {
		t.Fatalf("ListBatches: %v", err)
	}
	if _, err := svc.ListBatches(context.Background(), 7, 0, 10000); err != nil {
		t.Fatalf("ListBatches large: %v", err)
	}
}

func TestActivityCountCapStopsAtCap(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1, ActivityImportCap: ActivityImportCap, ActivityImportCap + 1: ActivityImportCap}
	for in, want := range cases {
		if got := capCount(in); got != want {
			t.Errorf("capCount(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestActivitySummaryWithoutIntegrationReadsNoOrders(t *testing.T) {
	svc, repo := activityFixture(t)
	repo.actCounts = ActivityCounts{IntegrationStatus: "", OpenReview: 2}
	summary, err := svc.ActivitySummary(context.Background(), 7)
	if err != nil {
		t.Fatalf("ActivitySummary: %v", err)
	}
	if summary.OpenReview != 2 || summary.SalesNew != 0 || summary.PurchaseNew != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if repo.actCountsOrg != 7 {
		t.Fatalf("counts read for organization %d, want 7", repo.actCountsOrg)
	}
}
