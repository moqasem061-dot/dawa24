package rbac

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Catalog is the assembled permission and group registry.
type Catalog struct {
	perms   []Permission
	groups  []Group
	byKey   map[string]Permission
	byGroup map[string]Group
	// closure caches the transitive Implies expansion of each key so a grant
	// resolves in one lookup instead of walking the graph per request.
	closure map[string][]string
}

var (
	once   sync.Once
	global *Catalog
)

// Default returns the process-wide catalogue, built on first use.
//
// It panics on a malformed declaration — a duplicate key, an unknown group, an
// implication pointing at a permission that does not exist, or a scope
// mismatch between a permission and its group. Those are programming errors in
// this package's own source, and a process that starts with a broken
// permission catalogue is a process that silently denies or silently grants.
func Default() *Catalog {
	once.Do(func() {
		c, err := Build(allGroups(), allPermissions())
		if err != nil {
			panic("rbac: " + err.Error())
		}
		global = c
	})
	return global
}

// Build validates and assembles a catalogue. Exported for tests that want to
// check a subset without disturbing the global one.
func Build(groups []Group, perms []Permission) (*Catalog, error) {
	c := &Catalog{
		perms:   make([]Permission, 0, len(perms)),
		groups:  make([]Group, 0, len(groups)),
		byKey:   make(map[string]Permission, len(perms)),
		byGroup: make(map[string]Group, len(groups)),
	}

	for _, g := range groups {
		if _, dup := c.byGroup[g.Key]; dup {
			return nil, fmt.Errorf("duplicate group %q", g.Key)
		}
		for _, s := range g.Scopes {
			if !ValidScope(s) {
				return nil, fmt.Errorf("group %q declares unknown scope %q", g.Key, s)
			}
		}
		c.byGroup[g.Key] = g
		c.groups = append(c.groups, g)
	}
	sort.SliceStable(c.groups, func(i, j int) bool { return c.groups[i].Order < c.groups[j].Order })

	for _, p := range perms {
		if _, dup := c.byKey[p.Key]; dup {
			return nil, fmt.Errorf("duplicate permission %q", p.Key)
		}
		if strings.TrimSpace(p.Key) == "" {
			return nil, fmt.Errorf("permission with empty key in group %q", p.Group)
		}
		if strings.Contains(p.Key, "*") {
			return nil, fmt.Errorf("permission %q may not contain a wildcard; wildcards are grants, not declarations", p.Key)
		}
		g, ok := c.byGroup[p.Group]
		if !ok {
			return nil, fmt.Errorf("permission %q names unknown group %q", p.Key, p.Group)
		}
		if len(p.Scopes) == 0 {
			return nil, fmt.Errorf("permission %q declares no scope", p.Key)
		}
		for _, s := range p.Scopes {
			if !ValidScope(s) {
				return nil, fmt.Errorf("permission %q declares unknown scope %q", p.Key, s)
			}
			if !g.InScope(s) {
				return nil, fmt.Errorf("permission %q is scoped %q but its group %q is not", p.Key, s, g.Key)
			}
		}
		c.byKey[p.Key] = p
		c.perms = append(c.perms, p)
	}

	for _, p := range c.perms {
		for _, imp := range p.Implies {
			if _, ok := c.byKey[imp]; !ok {
				return nil, fmt.Errorf("permission %q implies unknown permission %q", p.Key, imp)
			}
		}
	}

	c.closure = make(map[string][]string, len(c.perms))
	for _, p := range c.perms {
		c.closure[p.Key] = c.expand(p.Key)
	}
	return c, nil
}

