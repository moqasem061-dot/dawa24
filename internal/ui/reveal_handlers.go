package ui

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/components"
)

// The audit actions of a reveal. Every outcome writes exactly one row.
const (
	revealActionGrant = "reveal.grant"
	revealActionDeny  = "reveal.deny"
)

// revealEntityMaxLen bounds the kind text written to the audit log. A known kind
// is always shorter; the bound only limits what an unknown kind can write.
const revealEntityMaxLen = 64

var errRevealAuditUnavailable = errors.New("reveal: audit writer not configured")

// revealRefusal is one refusal and its answer. The codes are the ERP contract
// names; revealText picks the bilingual message for each.
type revealRefusal struct {
	code   string
	status int
}

var (
	revealNotRevealable = revealRefusal{code: "REVEAL_NOT_REVEALABLE", status: http.StatusUnprocessableEntity}
	revealForbidden     = revealRefusal{code: "REVEAL_FORBIDDEN", status: http.StatusForbidden}
	revealNotFound      = revealRefusal{code: "REVEAL_NOT_FOUND", status: http.StatusNotFound}
	revealUnavailable   = revealRefusal{code: "SERVICE_UNAVAILABLE", status: http.StatusServiceUnavailable}
)

// revealText is the message for a refusal. The keys are constants so each one is
// checked by the i18n catalogue tests.
func revealText(lang string, why revealRefusal) string {
	switch why.code {
	case revealNotRevealable.code:
		return i18n.T(lang, "erp.err.reveal_not_revealable")
	case revealForbidden.code:
		return i18n.T(lang, "erp.err.reveal_forbidden")
	case revealNotFound.code:
		return i18n.T(lang, "erp.err.reveal_not_found")
	default:
		return i18n.T(lang, "common.service_unavailable")
	}
}

// RevealSubmit answers POST /reveal (plan.md S8, plan.md E9). The form carries
// kind and id; the kind decides the permission and the read, never the caller.
//
// The order is fixed. An unknown kind is refused before any read. The permission
// for the kind is checked next. The value is then read, and only a found, non-empty
// value is granted. A grant writes its audit row first and fails closed: when that
// row cannot be written the value is not shown. A refusal writes a deny row. Each
// row is written by the identity service in its own transaction, so a refusal row
// survives the request it describes.
//
// The request reaches this handler through the approved tier, which already
// refuses anonymous and unapproved callers; an unauthenticated request has no actor
// to audit and is refused here without a row.
func (h *UIHandler) RevealSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	kindName := strings.TrimSpace(r.FormValue("kind"))
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)

	actor, ok := authctx.From(ctx)
	if !ok {
		writeRevealRefusal(ctx, w, lang, revealForbidden)
		return
	}
	kind, known := revealKinds[kindName]
	if !known {
		h.refuseReveal(ctx, w, lang, actor, kindName, id, revealNotRevealable)
		return
	}
	if !actor.Can(kind.Permission) {
		h.refuseReveal(ctx, w, lang, actor, kindName, id, revealForbidden)
		return
	}
	value, err := kind.Load(ctx, h, actor, id)
	if err != nil || value == "" {
		if err != nil && !errors.Is(err, errRevealNotFound) {
			h.log.WarnContext(ctx, "reveal read failed", "kind", kindName, "error", err)
		}
		h.refuseReveal(ctx, w, lang, actor, kindName, id, revealNotFound)
		return
	}
	if err := h.writeRevealAudit(ctx, actor, revealActionGrant, kindName, id); err != nil {
		h.log.ErrorContext(ctx, "reveal grant not audited; value withheld", "kind", kindName, "error", err)
		writeRevealRefusal(ctx, w, lang, revealUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := components.RevealedValue(kindName, id, value, lang).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render reveal", "kind", kindName, "error", err)
	}
}

// refuseReveal writes the deny row and then the refusal. A failed deny row is
// logged; the refusal still stands, so a caller never gets the value because the
// log was down.
func (h *UIHandler) refuseReveal(ctx context.Context, w http.ResponseWriter, lang string, actor authctx.Actor, kind string, id int64, why revealRefusal) {
	if err := h.writeRevealAudit(ctx, actor, revealActionDeny, kind, id); err != nil {
		h.log.ErrorContext(ctx, "reveal refusal not audited", "kind", kind, "error", err)
	}
	writeRevealRefusal(ctx, w, lang, why)
}

// writeRevealRefusal answers with the refusal's status and its bilingual text.
func writeRevealRefusal(ctx context.Context, w http.ResponseWriter, lang string, why revealRefusal) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(why.status)
	_ = components.RevealRefusal(why.code, revealText(lang, why)).Render(ctx, w)
}

// writeRevealAudit writes one reveal row through the identity service. The row
// holds the actor, the organization, the action, the kind and the id. It never
// holds the value: Before and After stay empty.
func (h *UIHandler) writeRevealAudit(ctx context.Context, actor authctx.Actor, action, kind string, id int64) error {
	if h.idSvc == nil {
		return errRevealAuditUnavailable
	}
	entry := database.AuditEntry{
		ActorUserID: actor.UserID,
		Action:      action,
		EntityType:  revealEntityType(kind),
		EntityID:    strconv.FormatInt(id, 10),
	}
	if actor.OrganizationID > 0 {
		orgID := actor.OrganizationID
		entry.OrganizationID = &orgID
	}
	return h.idSvc.RecordAudit(ctx, entry)
}

// revealEntityType returns the kind for the audit row, bounded in length.
func revealEntityType(kind string) string {
	if kind == "" {
		return "unknown"
	}
	if len(kind) > revealEntityMaxLen {
		return kind[:revealEntityMaxLen]
	}
	return kind
}
