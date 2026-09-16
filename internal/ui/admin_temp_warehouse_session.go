package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
)

// TempWarehousePhases represents the lifecycle of a resumable temporary warehouse session.
const (
	PhaseUploaded   = "uploaded"
	PhaseMapping    = "mapping"
	PhaseStaging    = "staging"
	PhaseReview     = "review"
	PhaseCommitting = "committing"
	PhaseDone       = "done"
	PhaseFailed     = "failed"
)

// TempWarehouseRunPayload holds in-progress session state durable on platform.import_runs.
type TempWarehouseRunPayload struct {
	FileIDs       []int64                         `json:"file_ids"`
	CurrentFileID int64                           `json:"current_file_id"`
	Step          int                             `json:"step"`
	TotalFiles    int                             `json:"total_files"`
	Mappings      map[int64]compare.MappingConfig `json:"mappings"`
	SupplierNames map[int64]string                `json:"supplier_names"`
	BaseURL       string                          `json:"base_url"`
}

// DecodeTempWarehousePayload unmarshals the JSONB payload from an import run.
func DecodeTempWarehousePayload(raw json.RawMessage) (*TempWarehouseRunPayload, error) {
	if len(raw) == 0 {
		return &TempWarehouseRunPayload{
			Mappings:      make(map[int64]compare.MappingConfig),
			SupplierNames: make(map[int64]string),
		}, nil
	}
	var p TempWarehouseRunPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Mappings == nil {
		p.Mappings = make(map[int64]compare.MappingConfig)
	}
	if p.SupplierNames == nil {
		p.SupplierNames = make(map[int64]string)
	}
	return &p, nil
}

// CreateTempWarehouseRun creates a new durable import run session for a batch of uploaded warehouse files.
func (h *UIHandler) CreateTempWarehouseRun(
	ctx context.Context,
	actor authctx.Actor,
	fileIDs []int64,
	supplierNames map[int64]string,
	baseURL string,
) (*importrun.Run, error) {
	if h.importRunRepo == nil {
		return nil, fmt.Errorf("importrun repository unavailable")
	}
	if len(fileIDs) == 0 {
		return nil, fmt.Errorf("no files to create import run")
	}

	payload := TempWarehouseRunPayload{
		FileIDs:       fileIDs,
		CurrentFileID: fileIDs[0],
		Step:          1,
		TotalFiles:    len(fileIDs),
		Mappings:      make(map[int64]compare.MappingConfig),
		SupplierNames: supplierNames,
		BaseURL:       baseURL,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	filename := fmt.Sprintf("%d_warehouse_files_batch", len(fileIDs))
	if len(fileIDs) == 1 && supplierNames[fileIDs[0]] != "" {
		filename = supplierNames[fileIDs[0]]
	}

	run := &importrun.Run{
		OrganizationID: actor.OrganizationID,
		UserID:         actor.UserID,
		Kind:           importrun.KindTempWarehouse,
		Audience:       importrun.AudienceAdmin,
		Filename:       filename,
		State:          importrun.StateProcessing,
		Phase:          PhaseMapping,
		Percent:        10,
		TotalRows:      0,
		ProcessedRows:  0,
		Payload:        payloadBytes,
	}

	if err := h.importRunRepo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// ResolveTempWarehouseRun fetches a run by its public ID or numeric ID.
func (h *UIHandler) ResolveTempWarehouseRun(ctx context.Context, runIDStr string) (*importrun.Run, error) {
	if h.importRunRepo == nil {
		return nil, fmt.Errorf("importrun repository unavailable")
	}
	sysCtx := database.AsSystem(ctx)

	// Try numeric ID first
	if id, err := strconv.ParseInt(runIDStr, 10, 64); err == nil && id > 0 {
		if run, err := h.importRunRepo.GetRunByID(sysCtx, id); err == nil && run != nil {
			return run, nil
		}
	}

	// Try public UUID
	return h.importRunRepo.GetRunByPublicIDSystem(sysCtx, runIDStr)
}

// GetActiveTempWarehouseRun returns the most recent unfinished session for an admin actor.
func (h *UIHandler) GetActiveTempWarehouseRun(ctx context.Context, userID int64) (*importrun.Run, *TempWarehouseRunPayload) {
	if h.importRunRepo == nil || userID <= 0 {
		return nil, nil
	}
	run, err := h.importRunRepo.GetActiveRunForUser(database.AsSystem(ctx), userID, importrun.KindTempWarehouse)
	if err != nil || run == nil {
		return nil, nil
	}
	payload, err := DecodeTempWarehousePayload(run.Payload)
	if err != nil {
		return nil, nil
	}
	return run, payload
}
