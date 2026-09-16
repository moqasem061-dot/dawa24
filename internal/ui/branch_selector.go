// Buying-branch selector for the customer shell (Rebuild V2 §3.2).
//
// The dropdown belongs in the shell, not on a page: changing the branch
// changes what the whole catalogue shows. The option list always comes from
// the database; the cookie only carries the chosen branch id, and every
// handler resolves coordinates from the branch record — never from the
// request — so a forged cookie cannot inject coordinates.
package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// buyingBranchCookie persists the chosen branch per browser.
const buyingBranchCookie = "dawa24_buying_branch"

// BuyingBranchSelector binds the customer's branch options and the active
// choice into the request context. Non-customer actors pass through untouched.
func (h *UIHandler) BuyingBranchSelector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authctx.From(r.Context())
		if !ok || !actor.IsBuyer() || h.orgSvc == nil {
			next.ServeHTTP(w, r)
			return
		}

		options := h.customerBranchOptions(r, actor)
		if len(options) == 0 {
			// Keep an explicit empty selection in context. Buying handlers must
			// not accept a hand-posted branch when the account has no usable
			// branch, because that would bypass the shell's receiving context.
			ctx := authctx.WithBuyingBranch(r.Context(), authctx.BuyingBranch{})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		selection := authctx.BuyingBranch{Branches: options}
		// Only non-owner staff strictly bound to a single branch get locked.
		// Owners, managers and staff with all branches can freely view and switch between all branches.
		isBoundToBranch := !actor.IsOwner && actor.BoundBranchID != nil && *actor.BoundBranchID > 0
		if isBoundToBranch && len(options) == 1 && options[0].ID == *actor.BoundBranchID {
			selection.Active = actor.BoundBranchID
			selection.IsLocked = true
		} else if active := h.validatedCookieBranch(r, actor, options); active != nil {
			selection.Active = active
			actor.BranchID = active
		} else if len(options) > 0 {
			selection.Active = &options[0].ID
			actor.BranchID = &options[0].ID
		}

		ctx := authctx.WithBuyingBranch(r.Context(), selection)
		ctx = authctx.WithActor(ctx, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// branchCacheTTL bounds how stale the selector's option list may be.
//
// The list was read from the database on EVERY request a customer made: the
// selector is shell chrome, so it renders on the catalogue, the cart, every
// order screen and every htmx fragment within them. pg_stat_user_tables
// recorded 46,272 sequential scans of org.branches, a table with four rows in
// it, and each one cost a full read transaction — four network round trips.
//
// A branch is created or renamed perhaps a few times in a company's life, and
// the cost of being thirty seconds late to notice is that a dropdown shows a
// stale name for half a minute. The cost of not caching it is four round trips
// on every page view.
//
// Thirty seconds rather than a longer window because there is no cross-process
// invalidation here: the ceiling IS the staleness bound. branchOptionsCache is
// also cleared directly by the branch write paths in this process, so the
// person who just renamed a branch sees it immediately.
const branchCacheTTL = 30 * time.Second

type branchOptionsEntry struct {
	options []authctx.BranchOption
	fetched time.Time
}

var (
	branchOptionsMu    sync.RWMutex
	branchOptionsCache = map[string]branchOptionsEntry{}
)

// InvalidateBranchOptionsCache drops the cached selector list for one
// organisation. Call it from anything that creates, renames, deactivates or
// deletes a branch.
func InvalidateBranchOptionsCache(orgID int64) {
	branchOptionsMu.Lock()
	defer branchOptionsMu.Unlock()
	for k := range branchOptionsCache {
		if strings.HasPrefix(k, strconv.FormatInt(orgID, 10)+":") {
			delete(branchOptionsCache, k)
		}
	}
}

// customerBranchOptions lists the actor's active branches for the selector.
func (h *UIHandler) customerBranchOptions(r *http.Request, actor authctx.Actor) []authctx.BranchOption {
	lang := langOf(r)

	// Keyed by organisation, language AND the actor's permanent branch binding, because
	// all three change what the list contains: names are localised, and an
	// employee strictly bound to one branch in org.members sees only that branch.
	var bound int64
	if actor.BoundBranchID != nil && *actor.BoundBranchID > 0 {
		bound = *actor.BoundBranchID
	} else if !actor.IsOwner && actor.BoundBranchID == nil && actor.BranchID != nil && *actor.BranchID > 0 {
		// Fallback if BoundBranchID was not tracked yet
		bound = *actor.BranchID
	}
	key := strconv.FormatInt(actor.OrganizationID, 10) + ":" + lang +
		":" + strconv.FormatInt(bound, 10) + ":" + strconv.FormatBool(actor.IsOwner)

	branchOptionsMu.RLock()
	if e, ok := branchOptionsCache[key]; ok && time.Since(e.fetched) < branchCacheTTL {
		branchOptionsMu.RUnlock()
		return e.options
	}
	branchOptionsMu.RUnlock()

	options := h.loadCustomerBranchOptions(r.Context(), actor, lang)

	branchOptionsMu.Lock()
	// Per-process and otherwise unbounded; a large estate would grow it without
	// limit. Clearing wholesale past a ceiling costs one re-read per company
	// and is cheaper than tracking eviction order.
	if len(branchOptionsCache) > 5000 {
		branchOptionsCache = map[string]branchOptionsEntry{}
	}
	branchOptionsCache[key] = branchOptionsEntry{options: options, fetched: time.Now()}
	branchOptionsMu.Unlock()

	return options
}

// loadCustomerBranchOptions is the uncached read.
func (h *UIHandler) loadCustomerBranchOptions(ctx context.Context, actor authctx.Actor, lang string) []authctx.BranchOption {
	branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil {
		return nil
	}

	isBoundToBranch := !actor.IsOwner && actor.BoundBranchID != nil && *actor.BoundBranchID > 0
	boundID := int64(0)
	if isBoundToBranch {
		boundID = *actor.BoundBranchID
	}

	var mainBranch *org.Branch
	var otherBranches []*org.Branch
	for _, b := range branches {
		if b == nil || b.OrganizationID != actor.OrganizationID || b.Status == "inactive" || b.Status == "suspended" {
			continue
		}
		// Non-owner employee strictly bound to an assigned branch only sees their branch
		if isBoundToBranch && b.ID != boundID {
			continue
		}
		if b.IsMain && mainBranch == nil {
			mainBranch = b
		} else {
			otherBranches = append(otherBranches, b)
		}
	}

	options := make([]authctx.BranchOption, 0, len(branches))
	if mainBranch != nil {
		options = append(options, authctx.BranchOption{ID: mainBranch.ID, Name: branchName(mainBranch, lang)})
	}
	for _, b := range otherBranches {
		options = append(options, authctx.BranchOption{ID: b.ID, Name: branchName(b, lang)})
	}

	// Fallback if employee assigned branch was inactive/suspended/not found
	if len(options) == 0 && isBoundToBranch {
		for _, b := range branches {
			if b == nil || b.OrganizationID != actor.OrganizationID || b.Status == "inactive" || b.Status == "suspended" {
				continue
			}
			options = append(options, authctx.BranchOption{ID: b.ID, Name: branchName(b, lang)})
		}
	}

	return options
}

// validatedCookieBranch accepts the cookie value only when it names one of
// the actor's own branches. The options already come from an
// organization-scoped database listing, so membership alone proves ownership;
// no extra per-request branch fetch is needed.
func (h *UIHandler) validatedCookieBranch(r *http.Request, actor authctx.Actor, options []authctx.BranchOption) *int64 {
	cookie, err := r.Cookie(buyingBranchCookie)
	id := parseBranchID(cookie)
	if err != nil || id <= 0 {
		return nil
	}
	for _, o := range options {
		if o.ID == id {
			return &id
		}
	}
	return nil
}

// SetBuyingBranchSubmit persists the chosen branch for this browser and
// returns to the previous page, which re-renders with the new selection.
func (h *UIHandler) SetBuyingBranchSubmit(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsBuyer() || h.orgSvc == nil {
		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
		return
	}
	// A refused switch goes back to the caller's own dashboard. It used to go
	// to the pharmacy's, which a supplier is not allowed to open.
	home := dashboardHome(actor)

	id := parseBranchID(&http.Cookie{Value: r.PostFormValue("branch_id")})
	if id <= 0 {
		http.Redirect(w, r, home, http.StatusSeeOther)
		return
	}

	// 1. Only non-owner employees strictly bound to a single branch in membership cannot switch away from it.
	// Employees with all branches (BoundBranchID == nil) or owners are completely free to switch.
	if !actor.IsOwner && actor.BoundBranchID != nil && *actor.BoundBranchID > 0 && *actor.BoundBranchID != id {
		http.Redirect(w, r, home, http.StatusSeeOther)
		return
	}

	// 2. Check buying branch context: if selection was marked locked, refuse switch
	if buying, ok := authctx.BuyingBranchFrom(r.Context()); ok {
		if buying.IsLocked {
			http.Redirect(w, r, home, http.StatusSeeOther)
			return
		}
		if len(buying.Branches) > 0 {
			valid := false
			for _, b := range buying.Branches {
				if b.ID == id {
					valid = true
					break
				}
			}
			if !valid {
				http.Redirect(w, r, home, http.StatusSeeOther)
				return
			}
		}
	}

	// 3. Verify target branch exists, belongs to caller's company, and is active
	branch, err := h.orgSvc.GetBranch(r.Context(), id)
	if err != nil || branch == nil || branch.OrganizationID != actor.OrganizationID || branch.Status == "inactive" || branch.Status == "suspended" {
		http.Redirect(w, r, home, http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     buyingBranchCookie,
		Value:    strconv.FormatInt(id, 10),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	InvalidateBranchOptionsCache(actor.OrganizationID)

	target := r.PostFormValue("redirect_to")
	if target == "" {
		target = r.Referer()
	}
	if target == "" {
		target = home
	}
	target = safeLocalRedirect(target, home)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// parseBranchID decodes a plain-decimal cookie value, or 0 on garbage.
func parseBranchID(c *http.Cookie) int64 {
	if c == nil || c.Value == "" {
		return 0
	}
	var id int64
	for _, ch := range c.Value {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + int64(ch-'0')
	}
	if id <= 0 {
		return 0
	}
	return id
}

// branchName prefers the current language branch name, then the alternate, then code.
func branchName(b *org.Branch, lang ...string) string {
	if b == nil {
		return ""
	}
	l := "ar"
	if len(lang) > 0 && lang[0] != "" {
		l = lang[0]
	}
	if name := b.Name.Get(i18n.Lang(l)); name != "" {
		return name
	}
	if l == "en" {
		return "Branch " + b.Code
	}
	return i18n.TDefault("w4_ui.w4str_16_16") + b.Code
}
