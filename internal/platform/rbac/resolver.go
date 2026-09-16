package rbac

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Grant is one caller's resolved authority.
type Grant struct {
	UserID         int64
	OrganizationID int64
	// PlatformRole is identity.users.role.
	PlatformRole string
	// Active is true when the account exists, is not deleted and its status is
	// 'active'. A browser session is revoked when an account is suspended, so
	// the web never needed this; a caller with no session — a Telegram message
	// — must check it, because membership grants still resolve for a
	// suspended user.
	Active bool
	// IsStaff comes from identity.roles.is_staff, not from a list of role
	// names in code. A super admin who creates a "Finance Moderator" role and
	// marks it staff gets a role whose holders reach /admin/* immediately.
	IsStaff bool
	// IsPlatformOwner marks the role that holds everything, unconditionally.
	IsPlatformOwner bool
	// Scope is the dashboard this caller's permissions belong to.
	Scope Scope
	// OrgType is the organization's type, empty for staff with no membership.
	OrgType string
	// OrgStatus is the organization's current approval status (pending, approved, suspended, rejected).
	OrgStatus string
	// MemberRoleKey and MemberRoleName describe the company role held.
	MemberRoleKey  string
	MemberRoleName string
	// IsOrgOwner marks the company owner, who holds their whole dashboard.
	IsOrgOwner bool
	// BranchID is non-nil when the member is bound to one branch.
	BranchID *int64
	// AvatarURL is identity.users.avatar_url.
	AvatarURL string
	// Name is the user's display name from identity.users.
	Name string
	// Permissions is the resolved holding.
	Permissions Set
	// Keys is the same holding as a sorted slice, for the session record.
	Keys []string
}

// Can answers a permission question.
func (g Grant) Can(permission string) bool { return g.Permissions.Has(permission) }

// CanAny answers whether any of the permissions is held.
func (g Grant) CanAny(permissions ...string) bool { return g.Permissions.HasAny(permissions...) }

// Resolver answers "what may this user do right now", reading the database and
// caching the answer against a version counter.
//
// The problem it exists to solve: permissions were computed once, at login,
// and copied into the Redis session. Revoking a role then had no effect until
// the user signed out — a session opened before the change kept every
// permission it was granted with, for the whole 720-hour session lifetime. A
// permission system that cannot revoke is a permission system in name only.
//
// The counter in identity.rbac_version makes revocation visible across
// processes. Every write that touches roles, grants or memberships bumps it in
// its own transaction; a resolver notices the change and re-reads. The counter
// itself is cached briefly, which bounds the worst case at versionTTL rather
// than at the session lifetime.
type Resolver struct {
	db *database.DB

	mu       sync.RWMutex
	entries  map[string]cacheEntry
	versions map[string]versionEntry
}

type cacheEntry struct {
	grant   Grant
	version int64
	// orgVersion is nil for a caller with no organization.
	orgVersion int64
	expires    time.Time
}

type versionEntry struct {
	value   int64
	fetched time.Time
}

const (
	// versionTTL bounds how long a revocation can stay invisible to a process
	// that did not perform it. Five seconds is short enough that an operator
	// revoking access and immediately testing it sees the new answer, and long
	// enough that the counter is not read on every request of a busy page.
	versionTTL = 5 * time.Second
	// entryTTL caps how long a resolved grant survives even at an unchanged
	// version, so a bug in invalidation degrades to a delay rather than to
	// permanent staleness.
	entryTTL = 2 * time.Minute
)

// NewResolver builds a resolver over a database handle.
func NewResolver(db *database.DB) *Resolver {
	return &Resolver{
		db:       db,
		entries:  make(map[string]cacheEntry),
		versions: make(map[string]versionEntry),
	}
}

