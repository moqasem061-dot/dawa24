package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// The sync surface: POST /products/sync (202 + status URL), GET
// /products/sync/{syncId}, GET /products/sync/{syncId}/items. Every handler
// reads the organization from the authenticated principal — never from the
// request — and item errors decode back to bilingual messages by their stored
// code (trap 24: no key material, or any payload, in the error path).

func (h *Handler) postSync(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		return // readBody already wrote the envelope
	}
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey != "" && len(idemKey) > 100 {
		writeError(w, r, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"idempotencyKey"}}))
		return
	}
	accepted, err := h.svc.AcceptBatch(r.Context(), principal, body, idemKey)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, http.StatusAccepted, map[string]any{
		"syncId":           accepted.SyncID,
		"state":            accepted.State,
		"itemCount":        accepted.ItemCount,
		"rejectedAtIntake": accepted.RejectedAtIntake,
		"statusUrl":        "/erp/v1/products/sync/" + accepted.SyncID,
	})
}

func (h *Handler) getSync(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	batch, err := h.svc.BatchStatus(r.Context(), principal.OrganizationID, chi.URLParam(r, "syncId"))
	if err != nil {
		writeError(w, r, syncNotFound(err))
		return
	}
	writeOK(w, r, http.StatusOK, map[string]any{
		"syncId":           batch.PublicID,
		"state":            batch.State,
		"itemCount":        batch.ItemCount,
		"chunkCount":       batch.ChunkCount,
		"chunksDone":       batch.ChunksDone,
		"created":          batch.CreatedCount,
		"updated":          batch.UpdatedCount,
		"unchanged":        batch.UnchangedCount,
		"needsReviewCount": batch.NeedsReviewCount,
		"failedCount":      batch.FailedCount,
		"acceptedAt":       batch.AcceptedAt,
		"finishedAt":       batch.FinishedAt,
	})
}

func (h *Handler) getSyncItems(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	batch, err := h.svc.BatchStatus(r.Context(), principal.OrganizationID, chi.URLParam(r, "syncId"))
	if err != nil {
		writeError(w, r, syncNotFound(err))
		return
	}
	cursor, err := decodeCursorParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	state := r.URL.Query().Get("state")
	limit, err := itemsLimit(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.svc.BatchItems(r.Context(), principal.OrganizationID, batch.ID, cursor, state, limit)
	if err != nil {
		writeError(w, r, erp.NewError("ERP_INTERNAL", 500, nil))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"code":      item.ExternalCode,
			"state":     item.State,
			"message":   itemMessage(item),
			"variantId": item.VariantID,
			"cursor":    encodeItemCursor(item.ID),
		})
	}
	next := ""
	if len(items) == limit {
		next = encodeItemCursor(items[len(items)-1].ID)
	}
	writeOK(w, r, http.StatusOK, map[string]any{"items": out, "nextCursor": next})
}

func syncNotFound(err error) error {
	_ = err // the not-found path is deliberately indistinguishable from missing
	return erp.NewError("ERP_SYNC_NOT_FOUND", 404, nil)
}

// itemMessage rebuilds the bilingual message from the stored code: the stored
// payload is never shipped to the caller.
func itemMessage(item erp.SyncItemRow) map[string]string {
	if item.Code == nil || *item.Code == "" {
		return map[string]string{}
	}
	translate := i18n.T
	key := "erp.err." + strings.ToLower(strings.TrimPrefix(*item.Code, "ERP_"))
	return map[string]string{
		"ar": translate(i18n.AR, key),
		"en": translate(i18n.EN, key),
	}
}

// itemsLimit caps the items page at SyncItemsPageMax with the default of
// SyncItemsPageDefault; a bad or over-large request is ERP_CURSOR_INVALID's
// sibling (a 422 naming the offending field).
func itemsLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return erp.SyncItemsPageDefault, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"limit"}})
	}
	if limit > erp.SyncItemsPageMax {
		limit = erp.SyncItemsPageMax
	}
	return limit, nil
}

// decodeCursorParam reads the optional `after` item id (keyset by sync_items.id);
// garbage on the cursor parameter is a validation refusal.
func decodeCursorParam(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("after")
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, erp.NewError("ERP_CURSOR_INVALID", 400, nil)
	}
	return id, nil
}

// encodeItemCursor is the same base64url-of-v1:<id> cursor contract the orders
// endpoints use (S3); the sync's pages are plain ints until then, wrapped in
// the same shape.
func encodeItemCursor(id int64) string {
	return strconv.FormatInt(id, 10)
}