// expand walks Implies transitively. The graph is small and hand-written; a
// cycle would loop forever, so visited membership terminates it and the result
// is simply the reachable set.
func (c *Catalog) expand(key string) []string {
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(k string) {
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		p, ok := c.byKey[k]
		if !ok {
			return
		}
		for _, imp := range p.Implies {
			walk(imp)
		}
	}
	walk(key)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Permissions returns every declared permission, in declaration order.
func (c *Catalog) Permissions() []Permission { return c.perms }

// Groups returns every declared group, ordered for display.
func (c *Catalog) Groups() []Group { return c.groups }

// Lookup returns a permission by key.
func (c *Catalog) Lookup(key string) (Permission, bool) {
	p, ok := c.byKey[key]
	return p, ok
}

// Known reports whether key is a declared permission. Route gates are checked
// against this in a test, so a typo in a gate fails the build rather than
// locking everyone out of a page at runtime.
func (c *Catalog) Known(key string) bool {
	_, ok := c.byKey[key]
	return ok
}

// PermissionsFor returns the permissions a role editor for this dashboard may
// offer, in declaration order.
func (c *Catalog) PermissionsFor(s Scope) []Permission {
	out := make([]Permission, 0, len(c.perms))
	for _, p := range c.perms {
		if p.InScope(s) {
			out = append(out, p)
		}
	}
	return out
}

// GroupsFor returns the matrix sections for this dashboard, ordered.
func (c *Catalog) GroupsFor(s Scope) []Group {
	out := make([]Group, 0, len(c.groups))
	for _, g := range c.groups {
		if g.InScope(s) {
			out = append(out, g)
		}
	}
	return out
}

// KeysFor returns every permission key grantable on this dashboard. It is what
// an owner role holds, and the ceiling a role editor may not exceed.
func (c *Catalog) KeysFor(s Scope) []string {
	out := make([]string, 0, len(c.perms))
	for _, p := range c.perms {
		if p.InScope(s) {
			out = append(out, p.Key)
		}
	}
	sort.Strings(out)
	return out
}

// Section is one group with its permissions, for rendering the role editor.
type Section struct {
	Group       Group
	Permissions []Permission
}

// Matrix returns the role editor layout for a dashboard: groups in order, each
// with the permissions filed under it. Empty groups are omitted.
func (c *Catalog) Matrix(s Scope) []Section {
	byGroup := map[string][]Permission{}
	for _, p := range c.PermissionsFor(s) {
		byGroup[p.Group] = append(byGroup[p.Group], p)
	}
	out := make([]Section, 0, len(c.groups))
	for _, g := range c.GroupsFor(s) {
		if ps := byGroup[g.Key]; len(ps) > 0 {
			out = append(out, Section{Group: g, Permissions: ps})
		}
	}
	return out
}

// Expand grows a set of granted keys by their declared implications, so a role
// that may edit a page can always open it. Unknown keys are dropped: a key
// removed from the catalogue must stop granting anything, or a deleted feature
// keeps its access rights in every role row that still names it.
func (c *Catalog) Expand(keys []string) []string {
	seen := map[string]struct{}{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		// A wildcard grant is not a catalogue entry; it is carried through
		// verbatim and answered by Set.Has.
		if k == Wildcard || strings.HasSuffix(k, ".*") {
			seen[k] = struct{}{}
			continue
		}
		for _, e := range c.closure[k] {
			seen[e] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Restrict drops every key that is not grantable on this dashboard, after
// expanding implications.
//
// This is the company-isolation rule in one function: a vendor owner posting a
// role form that names "platform.setting.update" gets a role without it, no
// matter what the form said. Filtering in the editor is a convenience;
// filtering here is the control.
func (c *Catalog) Restrict(keys []string, s Scope) []string {
	out := make([]string, 0, len(keys))
	for _, k := range c.Expand(keys) {
		if p, ok := c.byKey[k]; ok && p.InScope(s) {
			out = append(out, k)
		}
	}
	return out
}

// NavKeys maps each sidebar item key to the permissions that reveal it.
func (c *Catalog) NavKeys(s Scope) map[string][]string {
	out := map[string][]string{}
	for _, p := range c.PermissionsFor(s) {
		if p.Nav != "" {
			out[p.Nav] = append(out[p.Nav], p.Key)
		}
	}
	return out
}
