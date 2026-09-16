package http

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes ingest HTTP endpoints.
type Handler struct {
	service *ingest.Service
	log     *slog.Logger
}

// NewHandler creates an ingest HTTP handler.
func NewHandler(service *ingest.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers ingest endpoints on a Chi router.
//
// Everything that starts, changes or commits an import takes vendor.ingest.run,
// the same permission the HTML wizard requires. Without it, an approved
// organisation's ordinary member could commit a price list into their company's
// catalogue over JSON while being unable to open the screen that does it.
//
// Reads take vendor.ingest.view, which vendor.ingest.run implies.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.ingest.view", "vendor.ingest.run", "ingest.admin"))
		g.Get("/api/v1/ingest/uploads/chunk/status", h.GetChunkStatus)
		g.Get("/api/v1/ingest/sessions", h.ListSessions)
		g.Get("/api/v1/ingest/sessions/{id}", h.GetSession)
		g.Get("/api/v1/ingest/sessions/{id}/rows", h.ListRows)
		g.Get("/api/v1/ingest/sessions/{id}/events", h.StreamEvents)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.ingest.run", "ingest.admin"))
		g.Post("/api/v1/ingest/uploads/presign", h.PresignUpload)
		g.Post("/api/v1/ingest/uploads/chunk", h.UploadChunk)
		g.Post("/api/v1/ingest/uploads", h.RegisterUpload)
		g.Post("/api/v1/ingest/sessions", h.StartSession)
		g.Post("/api/v1/ingest/sessions/{id}/mapping", h.UpdateMapping)
		g.Post("/api/v1/ingest/sessions/{id}/commit", h.CommitSession)
		g.Post("/api/v1/ingest/sessions/{id}/cancel", h.CancelSession)
		g.Put("/api/v1/ingest/sessions/{id}/rows/{rid}", h.OverrideRowMatch)
	})

	h.RegisterAdminRoutes(r)
}

type PresignUploadRequest struct {
	Filename      string `json:"filename"`
	MimeType      string `json:"mime_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

// PresignUpload returns a presigned S3/MinIO upload URL for direct browser uploads.
func (h *Handler) PresignUpload(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var req PresignUploadRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	res, err := h.service.PresignUpload(r.Context(), userID, req.Filename, req.MimeType, req.FileSizeBytes)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, res)
}

// UploadChunk receives one part of a multipart chunked upload.
func (h *Handler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	// Limit each chunk to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("chunk.size", "Chunk size too large", nil))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("file.required", "Chunk file part is required", nil))
		return
	}
	defer file.Close()

	chunkBytes, err := io.ReadAll(file)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("file.read", "Failed to read chunk", nil))
		return
	}

	fileUUID := r.FormValue("file_uuid")
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	chunkIdx, _ := strconv.Atoi(r.FormValue("chunk_index"))
	totalChunks, _ := strconv.Atoi(r.FormValue("total_chunks"))

	res, err := h.service.UploadChunk(r.Context(), userID, fileUUID, filename, chunkIdx, totalChunks, chunkBytes)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	status := http.StatusOK
	if res.Completed {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, res)
}

// GetChunkStatus returns list of uploaded chunk indices for resumption.
func (h *Handler) GetChunkStatus(w http.ResponseWriter, r *http.Request) {
	fileUUID := r.URL.Query().Get("file_uuid")
	if fileUUID == "" {
		httpx.Error(w, r, h.log, apperr.Validation("file_uuid.required", "File UUID is required", nil))
		return
	}

	present, err := h.service.GetChunkStatus(r.Context(), fileUUID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"file_uuid":       fileUUID,
		"uploaded_chunks": present,
		"count":           len(present),
	})
}

// RegisterUpload registers a file uploaded to S3/MinIO.
func (h *Handler) RegisterUpload(w http.ResponseWriter, r *http.Request) {
	var f ingest.FileUpload
	if err := httpx.DecodeJSON(w, r, &f); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.RegisterUpload(r.Context(), &f)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

type StartSessionRequest struct {
	FileUploadID int64    `json:"file_upload_id"`
	Headers      []string `json:"headers"`
	MinScore     float64  `json:"min_score"`
}

// StartSession creates a session and runs column detection.
func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	var req StartSessionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	session, err := h.service.StartSession(r.Context(), req.FileUploadID, req.Headers, req.MinScore)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, session)
}

// GetSession retrieves the progress of an import session.
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	session, err := h.service.GetSessionProgress(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, session)
}
