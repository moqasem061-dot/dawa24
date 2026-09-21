package ui

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

func TestURLDownloadCache_Deduplication(t *testing.T) {
	var downloadCalls int64

	cache := &urlDownloadCache{
		inProg: make(map[string]*urlDownloadFlight),
		fetcher: func(ctx context.Context, imageURL, uploadDir string, storageClient *storage.Client) (string, error) {
			atomic.AddInt64(&downloadCalls, 1)
			time.Sleep(40 * time.Millisecond) // simulate network download + watermark + compress
			return "/uploads/products/dedup-test.png", nil
		},
	}

	tempDir := t.TempDir()
	const concurrentCallers = 15
	var wg sync.WaitGroup
	wg.Add(concurrentCallers)

	results := make([]string, concurrentCallers)
	errs := make([]error, concurrentCallers)

	for i := 0; i < concurrentCallers; i++ {
		go func(idx int) {
			defer wg.Done()
			path, err := cache.FetchOrDownload(context.Background(), "https://example.com/panadol.png", tempDir, nil)
			results[idx] = path
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	// 1. All concurrent callers must succeed
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d failed with error: %v", i, err)
		}
	}

	// 2. All callers must receive the exact same path
	for i, p := range results {
		if p != "/uploads/products/dedup-test.png" {
			t.Errorf("caller %d got path %s, expected /uploads/products/dedup-test.png", i, p)
		}
	}

	// 3. Download should happen only ONCE despite 15 concurrent callers!
	if calls := atomic.LoadInt64(&downloadCalls); calls != 1 {
		t.Errorf("expected exactly 1 download call for deduplicated URL, got %d", calls)
	}
}
