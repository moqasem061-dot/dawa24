package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AIToolDef provides descriptive metadata for a configurable AI tool/role.
type AIToolDef struct {
	Role         gateway.Role
	NameAR       string
	Description  string
	DefaultModel string
}

// ModelOption describes a model available in the AI gateway.
type ModelOption struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"model_type"`
	Context     int    `json:"context_window"`
}

// AdminAIFetchModelsAPI contacts the AI gateway to list available models live.
func (h *UIHandler) AdminAIFetchModelsAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	adminClient, _, ok := h.getGatewayAdminClient(ctx)
	if ok && adminClient != nil {
		models, err := adminClient.ListModels(ctx)
		if err == nil && len(models) > 0 {
			res := make([]ModelOption, 0, len(models))
			for _, m := range models {
				disp := m.DisplayName
				if disp == "" {
					disp = m.Name
				}
				if disp == "" {
					disp = m.ID
				}
				res = append(res, ModelOption{
					ID:          m.ID,
					DisplayName: disp,
					Type:        m.ModelType,
					Context:     m.ContextWindow,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": res})
			return
		}
	}

	// Fallback list of curated models when live gateway query is unreachable
	defaultOptions := []ModelOption{
		{ID: "qwen3.7-flash", DisplayName: "Qwen 3.7 Flash (Fast & Structured)", Type: "chat", Context: 131072},
		{ID: "gemma-4-31b-it", DisplayName: "Gemma 4 31B IT (High Quality)", Type: "chat", Context: 131072},
		{ID: "whisper-large-v3-turbo", DisplayName: "Whisper Large v3 Turbo (Voice)", Type: "transcribe", Context: 16000},
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"models": defaultOptions})
}

// AdminAISaveRoleModelSubmit saves a per-tool AI model mapping to the database.
func (h *UIHandler) AdminAISaveRoleModelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	isJSON := r.Header.Get("Accept") == "application/json"

	sendError := func(status int, msg string) {
		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": msg})
			return
		}
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", msg)
	}

	if h.adminSvc == nil {
		sendError(http.StatusServiceUnavailable, i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		sendError(http.StatusBadRequest, "يجب تحديد أداة / دور الذكاء الاصطناعي")
		return
	}

	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		sendError(http.StatusBadRequest, "يجب تحديد اسم النموذج")
		return
	}

	isActive := r.FormValue("is_active") == "true" || r.FormValue("is_active") == "1" || r.FormValue("is_active") == "on"
	notes := strings.TrimSpace(r.FormValue("notes"))

	var maxTokensPtr *int
	if maxStr := strings.TrimSpace(r.FormValue("max_tokens")); maxStr != "" {
		if val, err := strconv.Atoi(maxStr); err == nil && val > 0 {
			maxTokensPtr = &val
		}
	}

	var updatedBy *int64
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		updatedBy = &actor.UserID
	}

	rm := platformadmin.AIRoleModel{
		Role:      role,
		Model:     model,
		IsActive:  isActive,
		MaxTokens: maxTokensPtr,
		Notes:     notes,
		UpdatedBy: updatedBy,
	}

	if err := h.adminSvc.SaveAIRoleModel(ctx, &rm); err != nil {
		sendError(http.StatusInternalServerError, h.safeMessage(err, lang))
		return
	}

	if h.gatewayKeys != nil {
		h.gatewayKeys.Invalidate()
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "role": role, "model": model})
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "success", "تم حفظ إعدادات النموذج للأداة المحددة بنجاح")
}
