// Package rbac is the single source of truth for what may be controlled by a
// permission, and for how a held permission is matched against a required one.
//
// Before this package the answer lived in three places that could not agree:
// hardcoded strings in the sidebar templates, hardcoded strings in the route
// registrars, and rows in identity.permissions seeded by a migration. They did
// drift — the admin sidebar gated six items on keys such as
// "notifications.notification.view" and "workflow.coverage.view" that no row
// ever existed for, so those links were invisible to every account except
// super_admin, and no role could ever be granted them.
//
// The catalogue declared here is the definition. The database table is a
// mirror, synced from this package at boot (see Sync), so a permission cannot
// exist in a role editor without a Go declaration, and a route cannot be gated
// on a key nobody can be granted.
package rbac

import (
	"sort"
	"strings"
)

// Scope names a dashboard. A permission is offered in a role editor only for
// the scope that editor belongs to: a vendor owner building a role for their
// warehouse clerk must not be shown platform settings, and a super admin
// building a moderator role must not be shown a vendor's storefront.
type Scope string

const (
	// ScopeAdmin covers /admin/* — the platform staff dashboard.
	ScopeAdmin Scope = "admin"
	// ScopeVendor covers /vendor/* — one supplier company's dashboard.
	ScopeVendor Scope = "vendor"
	// ScopePharmacy covers /customer/* — one pharmacy company's dashboard.
	ScopePharmacy Scope = "pharmacy"
)

// Scopes is every dashboard the system knows about.
func Scopes() []Scope { return []Scope{ScopeAdmin, ScopeVendor, ScopePharmacy} }

// ValidScope reports whether s names a dashboard.
func ValidScope(s Scope) bool {
	switch s {
	case ScopeAdmin, ScopeVendor, ScopePharmacy:
		return true
	}
	return false
}

// Canonical organization types. The database holds legacy spellings from the
// Laravel schema — "supplier", "company", "agency", "pharmacy",
// "chain_pharmacy", "individual" — and every one of them still appears in
// live rows. Two names for one thing is how the vendor roles page came to
// render inside the pharmacy shell with an empty sidebar: the shell compared
// `OrgType == "vendor"` while the row said "supplier", so a supplier fell
// through to the pharmacy branch and then saw none of its links, because a
// supplier holds no pharmacy permissions.
const (
	OrgTypeVendor   = "vendor"
	OrgTypePharmacy = "customer"
)

// NormalizeOrgType folds a legacy organization type onto its canonical name.
// Anything unrecognised is returned unchanged, and TenantScopeFor will then
// refuse it rather than guess.
func NormalizeOrgType(orgType string) string {
	switch orgType {
	case "vendor", "supplier", "company", "agency":
		return OrgTypeVendor
	case "customer", "pharmacy", "chain_pharmacy", "individual":
		return OrgTypePharmacy
	}
	return orgType
}

// TenantScopeFor maps an organization type to the dashboard its members use.
// It returns false for an organization whose type has no dashboard.
//
// This is the single place that answers "which dashboard is this company on".
// Sidebars, shells, role editors and route gates all route through it, so a
// new legacy spelling is one line here rather than a hunt for string equality
// checks that quietly disagree.
func TenantScopeFor(orgType string) (Scope, bool) {
	switch NormalizeOrgType(orgType) {
	case OrgTypeVendor:
		return ScopeVendor, true
	case OrgTypePharmacy:
		return ScopePharmacy, true
	}
	return "", false
}

// TenantOrgTypes lists every legacy spelling of organization type that
// resolves to a dashboard.
//
// It is the inverse of TenantScopeFor, and it exists so a query that has to
// select companies by dashboard can do so in SQL without a second, drifting
// copy of the spelling list. TestTenantOrgTypesMatchTenantScopeFor holds the
// two to each other.
func TenantOrgTypes(scope Scope) []string {
	all := []string{
		"vendor", "supplier", "company", "agency",
		"customer", "pharmacy", "chain_pharmacy", "individual",
	}
	out := make([]string, 0, len(all))
	for _, t := range all {
		if s, ok := TenantScopeFor(t); ok && s == scope {
			out = append(out, t)
		}
	}
	return out
}

// Kind separates the two things a permission can control, because the role
// editor renders them differently: a page is a row in the matrix, an action is
// a checkbox within that row.
type Kind string