// Resolve returns the caller's authority. orgID may be zero for platform staff
// or a user with no membership.
func (r *Resolver) Resolve(ctx context.Context, userID, orgID int64) (Grant, error) {
	if r == nil || r.db == nil {
		return Grant{}, fmt.Errorf("rbac: resolver is not configured")
	}
	if userID <= 0 {
		return Grant{}, fmt.Errorf("rbac: resolve requires a user")
	}

	// Both counters in one call. They used to be read one at a time, each in
	// its own read transaction, so a signed-in user cost EIGHT network round
	// trips every five seconds just to be told nothing had changed.
	keys := []string{PlatformVersionKey}
	if orgID > 0 {
		keys = append(keys, OrgVersionKey(orgID))
	}
	vers, err := r.versionsFor(ctx, keys)
	if err != nil {
		return Grant{}, err
	}
	platformVer := vers[PlatformVersionKey]
	var orgVer int64
	if orgID > 0 {
		orgVer = vers[OrgVersionKey(orgID)]
	}

	key := fmt.Sprintf("%d:%d", userID, orgID)
	now := time.Now()
	r.mu.RLock()
	e, ok := r.entries[key]
	r.mu.RUnlock()
	if ok && e.version == platformVer && e.orgVersion == orgVer && now.Before(e.expires) {
		return e.grant, nil
	}

	g, err := r.load(ctx, userID, orgID)
	if err != nil {
		return Grant{}, err
	}
	r.mu.Lock()
	r.entries[key] = cacheEntry{grant: g, version: platformVer, orgVersion: orgVer, expires: now.Add(entryTTL)}
	// The map is per-process and unbounded otherwise; a large enough estate
	// would grow it without limit. Clearing wholesale when it gets big is
	// cheaper than tracking eviction order and costs one re-read per caller.
	if len(r.entries) > 20000 {
		r.entries = map[string]cacheEntry{key: r.entries[key]}
	}
	r.mu.Unlock()
	return g, nil
}

// Invalidate drops one caller's cached grant in this process. Cross-process
// invalidation is the version counter's job; this simply removes the delay for
// the process that performed the write.
func (r *Resolver) Invalidate(userID, orgID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.entries, fmt.Sprintf("%d:%d", userID, orgID))
	if orgID > 0 {
		delete(r.versions, OrgVersionKey(orgID))
	} else {
		delete(r.versions, PlatformVersionKey)
	}
	r.mu.Unlock()
}

// InvalidateOrg drops all cached grants for the given organization and forces its version counter to be re-read.
func (r *Resolver) InvalidateOrg(orgID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	suffix := fmt.Sprintf(":%d", orgID)
	for k := range r.entries {
		if strings.HasSuffix(k, suffix) {
			delete(r.entries, k)
		}
	}
	delete(r.versions, OrgVersionKey(orgID))
	r.mu.Unlock()
}

// InvalidateAll drops every cached grant and forces the version counters to be
// re-read. Call it after a bulk change such as a catalogue sync.
func (r *Resolver) InvalidateAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.entries = make(map[string]cacheEntry)
	r.versions = make(map[string]versionEntry)
	r.mu.Unlock()
}

// versions returns the current counter for each scope key, reading only the
// ones whose cached copy has expired.
//
// Two changes from the per-key version() this replaced, and both are about
// round trips rather than about correctness.
//
// It reads every stale key in ONE query instead of one query per key. Resolve
// needs two counters — the platform's and the organisation's — and asking for
// them separately doubled the cost of the check for no benefit; they are two
// rows of the same three-column table.
//
// And it reads them outside a transaction. identity.rbac_version has no
// row-level security policy (see database.UnscopedTables, and the test that
// holds that claim to the live schema), so the BEGIN, the set_config and the
// COMMIT that wrapped this read were three round trips protecting nothing.
// Together the two changes take the steady-state cost of an authenticated
// request from eight round trips per five seconds to one.
func (r *Resolver) versionsFor(ctx context.Context, keys []string) (map[string]int64, error) {
	now := time.Now()
	out := make(map[string]int64, len(keys))

	var stale []string
	r.mu.RLock()
	for _, k := range keys {
		if v, ok := r.versions[k]; ok && now.Sub(v.fetched) < versionTTL {
			out[k] = v.value
			continue
		}
		stale = append(stale, k)
	}
	r.mu.RUnlock()

	if len(stale) == 0 {
		return out, nil
	}

	// A key with no row has never had a role change. Zero is a valid version —
	// it moves the moment anything is written — so seed every requested key and
	// let the query overwrite the ones that exist.
	for _, k := range stale {
		out[k] = 0
	}

	rows, err := r.db.QueryUnscoped(ctx,
		`SELECT scope_key, version FROM identity.rbac_version WHERE scope_key = ANY($1);`,
		stale)
	if err != nil {
		return nil, fmt.Errorf("rbac: read versions %v: %w", stale, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			key   string
			value int64
		)
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("rbac: scan version: %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: read versions %v: %w", stale, err)
	}

	r.mu.Lock()
	for _, k := range stale {
		r.versions[k] = versionEntry{value: out[k], fetched: now}
	}
	r.mu.Unlock()
	return out, nil
}
