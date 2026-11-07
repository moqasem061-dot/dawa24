package ui

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// Integration-key handlers (S1). Both audiences share these handlers: the
// registered path picks the audience, which selects the permission keys the
// route gate uses and which shell the page renders. Ownership is read only
// from the signed-in actor (authctx.Actor.IsOwner) — never from a submitted
// field — and the generated secret is returned only in the create and rotate
// responses, always through the show-once dialog.

// erpAudience reports whether the request targets the vendor surface
// (as opposed to the customer surface).
func erpAudience(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/vendor/")
}

// erpAudienceName names the audience for path building.
func erpAudienceName(vendor bool) string {
	if vendor {
		return "vendor"
	}
	return "customer"
}

// erpActorOrg is the caller's organization, accepting either id field.
func erpActorOrg(actor authctx.Actor) int64 {
	if actor.OrganizationID > 0 {
		return actor.OrganizationID
	}
	return actor.OrgID
}

// erpKeysScreen builds the audience's shell around the shared body.
func erpKeysScreen(data pages.ErpKeysPageData, lang, dir string, actor authctx.Actor) templ.Component {
	if data.Audience == "vendor" {
		return pages.ErpKeysVendorPage(data, lang, dir)
	}
	return pages.ErpKeysCustomerPage(data, lang, dir, actor)
}

// erpMessage turns an error into user-safe wording. ERP validation and domain
// errors carry an error code whose bilingual text lives in the erp.err.* keys.
func (h *UIHandler) erpMessage(err error, lang string) string {
	translate := i18n.T
	var e *erp.Error
	if errors.As(err, &e) && e.Code != "" {
		key := "erp.err." + strings.ToLower(strings.TrimPrefix(e.Code, "ERP_"))
		msg := translate(lang, key)
		if msg != "" && msg != key {
			return msg
		}
	}
	return h.safeMessage(err, lang)
}

// erpLoginRedirect sends a signed-out caller to the login screen.
func erpLoginRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
}

// ErpKeysPage renders the caller's own integration keys, under
// /vendor/integrations or /customer/integrations.
func (h *UIHandler) ErpKeysPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vendor := erpAudience(r)
	lang, dir := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	data := pages.ErpKeysPageData{
		Audience: erpAudienceName(vendor),
		Owner:    actor.IsOwner,
	}
	data.NoticeKind, data.NoticeMessage = noticeFrom(r)
	if h.erpSvc != nil {
		keys, err := h.erpSvc.ListKeys(ctx, erpActorOrg(actor))
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.Keys = keys
	}
	h.renderPage(ctx, w, "render erp keys page", erpKeysScreen(data, lang, dir, actor))
}

// ErpKeyCreate creates a key for the caller's organization. On success the
// page is re-rendered with the show-once dialog open: the only place the full
// key ever appears (never the log, never a reload).
func (h *UIHandler) ErpKeyCreate(w http.ResponseWriter, r *http.Request) {
	h.erpKeyMutation(w, r, func(h *UIHandler, ctx erpKeyCtx) {
		in, errs := parseCreateKeyForm(r, ctx.vendor)
		if len(errs) > 0 {
			h.redirectWithNotice(w, r, ctx.back, "error", errs[0].Message)
			return
		}
		in.OrganizationID = ctx.orgID
		in.ActorUserID = ctx.actor.UserID
		in.ActorIsOwner = ctx.actor.IsOwner // from the signed-in actor only
		created, err := h.erpSvc.CreateKey(r.Context(), in)
		if err != nil {
			h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
			return
		}
		h.renderKeysWithShownOnce(w, r, ctx, created.FullKey)
	})
}

// ErpKeyRotate rotates a key; the new secret is shown once, the old key keeps
// working until its grace period ends.
func (h *UIHandler) ErpKeyRotate(w http.ResponseWriter, r *http.Request) {
	h.erpKeyMutation(w, r, func(h *UIHandler, ctx erpKeyCtx) {
		id, ok := ctx.keyID()
		if !ok {
			h.redirectWithNotice(w, r, ctx.back, "error", i18n.T(ctx.lang, "erp.err.not_found"))
			return
		}
		grace, errs := parseRotateForm(r)
		if len(errs) > 0 {
			h.redirectWithNotice(w, r, ctx.back, "error", errs[0].Message)
			return
		}
		created, err := h.erpSvc.RotateKey(r.Context(), ctx.orgID, id, grace, ctx.actor.UserID, ctx.actor.IsOwner)
		if err != nil {
			h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
			return
		}
		h.renderKeysWithShownOnce(w, r, ctx, created.FullKey)
	})
}