const (
	// KindPage grants sight of a page or section — it drives the sidebar.
	KindPage Kind = "page"
	// KindAction grants a mutation or a privileged read within a page.
	KindAction Kind = "action"
)

// Permission is one controllable page, section, feature or action.
type Permission struct {
	// Key is the wire identifier, "module.resource.action". It is what routes
	// are gated on, what role_permissions stores, and what a session carries.
	Key string
	// Group is the Group.Key this permission is filed under in the matrix.
	Group string
	// Kind decides whether this is a page gate or an action within one.
	Kind Kind
	// NameAr and NameEn label the checkbox. Arabic is primary (AGENTS.md).
	NameAr string
	NameEn string
	// Scopes lists the dashboards that may grant this permission. A key with
	// more than one scope is the same capability on two dashboards, held
	// against different data by tenancy.
	Scopes []Scope
	// Implies lists permissions granted automatically alongside this one. A
	// page's action normally implies the page itself, so granting "edit" can
	// never produce a role that may edit a page it cannot open.
	Implies []string
	// Nav, when set, is the sidebar item key this permission reveals. It is
	// what makes "every sidebar item is controlled by a permission" checkable
	// rather than aspirational.
	Nav string
}

// InScope reports whether p may be granted on the named dashboard.
func (p Permission) InScope(s Scope) bool {
	for _, x := range p.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

// Group is a section of the permission matrix — normally one sidebar group.
type Group struct {
	Key    string
	NameAr string
	NameEn string
	Scopes []Scope
	Order  int
}

// InScope reports whether g appears in the named dashboard's matrix.
func (g Group) InScope(s Scope) bool {
	for _, x := range g.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

// Wildcard grants every permission. Only the platform owner role holds it.
const Wildcard = "*"

// Match reports whether holding `held` satisfies a requirement for `want`.
//
// Matching is hierarchical on the dot-separated segments, so "catalog.*"
// satisfies "catalog.product.view" and "*" satisfies everything. Without this
// a module-wide grant would have to enumerate every key it covers, and would
// silently stop covering the next key added to that module.
func Match(held, want string) bool {
	if held == want {
		return true
	}
	if held == Wildcard {
		return true
	}
	if !strings.HasSuffix(held, ".*") {
		return false
	}
	prefix := strings.TrimSuffix(held, "*") // keeps the trailing dot
	return strings.HasPrefix(want, prefix)
}

// Set is a resolved permission holding, ready to answer questions quickly.
//
// It keeps exact keys in a map and wildcard grants in a slice, because a
// wildcard cannot be answered by a hash lookup and the slice is never long —
// a role holds at most one per module.
type Set struct {
	exact     map[string]struct{}
	wildcards []string
}

// NewSet builds a holding from the keys a role or a session carries.
func NewSet(keys []string) Set {
	s := Set{exact: make(map[string]struct{}, len(keys))}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if k == Wildcard || strings.HasSuffix(k, ".*") {
			s.wildcards = append(s.wildcards, k)
			continue
		}
		s.exact[k] = struct{}{}
	}
	return s
}

// Has reports whether the holding satisfies want.
func (s Set) Has(want string) bool {
	if want == "" {
		return true
	}
	if _, ok := s.exact[want]; ok {
		return true
	}
	for _, w := range s.wildcards {
		if Match(w, want) {
			return true
		}
	}
	return false
}

// HasAny reports whether the holding satisfies at least one of wants. An empty
// list is an ungated requirement and passes.
func (s Set) HasAny(wants ...string) bool {
	if len(wants) == 0 {
		return true
	}
	for _, w := range wants {
		if s.Has(w) {
			return true
		}
	}
	return false
}

// HasAll reports whether the holding satisfies every one of wants.
func (s Set) HasAll(wants ...string) bool {
	for _, w := range wants {
		if !s.Has(w) {
			return false
		}
	}
	return true
}

// Keys returns the holding as a sorted slice, for storage and for tests.
func (s Set) Keys() []string {
	out := make([]string, 0, len(s.exact)+len(s.wildcards))
	for k := range s.exact {
		out = append(out, k)
	}
	out = append(out, s.wildcards...)
	sort.Strings(out)
	return out
}

// Len reports how many distinct grants the holding contains.
func (s Set) Len() int { return len(s.exact) + len(s.wildcards) }
