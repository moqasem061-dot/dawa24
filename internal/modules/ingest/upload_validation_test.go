package ingest_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestRegisterUploadBindsStorageAndUserToActor(t *testing.T) {
	ctx := database.WithTenant(authctx.WithActor(context.Background(), authctx.Actor{UserID: 12, OrgID: 10}), 10)
	for _, key := range []string{"../../etc/passwd", "orgs/11/uploads/private.pdf", "orgs/10/uploads/../private.pdf", "orgs/10/uploads/"} {
		t.Run(key, func(t *testing.T) {
			repo := newMockIngestRepo()
			svc := ingest.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
			_, err := svc.RegisterUpload(ctx, &ingest.FileUpload{Filename: "catalog.csv", StorageKey: key, UserID: 99})
			if err == nil || len(repo.uploads) != 0 {
				t.Fatal("invalid storage key accepted")
			}
		})
	}
	repo := newMockIngestRepo()
	svc := ingest.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	upload, err := svc.RegisterUpload(ctx, &ingest.FileUpload{Filename: "كتالوج.csv", StorageKey: "orgs/10/uploads/catalog.csv", UserID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if upload.UserID != 12 || upload.OrganizationID != 10 {
		t.Fatalf("untrusted owner retained: %+v", upload)
	}
	_, err = svc.RegisterUpload(ctx, &ingest.FileUpload{Filename: "../secret.pdf", StorageKey: "orgs/10/uploads/catalog.csv"})
	if err == nil {
		t.Fatal("path-bearing original filename accepted")
	}
}
