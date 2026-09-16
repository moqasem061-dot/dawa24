package ingest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

var safeUUIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var chunkMutex sync.Mutex

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
	if chunkIndex < 0 || totalChunks <= 0 || chunkIndex >= totalChunks {
		return nil, apperr.Validation("chunk.invalid", "Invalid chunk parameters", nil)
	}
	if len(chunkData) == 0 {
		return nil, apperr.Validation("chunk.empty", "Chunk data cannot be empty", nil)
	}

	tempDir := filepath.Join(os.TempDir(), "dawa24_chunks", fmt.Sprintf("org_%d", orgID), fileUUID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp chunk dir: %w", err)
	}

	chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d", chunkIndex))
	if err := os.WriteFile(chunkPath, chunkData, 0644); err != nil {
		return nil, fmt.Errorf("write chunk file: %w", err)
	}

	chunkMutex.Lock()
	defer chunkMutex.Unlock()

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

	// All chunks present: Reassemble into a combined byte buffer / stream
	var combined bytes.Buffer
	for i := 0; i < totalChunks; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("chunk_%d", i))
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read chunk %d: %w", i, err)
		}
		combined.Write(data)
	}

	// Clean up temporary chunks directory
	_ = os.RemoveAll(tempDir)

	cleanFilename := filename
	if cleanFilename == "" {
		cleanFilename = "upload.csv"
	}
	key := fmt.Sprintf("orgs/%d/uploads/%d_%s", orgID, time.Now().UnixNano(), cleanFilename)

	// If storage backend is configured, upload the reassembled object
	if s.storage != nil {
		if putter, ok := s.storage.(interface {
			Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
		}); ok {
			_ = putter.Put(ctx, key, bytes.NewReader(combined.Bytes()), int64(combined.Len()), "application/octet-stream")
		}
	}

	f := &FileUpload{
		OrganizationID: orgID,
		UserID:         userID,
		Filename:       cleanFilename,
		StorageKey:     key,
		FileSizeBytes:  int64(combined.Len()),
		MimeType:       "application/octet-stream",
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.repo.CreateFileUpload(ctx, f); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "chunked upload complete and reassembled", "upload_id", f.ID, "total_chunks", totalChunks, "bytes", combined.Len())

	return &ChunkUploadResult{
		Completed:    true,
		ChunkIndex:   chunkIndex,
		TotalChunks:  totalChunks,
		FileUploadID: f.ID,
		PublicID:     f.PublicID,
		Filename:     cleanFilename,
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
