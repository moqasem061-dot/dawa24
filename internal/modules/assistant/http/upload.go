package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// maxAttachmentBytes is the per-file ceiling.
const maxAttachmentBytes = 10 << 20

// maxAttachmentsPerTurn bounds how many files one question may carry.
const maxAttachmentsPerTurn = 5

// uploaded is one accepted file, as the composer needs to know it.
type uploaded struct {
	Reference string  `json:"reference"`
	Filename  string  `json:"filename"`
	MIMEType  string  `json:"mime_type"`
	SizeMB    float64 `json:"size_mb"`
	// PreviewURL lets the transcript show the file the server actually kept,
	// rather than a blob: URL that dies with the tab. It is only set for the
	// kinds a browser can render inline.
	PreviewURL string `json:"preview_url,omitempty"`
}

// Upload validates a file, stores its bytes, and returns an opaque reference.
//
// Storage is best-effort with a guaranteed floor. Object storage is used when
// there is one; when there is not, the bytes go to the attachments table
// instead and the upload still succeeds. That is the fix for the oldest and
// most confusing attachment failure in this feature: a deployment with no
// STORAGE_BUCKET answered 503 to every upload, and the drawer reported it as
// "تعذّر رفع الملف" — which reads as the file being wrong rather than as the
// server having nowhere to put it.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if !h.limiter.Allow(actor.UserID) {
		writeFailure(w, http.StatusTooManyRequests, assistant.Fail(assistant.CodeRateLimited))
		return
	}

	// The reader caps the whole body, so a client that lies about Content-Length
	// cannot make the parse allocate past the ceiling. Five files at ten
	// megabytes plus multipart overhead is the widest legitimate request.
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentsPerTurn*maxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeFailure(w, http.StatusRequestEntityTooLarge,
			assistant.Fail(assistant.CodeAttachmentTooLarge))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 || len(files) > maxAttachmentsPerTurn {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	results := make([]uploaded, 0, len(files))
	for _, fh := range files {
		item, status, failure := h.acceptFile(ctx, actor, fh)
		if failure != nil {
			writeFailure(w, status, *failure)
			return
		}
		results = append(results, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{"attachments": results})
}

// acceptFile validates and stores one uploaded file.
func (h *Handler) acceptFile(
	ctx context.Context, actor authctx.Actor, fh *multipart.FileHeader,
) (uploaded, int, *assistant.Failure) {
	tooLarge := assistant.Fail(assistant.CodeAttachmentTooLarge)
	if fh.Size > maxAttachmentBytes {
		return uploaded{}, http.StatusRequestEntityTooLarge, &tooLarge
	}

	f, err := fh.Open()
	if err != nil {
		rejected := assistant.Fail(assistant.CodeAttachmentRejected)
		return uploaded{}, http.StatusBadRequest, &rejected
	}
	content, err := io.ReadAll(io.LimitReader(f, maxAttachmentBytes+1))
	_ = f.Close()
	if err != nil || len(content) > maxAttachmentBytes {
		return uploaded{}, http.StatusRequestEntityTooLarge, &tooLarge
	}

	row, err := h.svc.IngestAttachment(ctx, actor, fh.Filename, content)
	if err != nil {
		code := assistant.CodeAttachmentRejected
		if errors.Is(err, assistant.ErrHEIC) {
			code = assistant.CodeAttachmentHEIC
		}
		f := assistant.Fail(code)
		return uploaded{}, http.StatusBadRequest, &f
	}

	item := uploaded{
		Reference: row.PublicID.String(),
		Filename:  row.Filename,
		MIMEType:  row.MIMEType,
		SizeMB:    float64(row.SizeBytes) / (1024 * 1024),
	}
	if assistant.ClassifyMIME(row.MIMEType) == assistant.KindImage {
		item.PreviewURL = "/api/v1/assistant/attachments/" + item.Reference
	}
	return item, 0, nil
}

// Download serves an attachment back to the person who uploaded it.
//
// It exists so the transcript survives a reload. Previews used to be object
// URLs minted in the browser, which meant reopening a conversation from history
// showed a paperclip and a filename where the photograph had been — the file
// was still on the server, and nothing could ask for it.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	row, err := h.repo.GetAttachment(ctx, chi.URLParam(r, "ref"), actor.OrgID, actor.UserID)
	if err != nil || row == nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}
	content, err := h.svc.AttachmentBytes(ctx, row)
	if err != nil {
		writeFailure(w, http.StatusNotFound, assistant.Fail(assistant.CodeNotFound))
		return
	}

	// nosniff plus an explicit disposition: an uploaded file is untrusted
	// content served from our own origin, and only the kinds a browser renders
	// safely are shown inline. Everything else downloads.
	w.Header().Set("Content-Type", row.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=600")
	if assistant.ClassifyMIME(row.MIMEType) == assistant.KindImage {
		w.Header().Set("Content-Disposition", "inline")
	} else {
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(content)))
	_, _ = w.Write(content)
}
