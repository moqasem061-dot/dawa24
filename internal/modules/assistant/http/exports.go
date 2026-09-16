package http

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// ExportReader loads a stored export by its download token.
type ExportReader interface {
	LoadExport(ctx context.Context, token string) (*assistant.Export, error)
}

// SetExports installs the export store.
func (h *Handler) SetExports(e ExportReader) { h.exports = e }

// DownloadExport serves a file export_data produced.
//
// The token is unguessable, but it is not the authority: the file is served
// only to the user it was generated for, in the organisation it was generated
// in. A link pasted to a colleague — or to a different منشأة of the same
// user — answers 404, the same as a token that never existed.
func (h *Handler) DownloadExport(w http.ResponseWriter, r *http.Request) {
	actor, _ := authctx.From(r.Context())
	if h.exports == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}
	file, err := h.exports.LoadExport(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		h.log.ErrorContext(r.Context(), "assistant: load export", "error", err)
		writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
		return
	}
	if file == nil || file.UserID != actor.UserID || file.OrganizationID != actor.OrgID {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}
	WriteExport(w, file)
}

// WriteExport writes a file as a download. Shared with the Telegram bridge.
func WriteExport(w http.ResponseWriter, file *assistant.Export) {
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Content)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}
