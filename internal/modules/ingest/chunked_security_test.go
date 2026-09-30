package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

type recordingChunkStorage struct {
	err     error
	payload []byte
	started chan struct{}
	release chan struct{}
}

func (*recordingChunkStorage) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}
func (s *recordingChunkStorage) Put(_ context.Context, _ string, reader io.Reader, _ int64, _ string) error {
	if s.started != nil {
		close(s.started)
		<-s.release
	}
	if s.err != nil {
		return s.err
	}
	var err error
	s.payload, err = io.ReadAll(reader)
	return err
}

func isolateChunkDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, directory)
	}
	return directory
}

func chunkService(storage *recordingChunkStorage) (*ingest.Service, *mockIngestRepo) {
	repo := newMockIngestRepo()
	svc := ingest.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetStorage(storage)
	return svc, repo
}

func TestChunkStorageFailurePreservesUploadForRetry(t *testing.T) {
	temp := isolateChunkDirectory(t)
	storage := &recordingChunkStorage{err: errors.New("storage offline")}
	svc, repo := chunkService(storage)
	ctx := database.WithTenant(context.Background(), 10)
	if _, err := svc.UploadChunk(ctx, 12, "retry", "catalog.csv", 0, 2, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UploadChunk(ctx, 12, "retry", "catalog.csv", 1, 2, []byte("second")); !errors.Is(err, storage.err) {
		t.Fatalf("storage error lost: %v", err)
	}
	if len(repo.uploads) != 0 {
		t.Fatal("failed storage reported as completed upload")
	}
	status, err := svc.GetChunkStatus(ctx, "retry")
	if err != nil || len(status) != 2 {
		t.Fatalf("retry chunks lost: %v, %v", status, err)
	}
	entries, err := os.ReadDir(filepath.Join(temp, "dawa24_chunks", "org_10", "retry"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("failed upload left an assembled temporary file: %v, %v", entries, err)
	}
	storage.err = nil
	result, err := svc.UploadChunk(ctx, 12, "retry", "catalog.csv", 1, 2, []byte("second"))
	if err != nil || !result.Completed || string(storage.payload) != "firstsecond" {
		t.Fatalf("retry failed: %+v, %v", result, err)
	}
}

func TestChunkTotalSizeLimitBeforeWrite(t *testing.T) {
	temp := isolateChunkDirectory(t)
	directory := filepath.Join(temp, "dawa24_chunks", "org_10", "large")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for index := range 20 {
		file, err := os.Create(filepath.Join(directory, fmt.Sprintf("chunk_%d", index)))
		if err != nil {
			t.Fatal(err)
		}
		err = file.Truncate(10 << 20)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	svc, repo := chunkService(&recordingChunkStorage{})
	ctx := database.WithTenant(context.Background(), 10)
	if _, err := svc.UploadChunk(ctx, 12, "large", "catalog.csv", 20, 21, []byte("x")); err == nil {
		t.Fatal("upload past200MB accepted")
	}
	if _, err := os.Stat(filepath.Join(directory, "chunk_20")); !os.IsNotExist(err) {
		t.Fatal("oversized upload chunk was written")
	}
	if len(repo.uploads) != 0 {
		t.Fatal("oversized upload registered")
	}
}

func TestIndependentChunkUploadsDoNotShareStorageLock(t *testing.T) {
	isolateChunkDirectory(t)
	blockedStorage := &recordingChunkStorage{started: make(chan struct{}), release: make(chan struct{})}
	blocked, _ := chunkService(blockedStorage)
	other, _ := chunkService(&recordingChunkStorage{})
	ctx := database.WithTenant(context.Background(), 10)
	finished := make(chan error, 1)
	go func() {
		_, err := blocked.UploadChunk(ctx, 12, "blocked", "catalog.csv", 0, 1, []byte("x"))
		finished <- err
	}()
	select {
	case <-blockedStorage.started:
	case <-time.After(time.Second):
		t.Fatal("storage did not start")
	}
	independent := make(chan error, 1)
	go func() {
		_, err := other.UploadChunk(ctx, 12, "other", "catalog.csv", 0, 1, []byte("y"))
		independent <- err
	}()
	select {
	case err := <-independent:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("unrelated upload blocked by global storage lock")
	}
	close(blockedStorage.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
