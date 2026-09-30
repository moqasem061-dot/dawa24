package ingest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

const maxChunkBytes = 10 << 20
const maxChunkUploadBytes int64 = 200 << 20

type chunkStorage interface {
	Put(context.Context, string, io.Reader, int64, string) error
}

type uploadChunkLock struct {
	mu    sync.Mutex
	users int
}

var chunkLocks = struct {
	sync.Mutex
	entries map[string]*uploadChunkLock
}{entries: make(map[string]*uploadChunkLock)}

// Locks cover one upload's disk writes and completion. A slow storage request
// must not block every other tenant's upload, and completed locks do not accumulate.
func lockChunkUpload(directory string) func() {
	chunkLocks.Lock()
	lock := chunkLocks.entries[directory]
	if lock == nil {
		lock = &uploadChunkLock{}
		chunkLocks.entries[directory] = lock
	}
	lock.users++
	chunkLocks.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		chunkLocks.Lock()
		lock.users--
		if lock.users == 0 {
			delete(chunkLocks.entries, directory)
		}
		chunkLocks.Unlock()
	}
}

func checkChunkUploadSize(directory, replacing string, incoming int64) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	total := incoming
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "chunk_") || filepath.Join(directory, entry.Name()) == replacing {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
	}
	if total > maxChunkUploadBytes {
		return apperr.Validation("chunk.too_large", i18n.TDefault("chunk.too_large"), nil)
	}
	return nil
}

func assembleUploadChunks(directory string, count int) (*os.File, int64, error) {
	combined, err := os.CreateTemp(directory, "assembled-*")
	if err != nil {
		return nil, 0, err
	}
	var total int64
	for index := range count {
		copied, err := appendUploadChunk(combined, filepath.Join(directory, fmt.Sprintf("chunk_%d", index)))
		total += copied
		if err != nil || copied > maxChunkBytes || total > maxChunkUploadBytes {
			_ = combined.Close()
			_ = os.Remove(combined.Name())
			if err != nil {
				return nil, 0, err
			}
			return nil, 0, apperr.Validation("chunk.too_large", i18n.TDefault("chunk.too_large"), nil)
		}
	}
	if _, err := combined.Seek(0, io.SeekStart); err != nil {
		_ = combined.Close()
		_ = os.Remove(combined.Name())
		return nil, 0, err
	}
	return combined, total, nil
}

func appendUploadChunk(combined *os.File, chunkPath string) (int64, error) {
	chunk, err := os.Open(chunkPath)
	if err != nil {
		return 0, err
	}
	defer chunk.Close()
	return io.Copy(combined, io.LimitReader(chunk, maxChunkBytes+1))
}
