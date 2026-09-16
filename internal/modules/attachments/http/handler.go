package http

import (
	"encoding/json"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes REST endpoints for file upload presigning, confirmation, and admin verification.
type Handler struct {
	svc *attachments.Service
	log *slog.Logger
}

// NewHandler creates a new HTTP Handler for attachments.
func NewHandler(svc *attachments.Service, log *slog.Logger) *Handler {
	return &Handler{
		svc: svc,
		log: log,
	}
}

// RegisterRoutes mounts attachment endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/attachments", func(ar chi.Router) {
		ar.Post("/presign", h.presign)
		ar.Post("/{id}/confirm", h.confirm)
		ar.Get("/{id}", h.downloadURL)
		ar.Delete("/{id}", h.delete)
	})

	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("platform.settings.manage"))
		admin.Route("/api/v1/admin/attachments", func(aar chi.Router) {
			aar.Get("/", h.adminList)
			aar.Post("/{id}/verify", h.adminVerify)
		})
	})
}

func (h *Handler) presign(w http.ResponseWriter, r *http.Request) {
	actor := authctx.FromContext(r.Context())
	if actor.UserID == 0 {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var req attachments.PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_244_244"), nil))
		return
	}

	res, err := h.svc.PresignUpload(r.Context(), actor, req)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	actor := authctx.FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_245_245"), nil))
		return
	}

	doc, err := h.svc.ConfirmUpload(r.Context(), actor, id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]interface{}{
		"status":   "confirmed",
		"document": doc,
	})
}

func (h *Handler) downloadURL(w http.ResponseWriter, r *http.Request) {
	actor := authctx.FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_245_245"), nil))
		return
	}

	url, err := h.svc.GetDownloadURL(r.Context(), actor, id)
	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			if appErr.Kind == apperr.KindNotFound {
				httpx.Error(w, r, h.log, apperr.NotFound(i18n.TDefault("w4_mod.s_246_246")))
				return
			}
			if appErr.Kind == apperr.KindForbidden {
				httpx.Error(w, r, h.log, apperr.Forbidden("document.access_denied", i18n.TDefault("w4_mod.s_236_236")))
				return
			}
		}
		httpx.Error(w, r, h.log, apperr.NotFound(i18n.TDefault("w4_mod.s_247_247")))
		return
	}

	if r.Header.Get("Accept") == "application/json" {
		httpx.JSON(w, http.StatusOK, map[string]string{
			"download_url": url,
		})
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	actor := authctx.FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_245_245"), nil))
		return
	}

	if err := h.svc.Delete(r.Context(), actor, id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
	})
}

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := attachments.DocumentFilter{
		Search: q.Get("q"),
	}

	if orgID, err := strconv.ParseInt(q.Get("org_id"), 10, 64); err == nil && orgID > 0 {
		filter.OrganizationID = &orgID
	}
	if userID, err := strconv.ParseInt(q.Get("user_id"), 10, 64); err == nil && userID > 0 {
		filter.UserID = &userID
	}
	if dt := q.Get("type"); dt != "" {
		t := attachments.DocumentType(dt)
		filter.DocumentType = &t
	}
	if st := q.Get("status"); st != "" {
		s := attachments.DocumentStatus(st)
		filter.Status = &s
	}
	if limit, err := strconv.Atoi(q.Get("limit")); err == nil {
		filter.Limit = limit
	}
	if offset, err := strconv.Atoi(q.Get("offset")); err == nil {
		filter.Offset = offset
	}

	docs, total, err := h.svc.ListAll(r.Context(), filter)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]interface{}{
		"documents": docs,
		"total":     total,
	})
}

type verifyRequest struct {
	DocumentType string `json:"document_type"`
	Status       string `json:"status"`
	Notes        string `json:"notes"`
}

func (h *Handler) adminVerify(w http.ResponseWriter, r *http.Request) {
	actor := authctx.FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_245_245"), nil))
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_248_248"), nil))
		return
	}

	status := attachments.DocumentStatus(req.Status)
	if status != attachments.StatusVerified && status != attachments.StatusRejected {
		httpx.Error(w, r, h.log, apperr.Validation("request.invalid", i18n.TDefault("w4_mod.s_249_249"), nil))
		return
	}

	if req.DocumentType != "" {
		if err := h.svc.VerifyDocumentWithType(r.Context(), actor, id, attachments.DocumentType(req.DocumentType), status, req.Notes); err != nil {
			httpx.Error(w, r, h.log, err)
			return
		}
	} else {
		if err := h.svc.VerifyDocument(r.Context(), actor, id, status, req.Notes); err != nil {
			httpx.Error(w, r, h.log, err)
			return
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "updated",
	})
}
