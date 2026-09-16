package ui

import (
	"context"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// The admin dashboard's numbers, computed off the request path.
//
// This exists because of a production failure with a specific shape: signing in
// as an administrator left the browser loading until the hosting proxy gave up
// and answered 502, and while that was happening the rest of the platform
// crawled.
//
// The cause was not one slow query. Administrators land on /admin/dashboard,
// and that handler ran roughly fifteen service calls in sequence, each opening
// its own transaction — every one of which takes a connection from a pool of
// twenty and costs an extra round trip for the tenant GUC before it does any
// work. Several of those calls are unbounded aggregates over the largest tables
// on the platform: every visitor row, every product, every order, every saving
// product. Summed on one request, on a hosted database with real data, that
// exceeds any reasonable proxy timeout. And because the work is spent on
// connections and database CPU rather than on anything local, the cost lands on
// every other request too — which is why one administrator signing in was
// visible to everybody.
//
// Three properties fix it, and all three are needed:
//
//   - The numbers are CACHED. A statistics overview does not need to be
//     accurate to the second, so the database sees one computation per TTL no
//     matter how many administrators are looking or how often they refresh.
//   - The computation is SINGLE-FLIGHT and runs on a detached context. Two
//     administrators, or one administrator hammering reload, cannot stack
//     fifteen more transactions on top of the fifteen already running — which
//     is the mechanism that turned a slow page into a platform-wide stall.
//   - A request NEVER waits on it indefinitely. A stale snapshot is served
//     immediately while the refresh runs behind it, and the very first load
//     after a restart waits only a short budget before rendering with whatever
//     is ready. A dashboard showing slightly old numbers is a working
//     dashboard; a 502 is not.
//
// Money is kept here in minor units rather than formatted, because the strings
// the page displays are localised per request and a cached snapshot is shared
// across languages.

// dashboardSnapshot is one computed view of the platform's headline numbers.
type dashboardSnapshot struct {
	totalUsers         int
	totalOrganizations int
	totalPharmacies    int
	totalVendors       int
	totalBranches      int
	pendingApprovals   int

	pendingDepositsCount int
	pendingDepositsMinor int64
	heldInWalletsMinor   int64

	totalOrders          int
	activeOrders         int
	completedOrders      int
	totalProducts        int
	totalSavingProducts  int
	totalGMV             string
	hasGMV               bool
	totalVisitors        int
	todayVisitors        int
	topDevices           map[string]int
	topBrowsers          map[string]int
	topLocations         map[string]int
	recentOrders         []*commerce.Order
	recentOrganizations  []*org.Organization
	pendingOrganizations []*org.Organization
	gatewayOnline        bool

	computedAt time.Time
}

// Variables rather than constants so the tests can shrink them; nothing else
// writes to them.
var (
	// dashboardTTL is how long a snapshot is served before a refresh is
	// started. Headline counts on a pharmacy marketplace do not move
	// meaningfully inside two minutes, and the staleness buys a hard ceiling on
	// how often the database is asked for them.
	dashboardTTL = 120 * time.Second

	// dashboardComputeBudget bounds the background computation. Past this the
	// attempt is abandoned rather than left holding connections. It is
	// deliberately longer than the 30s HTTP write timeout, because this work no
	// longer happens on a connection anybody is waiting on.
	dashboardComputeBudget = 45 * time.Second

	// dashboardColdWait is how long the FIRST request after a restart is
	// willing to wait, when there is nothing cached to fall back on. Every
	// later request is served from cache and waits for nothing.
	dashboardColdWait = 4 * time.Second
)

// dashboardCache holds the latest snapshot and at most one refresh in flight.
type dashboardCache struct {
	mu   sync.Mutex
	snap *dashboardSnapshot
	// lastAttempt gates how often a refresh may START, which is not the same
	// as how old the last GOOD snapshot is. Gating on the snapshot alone means
	// a computation that keeps failing — the exact case where the database is
	// already struggling — is retried by every single request, which is how a
	// slow dashboard becomes a self-inflicted denial of service.
	lastAttempt time.Time
	inflight    chan struct{}
}

var adminDashboard = &dashboardCache{}

// get returns the snapshot to render, refreshing in the background when stale.
//
// It never blocks when something is cached, and never blocks for longer than
// dashboardColdWait when nothing is.
func (c *dashboardCache) get(
	ctx context.Context, compute func(context.Context) *dashboardSnapshot,
) *dashboardSnapshot {
	c.mu.Lock()
	if c.snap != nil && time.Since(c.snap.computedAt) < dashboardTTL {
		snap := c.snap
		c.mu.Unlock()
		return snap
	}

	done := c.inflight
	if done == nil && time.Since(c.lastAttempt) >= dashboardTTL {
		done = make(chan struct{})
		c.inflight = done
		c.lastAttempt = time.Now()

		// context.WithoutCancel keeps the caller's values — the actor and the
		// tenant GUC the system context is derived from — while dropping their
		// cancellation, so an administrator who navigates away does not kill a
		// refresh that other requests are about to be served from.
		refreshCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), dashboardComputeBudget)

		go func() {
			defer cancel()
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					c.mu.Lock()
					c.inflight = nil
					c.mu.Unlock()
				}
			}()
			snap := compute(refreshCtx)
			c.mu.Lock()
			if snap != nil {
				snap.computedAt = time.Now()
				c.snap = snap
			}
			c.inflight = nil
			c.mu.Unlock()
		}()
	}

	stale := c.snap
	c.mu.Unlock()

	// Stale-while-revalidate: an old number now beats a correct one after the
	// proxy has already given up.
	if stale != nil {
		return stale
	}
	// Nothing cached and no refresh running — a recent attempt failed and its
	// backoff has not elapsed. Render empty rather than start another.
	if done == nil {
		return nil
	}

	select {
	case <-done:
	case <-time.After(dashboardColdWait):
	case <-ctx.Done():
	}

	c.mu.Lock()
	snap := c.snap
	c.mu.Unlock()
	return snap
}

// runParallel executes the independent query groups concurrently.
//
// They were sequential, so the page cost the SUM of every group's latency. They
// touch different tables and share nothing, so the cost is now the slowest one.
// The count is small and deliberate: this runs once per TTL, and a pool of
// twenty connections absorbs a handful of concurrent readers without becoming
// the next problem.
func runParallel(groups ...func()) {
	var wg sync.WaitGroup
	wg.Add(len(groups))
	for _, g := range groups {
		go func(fn func()) {
			defer wg.Done()
			fn()
		}(g)
	}
	wg.Wait()
}
