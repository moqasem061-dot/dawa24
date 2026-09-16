package pagecontrol

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

var (
	routerMu    sync.RWMutex
	knownRouter chi.Routes
)

// SetRouter records the mounted router so the admin screen's "rescan" button
// can re-run discovery without a restart. Routes do not change at runtime, so
// this is captured once at boot.
func SetRouter(r chi.Routes) {
	routerMu.Lock()
	knownRouter = r
	routerMu.Unlock()
}

// Rescan re-runs discovery against the router SetRouter recorded and folds the
// result into the store, then refreshes the live engine.
func Rescan(ctx context.Context) (int, error) {
	routerMu.RLock()
	r := knownRouter
	routerMu.RUnlock()
	e := Global()
	if r == nil || e == nil {
		return 0, fmt.Errorf("pagecontrol: engine or router not initialised")
	}
	added, err := e.store.UpsertDiscovered(ctx, DiscoverRoutes(r))
	_ = e.Reload(ctx)
	return added, err
}

// Candidate is a route root the discovery pass found: a static path prefix and
// the concrete chi patterns that live under it.
type Candidate struct {
	Path     string
	Resource Resource
	Patterns []string
	LabelAr  string
	LabelEn  string
}

// DiscoverRoutes walks a chi router and groups its patterns by their static
// prefix — the part before the first path parameter or wildcard. One managed
// row per group means an operator disables "/admin/users" once and covers the
// list, the detail page and every action under it, without ever typing "{id}".
func DiscoverRoutes(routes chi.Routes) []Candidate {
	if routes == nil {
		return nil
	}
	groups := map[string]map[string]struct{}{}
	_ = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		prefix := staticPrefix(route)
		if prefix == "" || prefix == "/" {
			return nil
		}
		if groups[prefix] == nil {
			groups[prefix] = map[string]struct{}{}
		}
		groups[prefix][NormalizePath(route)] = struct{}{}
		return nil
	})

	out := make([]Candidate, 0, len(groups))
	for prefix, patternSet := range groups {
		patterns := make([]string, 0, len(patternSet))
		for p := range patternSet {
			patterns = append(patterns, p)
		}
		sort.Strings(patterns)
		out = append(out, Candidate{
			Path:     prefix,
			Resource: ClassifyResource(prefix),
			Patterns: patterns,
			LabelAr:  prefix,
			LabelEn:  prefix,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// SyncDiscovered runs the discovery pass and folds it into the store, then
// refreshes the live engine so the new rows take effect without a restart.
func SyncDiscovered(ctx context.Context, db *database.DB, routes chi.Routes) (int, error) {
	added, err := NewStore(db).UpsertDiscovered(ctx, DiscoverRoutes(routes))
	if e := Global(); e != nil {
		_ = e.Reload(ctx)
	}
	return added, err
}

// staticPrefix returns the leading run of literal path segments in a chi
// pattern, stopping at the first segment that holds a "{param}" or "*".
func staticPrefix(route string) string {
	route = NormalizePath(route)
	if route == "/" {
		return "/"
	}
	segments := strings.Split(strings.TrimPrefix(route, "/"), "/")
	var kept []string
	for _, seg := range segments {
		if seg == "" || strings.ContainsAny(seg, "{*") {
			break
		}
		kept = append(kept, seg)
	}
	if len(kept) == 0 {
		return "/"
	}
	return "/" + strings.Join(kept, "/")
}