// ErpKeyRevoke revokes one of the caller's own keys.
func (h *UIHandler) ErpKeyRevoke(w http.ResponseWriter, r *http.Request) {
	h.erpKeyMutation(w, r, func(h *UIHandler, ctx erpKeyCtx) {
		id, ok := ctx.keyID()
		if !ok {
			h.redirectWithNotice(w, r, ctx.back, "error", i18n.T(ctx.lang, "erp.err.not_found"))
			return
		}
		reason, _ := parseRevokeForm(r)
		if err := h.erpSvc.RevokeKey(r.Context(), ctx.orgID, id, reason, ctx.actor.UserID); err != nil {
			h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
			return
		}
		h.redirectWithNotice(w, r, ctx.back, "success", i18n.T(ctx.lang, "erp.keys.revoked_done"))
	})
}

// ErpKeyLimits changes a key's per-minute request limit.
func (h *UIHandler) ErpKeyLimits(w http.ResponseWriter, r *http.Request) {
	h.erpKeyMutation(w, r, func(h *UIHandler, ctx erpKeyCtx) {
		id, ok := ctx.keyID()
		if !ok {
			h.redirectWithNotice(w, r, ctx.back, "error", i18n.T(ctx.lang, "erp.err.not_found"))
			return
		}
		rate, errs := parseLimitsForm(r)
		if len(errs) > 0 {
			h.redirectWithNotice(w, r, ctx.back, "error", errs[0].Message)
			return
		}
		// actorIsOwner comes from the signed-in actor, never from the form.
		if err := h.erpSvc.UpdateKeyLimits(r.Context(), ctx.orgID, id, rate, ctx.actor.UserID, ctx.actor.IsOwner); err != nil {
			h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
			return
		}
		h.redirectWithNotice(w, r, ctx.back, "success", i18n.T(ctx.lang, "erp.keys.limits_saved"))
	})
}

// ErpIntegrationPause pauses or resumes the organization's integration.
func (h *UIHandler) ErpIntegrationPause(w http.ResponseWriter, r *http.Request) {
	h.erpKeyMutation(w, r, func(h *UIHandler, ctx erpKeyCtx) {
		_ = r.ParseForm()
		pause := r.Form.Get("pause") == "true"
		if err := h.erpSvc.PauseIntegration(r.Context(), ctx.orgID, pause, ctx.actor.UserID); err != nil {
			h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
			return
		}
		key := "erp.keys.resumed"
		if pause {
			key = "erp.keys.paused"
		}
		translate := i18n.T
		h.redirectWithNotice(w, r, ctx.back, "success", translate(ctx.lang, key))
	})
}

// erpKeyCtx is the shared context of one key mutation request.
type erpKeyCtx struct {
	actor  authctx.Actor
	orgID  int64
	vendor bool
	lang   string
	dir    string
	back   string
	req    *http.Request
}

// keyID parses the {id} route parameter.
func (c erpKeyCtx) keyID() (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(c.req, "id"), 10, 64)
	return id, err == nil && id > 0
}

// erpKeyMutation is the shared frame around every key mutation: resolve the
// actor, require the owner (the notice the test's non-owner case asserts),
// refuse a disabled service, then run the body.
func (h *UIHandler) erpKeyMutation(w http.ResponseWriter, r *http.Request, body func(*UIHandler, erpKeyCtx)) {
	ctx := r.Context()
	vendor := erpAudience(r)
	lang, dir := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	back := fmt.Sprintf("/%s/integrations", erpAudienceName(vendor))
	if !actor.IsOwner {
		// Ownership is the signed-in actor's property. A posted is_owner or
		// actorIsOwner field is never read.
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.keys.owner_only"))
		return
	}
	if h.erpSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	body(h, erpKeyCtx{
		actor:  actor,
		orgID:  erpActorOrg(actor),
		vendor: vendor,
		lang:   lang,
		dir:    dir,
		back:   back,
		req:    r,
	})
}

// renderKeysWithShownOnce re-renders the keys page (with the fresh list) and
// appends the show-once dialog carrying the full secret. The secret is in
// this response only — never logged, never a redirect target.
func (h *UIHandler) renderKeysWithShownOnce(w http.ResponseWriter, r *http.Request, ctx erpKeyCtx, fullKey string) {
	keys, err := h.erpSvc.ListKeys(r.Context(), ctx.orgID)
	if err != nil {
		h.redirectWithNotice(w, r, ctx.back, "error", h.erpMessage(err, ctx.lang))
		return
	}
	data := pages.ErpKeysPageData{
		Keys:     keys,
		Audience: erpAudienceName(ctx.vendor),
		Owner:    ctx.actor.IsOwner,
	}
	h.renderPage(r.Context(), w, "render erp key with show-once", erpKeysScreen(data, ctx.lang, ctx.dir, ctx.actor))
	if rerr := pages.ErpKeyShownOnceModal(fullKey, ctx.lang).Render(r.Context(), w); rerr != nil {
		h.log.ErrorContext(r.Context(), "render erp show-once modal", "error", rerr)
	}
}
