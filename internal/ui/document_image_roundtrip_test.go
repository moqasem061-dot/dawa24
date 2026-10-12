package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type roundTripDocumentRepo struct{ *mockAttachmentsRepo }

func (r *roundTripDocumentRepo) Create(_ context.Context, doc *attachments.Document) (*attachments.Document, error) {
	doc.ID = int64(len(r.docs) + 1)
	// Persist and reload metadata, rather than relying on the original upload object.
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var stored attachments.Document
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, err
	}
	r.docs[stored.ID] = &stored
	return &stored, nil
}

func TestOfficialDocumentImageUploadAndReload(t *testing.T) {
	t.Setenv("UPLOAD_DIR", t.TempDir())
	repo := &roundTripDocumentRepo{newMockAttachmentsRepo()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &UIHandler{attSvc: attachments.NewService(repo, nil, logger), log: logger}
	actor := authctx.Actor{UserID: 7, OrganizationID: 10, OrgID: 10, Role: "vendor"}
	ctx := authctx.WithActor(context.Background(), actor)
	for index, originalName := range []string{"images (1).png", "replacement.png"} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("document_type", "commercial_register"); err != nil {
			t.Fatal(err)
		}
		file, err := writer.CreateFormFile("file", originalName)
		if err != nil {
			t.Fatal(err)
		}
		picture := image.NewRGBA(image.Rect(0, 0, 1300, 100))
		draw.Draw(picture, picture.Bounds(), image.White, image.Point{}, draw.Src)
		if err := png.Encode(file, picture); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		upload := httptest.NewRequestWithContext(ctx, http.MethodPost, "/documents/upload", &body)
		upload.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		h.OrganizationDocumentsUploadSubmit(response, upload)
		if response.Code != http.StatusSeeOther || len(repo.docs) != index+1 {
			t.Fatalf("upload: %d %s", response.Code, response.Header().Get("Location"))
		}
		id := strconv.Itoa(len(repo.docs))
		if repo.docs[int64(index+1)].OriginalName != originalName {
			t.Fatal("preview metadata belongs to the previous document")
		}
		router := chi.NewRouter()
		router.Get("/documents/{id}/view", h.DocumentViewHandler)
		preview := httptest.NewRecorder()
		router.ServeHTTP(preview, httptest.NewRequestWithContext(ctx, http.MethodGet, "/documents/"+id+"/view", nil))
		if preview.Code != http.StatusOK {
			t.Fatalf("preview after reload: %d %s", preview.Code, preview.Body.String())
		}
		decoded, _, err := image.Decode(bytes.NewReader(preview.Body.Bytes()))
		if err != nil {
			t.Fatalf("document image cannot decode: %v", err)
		}
		if decoded.Bounds().Dx() == 0 {
			t.Fatal("empty document image")
		}
		if preview.Header().Get("Content-Type") != http.DetectContentType(preview.Body.Bytes()) {
			t.Fatalf("wrong stored-image content type: %s", preview.Header().Get("Content-Type"))
		}
	}
}
