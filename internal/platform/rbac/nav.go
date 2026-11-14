package rbac

// Navigation is declared here, beside the permissions, for one reason: a
// sidebar link and the gate on the page it points at have to agree, and while
// they lived in two files written by hand they did not. The admin sidebar hid
// six links behind keys no role could hold; the vendor and pharmacy sidebars
// showed every link to everyone, including links to pages the member had no
// business opening.
//
// Now a sidebar item names one permission, the shells render whatever this
// registry says is visible, and a test asserts that every named permission is
// declared in the same scope. A link cannot exist without a permission, and a
// permission that reveals a page cannot be missing from the catalogue.

// NavItem is one sidebar link.
type NavItem struct {
	// Key matches the activeNav value handlers pass to the shell.
	Key string
	// Aliases are further activeNav values that should highlight this item —
	// a detail page under the same section, normally.
	Aliases []string
	Href    string
	// Icon names a components.Icon case. An unknown name renders nothing
	// rather than failing, so a typo costs a glyph, not a page.
	Icon   string
	NameAr string
	NameEn string
	// Perm is the permission that reveals this item. Every item has one.
	Perm string
	// Also lists further permissions that alone are enough to reveal the item,
	// for a page two different roles reach for different reasons.
	Also []string
	// AlwaysVisible marks an item every member of the scope may reach, whatever
	// they hold.
	//
	// There is exactly one kind of item this is for: a page about the caller
	// rather than about the company. Account settings is the case — a member
	// locked out of every company screen must still be able to change their own
	// password, and inventing a permission for that would only create a role
	// that can be configured to forbid it.
	AlwaysVisible bool
}

// Active reports whether activeNav refers to this item.
func (n NavItem) Active(activeNav string) bool {
	if activeNav == n.Key {
		return true
	}
	for _, a := range n.Aliases {
		if a == activeNav {
			return true
		}
	}
	return false
}

// Visible reports whether the holding reveals this item.
func (n NavItem) Visible(s Set) bool {
	if n.AlwaysVisible {
		return true
	}
	if s.Has(n.Perm) {
		return true
	}
	return s.HasAny(n.Also...) && len(n.Also) > 0
}

// NavSection is a labelled group of sidebar links.
type NavSection struct {
	Key    string
	NameAr string
	NameEn string
	Items  []NavItem
}

// Nav returns the full sidebar declaration for a dashboard, unfiltered.
func Nav(scope Scope) []NavSection {
	switch scope {
	case ScopeAdmin:
		return adminNav()
	case ScopeVendor:
		return vendorNav()
	case ScopePharmacy:
		return pharmacyNav()
	}
	return nil
}

// NavSuppressions lists sidebar entry keys this deployment has switched off —
// "integrations" when the ERP API is disabled, for example. The composition
// root writes it once at startup, before serving. It is a deployment switch,
// not an authorization control: the gate on the route stays the control, and
// hiding a link never changes what a request may do.
var NavSuppressions = map[string]bool{}

// NavSuppressedFunc is the runtime counterpart of NavSuppressions: the
// composition root sets it once, and it is consulted on every render, so an
// entry can disappear the moment an admin flips a switch, with no restart. It
// applies to the vendor and pharmacy sidebars only; the staff sidebar keeps its
// entries (the admin integrations page stays reachable while the integration is
// off). Like NavSuppressions it hides a link and nothing else.
var NavSuppressedFunc func(key string) bool

func navSuppressed(scope Scope, key string) bool {
	if NavSuppressions[key] {
		return true
	}
	return scope != ScopeAdmin && NavSuppressedFunc != nil && NavSuppressedFunc(key)
}

// VisibleNav returns the sidebar a holder actually sees: items they hold a
// permission for, and only the sections that still have items.
//
// This is the whole of "sidebar items show or hide based on the assigned
// permissions". It is a convenience, not a control — the gate on the route is
// the control, and it is applied independently. Entries in NavSuppressions are
// hidden whatever the caller holds, because the page they point at does not
// exist on this deployment.
func VisibleNav(scope Scope, held Set) []NavSection {
	src := Nav(scope)
	out := make([]NavSection, 0, len(src))
	for _, sec := range src {
		if navSuppressed(scope, sec.Key) {
			continue
		}
		items := make([]NavItem, 0, len(sec.Items))
		for _, it := range sec.Items {
			if navSuppressed(scope, it.Key) {
				continue
			}
			if it.Visible(held) {
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			sec.Items = items
			out = append(out, sec)
		}
	}
	return out
}

// NavLabel picks the language for a section or item label.
func NavLabel(lang, ar, en string) string {
	if lang == "en" && en != "" {
		return en
	}
	if ar != "" {
		return ar
	}
	return en
}

// Label returns the group's label in the requested language.
func (g Group) Label(lang string) string { return NavLabel(lang, g.NameAr, g.NameEn) }

// Label returns the sidebar item's label in the requested language.
func (n NavItem) Label(lang string) string { return NavLabel(lang, n.NameAr, n.NameEn) }

// Label returns the sidebar section's label in the requested language.
func (s NavSection) Label(lang string) string { return NavLabel(lang, s.NameAr, s.NameEn) }
