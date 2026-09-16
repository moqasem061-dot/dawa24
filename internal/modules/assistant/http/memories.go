package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// ListMemories returns active memories for the caller's organization.
func (h *Handler) ListMemories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if actor.OrgID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	var uidPtr *int64
	if actor.UserID > 0 {
		uid := actor.UserID
		uidPtr = &uid
	}

	memories, err := h.repo.ListMemories(ctx, actor.OrgID, uidPtr, 50)
	if err != nil {
		h.log.ErrorContext(ctx, "assistant: list memories", "org_id", actor.OrgID, "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}

	if memories == nil {
		memories = []*assistant.Memory{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"memories": memories,
		"count":    len(memories),
	})
}

// CreateMemory manually adds an organization rule or preference.
func (h *Handler) CreateMemory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if actor.OrgID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	var body struct {
		Content  string `json:"content"`
		Category string `json:"category"`
		Scope    string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	content := strings.TrimSpace(body.Content)
	if content == "" {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}
	if len(content) > 1000 {
		content = content[:1000]
	}

	scope := assistant.MemoryScopeOrganization
	if body.Scope == string(assistant.MemoryScopeUser) {
		scope = assistant.MemoryScopeUser
	}

	cat := assistant.MemoryCategory(body.Category)
	switch cat {
	case assistant.MemoryCategoryProfile, assistant.MemoryCategoryProcurement,
		assistant.MemoryCategoryFinancial, assistant.MemoryCategoryLogistics,
		assistant.MemoryCategoryPreferences, assistant.MemoryCategoryContacts:
	default:
		cat = assistant.MemoryCategoryGeneral
	}

	mem := &assistant.Memory{
		OrganizationID: actor.OrgID,
		Scope:          scope,
		Category:       cat,
		Content:        content,
		Source:         assistant.MemorySourceAdminManual,
		IsActive:       true,
	}
	if scope == assistant.MemoryScopeUser && actor.UserID > 0 {
		uid := actor.UserID
		mem.UserID = &uid
	}

	if err := h.repo.SaveMemory(ctx, mem); err != nil {
		h.log.ErrorContext(ctx, "assistant: save manual memory", "org_id", actor.OrgID, "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"memory": mem,
	})
}

// DeleteMemory deactivates a memory belonging to this organization.
func (h *Handler) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if actor.OrgID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	memoryID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memoryID <= 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	if err := h.repo.DeleteMemory(ctx, actor.OrgID, memoryID); err != nil {
		h.log.ErrorContext(ctx, "assistant: delete memory", "org_id", actor.OrgID, "memory_id", memoryID, "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"memory_id": memoryID,
	})
}
