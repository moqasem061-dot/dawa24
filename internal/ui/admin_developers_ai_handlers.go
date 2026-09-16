package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AIToolDef provides descriptive metadata for a configurable AI tool/role.
type AIToolDef struct {
	Role         gateway.Role
	NameAR       string
	Description  string
	DefaultModel string
}

// CanonicalAITools returns the list of system-supported AI tools for administration.
func CanonicalAITools() []AIToolDef {
	return []AIToolDef{
		{
			Role:         gateway.RoleMatchVendorImport,
			NameAR:       "مطابقة استيراد المورد",
			Description:  "مطابقة بنود ملفات الأصناف المرفوعة من الموردين مع الكتالوج الموحد (/vendor/ingest)",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleMatchSavingProducts,
			NameAR:       "مطابقة منتجات التوفير",
			Description:  "مطابقة وتحديد بدائل التوفير المتاحة في عروض الموردين والصيدليات",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleMatchSmartOrder,
			NameAR:       "الطلب الذكي بالذكاء الاصطناعي",
			Description:  "تحليل ومطابقة نواقص الطلب الذكي للصيدليات واقتراح أفضل الموردين (/customer/smart-order)",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleMatchAdminCatalog,
			NameAR:       "استيراد دليل الإدارة",
			Description:  "مطابقة وتدقيق استيراد الكتالوج العام للمنصة بواسطة مسؤولي النظام (/admin/products/import)",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleMatching,
			NameAR:       "المطابقة العامة الافتراضية",
			Description:  "النموذج المعتمد لجميع مهام المطابقة في حال عدم تعيين نموذج مخصص للأداة",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RolePrimary,
			NameAR:       "المساعد الذكي الأساسي",
			Description:  "محادثات المساعد الذكي مع الصيدليات والموردين وقراءة الاستفسارات والصور",
			DefaultModel: "gemma-4-31b-it",
		},
		{
			Role:         gateway.RoleAttachment,
			NameAR:       "تحليل مرفقات المساعد",
			Description:  "معالجة الملفات والمستندات والجداول المرفقة في محادثات المساعد الذكي",
			DefaultModel: "gemma-4-31b-it",
		},
		{
			Role:         gateway.RoleTranscribe,
			NameAR:       "التفريغ الصوتي للمحادثات",
			Description:  "تحويل الرسائل الصوتية في المحادثات إلى نصوص دقيقة",
			DefaultModel: "whisper-large-v3-turbo",
		},
		{
			Role:         gateway.RoleColumns,
			NameAR:       "كشف أعمدة الملفات",
			Description:  "التعرف الذكي التلقائي على أعمدة ملفات إكسل وCSV أثناء الاستيراد",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleExpand,
			NameAR:       "توسيع البحث بالمرادفات",
			Description:  "توليد مرادفات وأسماء بديلة للبحث عن الأدوية والمستلزمات",
			DefaultModel: "qwen3.7-flash",
		},
		{
			Role:         gateway.RoleClassify,
			NameAR:       "تصنيف تذاكر الدعم",
			Description:  "التصنيف والتحليل التلقائي لطلبات الدعم الفني والشكاوى",
			DefaultModel: "qwen3.7-flash",
		},
	}
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

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", "يجب تحديد أداة / دور الذكاء الاصطناعي")
		return
	}

	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", "يجب تحديد اسم النموذج")
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
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", h.safeMessage(err, lang))
		return
	}

	if h.gatewayKeys != nil {
		h.gatewayKeys.Invalidate()
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "role": role, "model": model})
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "success", "تم حفظ إعدادات النموذج للأداة المحددة بنجاح")
}
