// Package pagecontrol enables or disables system pages at the route level.
//
// A disabled page is refused by Guard, an outermost HTTP handler that wraps the
// whole router and answers with the same 404 an unknown route gets — before
// authentication, for every caller, on both the HTML and the JSON surface. The
// admin screen at /admin/system-pages edits the catalogue; SyncDiscovered fills
// it from the live chi route table at boot so no route has to be typed in by
// hand.
//
// The package is deliberately shaped like internal/platform/features: an
// in-memory engine with a background reload, a process-global accessor, and a
// store that is the source of truth. It differs in what the key is — a route
// path, matched exact or by prefix, rather than a feature name.
package pagecontrol

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MatchMode decides how a rule's path is compared to a request path.
type MatchMode string

const (
	// MatchExact blocks only the one path.
	MatchExact MatchMode = "exact"
	// MatchPrefix blocks the path and everything nested under it.
	MatchPrefix MatchMode = "prefix"
)

// ValidMatchMode reports whether m is a known mode.
func ValidMatchMode(m MatchMode) bool { return m == MatchExact || m == MatchPrefix }

// Resource names the dashboard a page belongs to. It drives the admin screen's
// grouping and is inferred from the path prefix; it is not a security boundary.
type Resource string

const (
	ResourceAdmin       Resource = "admin"
	ResourceVendor      Resource = "vendor"
	ResourceClient      Resource = "client"
	ResourceIndependent Resource = "independent"
)

// ValidResource reports whether r is a known resource.
func ValidResource(r Resource) bool {
	switch r {
	case ResourceAdmin, ResourceVendor, ResourceClient, ResourceIndependent:
		return true
	}
	return false
}

// Source records whether a row was discovered from the route table or typed by
// an operator. Only manual rows may be deleted.
type Source string

const (
	SourceDiscovered Source = "discovered"
	SourceManual     Source = "manual"
)

// Page is one row of platform_admin.managed_pages.
type Page struct {
	ID            int64
	Resource      Resource
	Path          string
	MatchMode     MatchMode
	LabelAr       string
	LabelEn       string
	Description   string
	IsEnabled     bool
	IsSystem      bool
	IsLockable    bool
	RoutePatterns []string
	Source        Source
	DiscoveredAt  *time.Time
	UpdatedBy     *int64
	UpdatedAt     time.Time
	CreatedAt     time.Time
}

// Label picks the operator-facing name for a language, falling back to the path.
func (p Page) Label(lang string) string {
	if lang == "en" && p.LabelEn != "" {
		return p.LabelEn
	}
	if p.LabelAr != "" {
		return p.LabelAr
	}
	if p.LabelEn != "" {
		return p.LabelEn
	}
	return p.Path
}

// rule is the matcher's compact view of a Page: everything Decision needs and
// nothing it does not.
type rule struct {
	id          int64
	path        string
	mode        MatchMode
	enabled     bool
	labelAr     string
	labelEn     string
	description string
}

// protectedPrefixes are paths Guard serves no matter what a row says, and that
// the store refuses to disable. Two layers, on purpose: the seeded rows carry
// is_lockable = false, and this list means even a row that loses that flag by
// any means cannot lock an operator out of the control panel, the dashboard,
// the role editor, sign-in, or the health probes.
var protectedPrefixes = []string{
	"/health",
	"/ready",
	"/api/v1/status",
	"/static/",
	"/uploads/",
	"/auth/",
	"/lang/",
	"/admin/system-pages",
	"/admin/dashboard",
	"/admin/roles",
	"/onboarding/pending",
}

// IsProtected reports whether p is a path the guard must always serve. p is
// expected to be normalized already.
func IsProtected(p string) bool {
	for _, pre := range protectedPrefixes {
		if p == pre || strings.HasPrefix(p, strings.TrimSuffix(pre, "/")+"/") || p == strings.TrimSuffix(pre, "/") {
			return true
		}
	}
	return false
}

var pathShape = regexp.MustCompile(`^/[^?#\s]*$`)

// NormalizePath folds a request or a stored path onto one canonical form so the
// two can be compared: a leading slash, collapsed repeats, no query or fragment,
// and no trailing slash except for the root.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		return "/"
	}
	return p
}

// ClassifyResource infers the dashboard a path belongs to from its prefix.
func ClassifyResource(path string) Resource {
	switch {
	case path == "/admin" || strings.HasPrefix(path, "/admin/"):
		return ResourceAdmin
	case path == "/vendor" || strings.HasPrefix(path, "/vendor/"):
		return ResourceVendor
	case path == "/customer" || strings.HasPrefix(path, "/customer/"):
		return ResourceClient
	default:
		return ResourceIndependent
	}
}

// ValidatePath rejects a stored path that the matcher could not use or that a
// CHECK constraint would refuse.
func ValidatePath(p string) error {
	if p == "" {
		return fmt.Errorf("path is required")
	}
	if !pathShape.MatchString(p) {
		return fmt.Errorf("path must start with %q and contain no spaces, query or fragment", "/")
	}
	if len(p) > 512 {
		return fmt.Errorf("path is longer than 512 characters")
	}
	return nil
}

// ValidatePrefixRule guards the one prefix that would take the whole site down.
func ValidatePrefixRule(p string, mode MatchMode) error {
	if mode == MatchPrefix && p == "/" {
		return fmt.Errorf("a prefix rule on %q would disable every page", "/")
	}
	return nil
}
