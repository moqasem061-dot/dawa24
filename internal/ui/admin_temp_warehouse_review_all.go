package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

const warehouseReviewKind = "temp_warehouse_review"

// One platform-wide pass at a time, including requests from different admins.
var warehouseReviewSlot = make(chan struct{}, 1)

type warehouseReviewPayload struct {
	FileIDs   []int64 `json:"file_ids"`
	Completed int     `json:"completed"`
	Succeeded int     `json:"succeeded"`
	FailedIDs []int64 `json:"failed_ids"`
}

func (h *UIHandler) AdminTempWarehouseReviewAllSubmit(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsSuperAdmin() {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if h.compareSvc == nil || h.importRunRepo == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	select {
	case warehouseReviewSlot <- struct{}{}:
	default:
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", i18n.T(langOf(r), "warehouse.review.busy"))
		return
	}
	ctx := database.AsSystem(r.Context())
	run, payload, err := h.warehouseReviewRun(ctx, actor)
	if err != nil {
		<-warehouseReviewSlot
		h.log.ErrorContext(ctx, "start warehouse review", "error", err)
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", i18n.T(langOf(r), "warehouse.review.start_failed"))
		return
	}
	go h.applyWarehouseReview(context.WithoutCancel(ctx), run, payload)
	http.Redirect(w, r, fmt.Sprintf("%s/review-all/%d", tempWarehouseSuperBase, run.ID), http.StatusSeeOther)
}

func (h *UIHandler) warehouseReviewRun(ctx context.Context, actor authctx.Actor) (*importrun.Run, *warehouseReviewPayload, error) {
	run, err := h.importRunRepo.GetActiveRunForUser(ctx, actor.UserID, warehouseReviewKind)
	if err != nil {
		return nil, nil, err
	}
	if run != nil {
		var payload warehouseReviewPayload
		if err := json.Unmarshal(run.Payload, &payload); err != nil {
			return nil, nil, err
		}
		if payload.Completed < 0 || payload.Completed > len(payload.FileIDs) || len(payload.FileIDs) == 0 {
			return nil, nil, fmt.Errorf("invalid review checkpoint")
		}
		return run, &payload, nil
	}
	files, err := h.compareSvc.ListAdminTempWarehouses(ctx, compare.AdminTempWarehouseFilter{})
	if err != nil {
		return nil, nil, err
	}
	payload := &warehouseReviewPayload{}
	for _, file := range files {
		if file != nil && file.CompareFile != nil && file.DeletedAt == nil {
			payload.FileIDs = append(payload.FileIDs, file.ID)
		}
	}
	if len(payload.FileIDs) == 0 {
		return nil, nil, fmt.Errorf("no warehouses to review")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	run = &importrun.Run{UserID: actor.UserID, OrganizationID: actor.OrganizationID, Kind: warehouseReviewKind, Audience: importrun.AudienceAdmin, Filename: "warehouse column review", State: importrun.StateProcessing, Phase: "review", TotalRows: len(payload.FileIDs), Payload: encoded}
	if err := h.importRunRepo.CreateRun(ctx, run); err != nil {
		return nil, nil, err
	}
	return run, payload, nil
}

func (h *UIHandler) applyWarehouseReview(ctx context.Context, run *importrun.Run, payload *warehouseReviewPayload) {
	defer func() { <-warehouseReviewSlot }()
	defer func() {
		if recovered := recover(); recovered != nil {
			h.log.ErrorContext(ctx, "warehouse review panic", "panic", recovered, "run_id", run.ID)
			h.failWarehouseReview(ctx, run.ID)
		}
	}()
	for payload.Completed < len(payload.FileIDs) {
		fileID := payload.FileIDs[payload.Completed]
		fileCtx, cancel := context.WithTimeout(ctx, tempWarehouseStageTimeout)
		err := h.compareSvc.ReviewFileColumns(fileCtx, fileID)
		cancel()
		if err != nil {
			h.log.ErrorContext(ctx, "review warehouse columns", "file_id", fileID, "error", err)
			payload.FailedIDs = append(payload.FailedIDs, fileID)
		} else {
			payload.Succeeded++
		}
		payload.Completed++
		if err := h.saveWarehouseReviewCheckpoint(ctx, run.ID, payload); err != nil {
			h.log.ErrorContext(ctx, "save warehouse review checkpoint", "error", err, "run_id", run.ID)
			h.failWarehouseReview(ctx, run.ID)
			return
		}
	}
	if len(payload.FailedIDs) > 0 {
		h.failWarehouseReview(ctx, run.ID)
		return
	}
	if err := h.importRunRepo.TransitionState(ctx, run.ID, importrun.StateCommitted); err != nil {
		h.log.ErrorContext(ctx, "finish warehouse review", "error", err, "run_id", run.ID)
		h.failWarehouseReview(ctx, run.ID)
	}
}

func (h *UIHandler) saveWarehouseReviewCheckpoint(ctx context.Context, runID int64, payload *warehouseReviewPayload) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := h.importRunRepo.UpdatePayload(ctx, runID, encoded); err != nil {
		return err
	}
	return h.importRunRepo.UpdateProgress(ctx, runID, "review", payload.Completed*100/len(payload.FileIDs), payload.Completed)
}

func (h *UIHandler) failWarehouseReview(ctx context.Context, runID int64) {
	if err := h.importRunRepo.FailRun(ctx, runID, i18n.T("ar", "warehouse.review.partial")); err != nil {
		h.log.ErrorContext(ctx, "record warehouse review failure", "run_id", runID, "error", err)
	}
}

func (h *UIHandler) AdminTempWarehouseReviewAllPage(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsSuperAdmin() {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if h.importRunRepo == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "runID"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	run, err := h.importRunRepo.GetRunByID(database.AsSystem(r.Context()), id)
	if err != nil || run == nil || run.Kind != warehouseReviewKind || run.UserID != actor.UserID || run.OrganizationID != actor.OrganizationID {
		http.NotFound(w, r)
		return
	}
	var payload warehouseReviewPayload
	if err := json.Unmarshal(run.Payload, &payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	lang, dir := h.localeAndDir(r)
	h.renderPage(r.Context(), w, "warehouse review progress", pages.AdminWarehouseReviewProgress(pages.WarehouseReviewProgress{URL: r.URL.Path, Total: len(payload.FileIDs), Completed: payload.Completed, Succeeded: payload.Succeeded, FailedIDs: payload.FailedIDs, Done: run.IsDone(), Failed: run.State == importrun.StateFailed}, lang, dir))
}
