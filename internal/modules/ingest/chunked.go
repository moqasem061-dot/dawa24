package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

var safeUUIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// maxUploadChunks bounds one chunked upload: 200 MB in chunks of at least a
// few MB never needs more.
const maxUploadChunks = 64

// ChunkUploadResult models the response returned after receiving a chunk.
type ChunkUploadResult struct {
	Completed    bool   `json:"completed"`
	ChunkIndex   int    `json:"chunk_index"`
	TotalChunks  int    `json:"total_chunks"`
	FileUploadID int64  `json:"file_upload_id,omitempty"`
	PublicID     string `json:"public_id,omitempty"`
	Filename     string `json:"filename,omitempty"`
	StorageKey   string `json:"storage_key,omitempty"`
}

// UploadChunk receives, writes, and reassembles uploaded file pieces.
func (s *Service) UploadChunk(
	ctx context.Context,
	userID int64,
	fileUUID string,
	filename string,
	chunkIndex int,
	totalChunks int,
	chunkData []byte,
) (*ChunkUploadResult, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	if !safeUUIDRe.MatchString(fileUUID) {
		return nil, apperr.Validation("file_uuid.invalid", "Invalid file UUID", nil)
	}
	// Chunks are at most 10 MB and an upload at most 200 MB; an unbounded count
	// let one caller keep writing chunk files to the shared temp volume.
	if chunkIndex < 0 || totalChunks <= 0 || totalChunks > maxUploadChunks || chunkIndex >= totalChunks {
		return nil, apperr.Validation("chunk.invalid", "Invalid chunk parameters", nil)
	}
	if len(chunkData) == 0 {
		return nil, apperr.Validation("chunk.empty", "Chunk data cannot be empty", nil)
	}
	if len(chunkData) > maxChunkBytes {
		return nil, apperr.Validation("chunk.too_large", i18n.TDefault("chunk.too_large"), nil)
	}
	if filename == "" {
		filename = "upload.csv"
	}
	if err := validateUploadFilename(filename); err != nil {
		return nil, err
	}
	putter, ok := s.storage.(chunkStorage)
	if !ok {
		return nil, apperr.Unavailable("storage", nil)
	}

	tempDir := filepath.Join(os.TempDir(), "dawa24_chunks", fmt.Sprintf("org_%d", orgID), fileUUID)
	unlock := lockChunkUpload(tempDir)
	defer unlock()
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp chunk dir: %w", err)
	}

	chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d", chunkIndex))
	if err := checkChunkUploadSize(tempDir, chunkPath, int64(len(chunkData))); err != nil {
		return nil, err
	}
	if err := os.WriteFile(chunkPath, chunkData, 0644); err != nil {
		return nil, fmt.Errorf("write chunk file: %w", err)
	}

	// Check if all chunks (0..totalChunks-1) exist
	allPresent := true
	for i := 0; i < totalChunks; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("chunk_%d", i))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			allPresent = false
			break
		}
	}

	if !allPresent {
		return &ChunkUploadResult{
			Completed:   false,
			ChunkIndex:  chunkIndex,
			TotalChunks: totalChunks,
			Filename:    filename,
		}, nil
	}

	combined, size, err := assembleUploadChunks(tempDir, totalChunks)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = combined.Close()
		_ = os.Remove(combined.Name())
	}()
	key := fmt.Sprintf("orgs/%d/uploads/%d_%s", orgID, time.Now().UnixNano(), filename)
	if err := putter.Put(ctx, key, combined, size, "application/octet-stream"); err != nil {
		return nil, fmt.Errorf("store chunked upload: %w", err)
	}

	f := &FileUpload{
		OrganizationID: orgID,
		UserID:         userID,
		Filename:       filename,
		StorageKey:     key,
		FileSizeBytes:  size,
		MimeType:       "application/octet-stream",
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.repo.CreateFileUpload(ctx, f); err != nil {
		return nil, err
	}
	_ = combined.Close()
	if err := os.RemoveAll(tempDir); err != nil {
		s.log.WarnContext(ctx, "completed upload chunk cleanup failed", "error", err)
	}

	s.log.InfoContext(ctx, "chunked upload complete and reassembled", "upload_id", f.ID, "total_chunks", totalChunks, "bytes", size)

	return &ChunkUploadResult{
		Completed:    true,
		ChunkIndex:   chunkIndex,
		TotalChunks:  totalChunks,
		FileUploadID: f.ID,
		PublicID:     f.PublicID,
		Filename:     filename,
		StorageKey:   key,
	}, nil
}

// GetChunkStatus returns list of uploaded chunk indices for a given file UUID.
func (s *Service) GetChunkStatus(ctx context.Context, fileUUID string) ([]int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	if !safeUUIDRe.MatchString(fileUUID) {
		return nil, apperr.Validation("file_uuid.invalid", "Invalid file UUID", nil)
	}

	tempDir := filepath.Join(os.TempDir(), "dawa24_chunks", fmt.Sprintf("org_%d", orgID), fileUUID)
	entries, err := os.ReadDir(tempDir)
	if os.IsNotExist(err) {
		return []int{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read chunk status: %w", err)
	}

	var present []int
	for _, entry := range entries {
		var idx int
		if n, _ := fmt.Sscanf(entry.Name(), "chunk_%d", &idx); n == 1 {
			present = append(present, idx)
		}
	}

	return present, nil
}
