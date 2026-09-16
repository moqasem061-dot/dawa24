package catalog

import (
	"sync"
	"time"
)

// A short-lived hold on the decoded workbook.
//
// The mapping screen is interactive: the admin rebinds a column, asks for the
// preview again, rebinds another. Every one of those was previously a read of
// the whole upload out of the session row followed by a full decode of the
// workbook — for the nine-thousand-row distributor export that is roughly a
// second and a half per keystroke-driven refresh, which is the entire reason
// the screen felt broken.
//
// Decoding once and holding the result for a few minutes turns that into a map
// lookup. It is a cache of something immutable — the bytes on the session row
// never change — so there is nothing to invalidate; entries simply expire.

// sheetCacheTTL is how long a decoded sheet stays warm. Long enough to cover an
// admin working through the mapping screen, short enough that an abandoned
// session's rows are not held for the rest of the process's life.
const sheetCacheTTL = 15 * time.Minute

// sheetCacheLimit caps how many sessions are held at once. Two admins importing
// simultaneously is the realistic peak; the limit exists so a scripted caller
// cannot pin an unbounded number of workbooks in memory.
const sheetCacheLimit = 4

type cachedSheet struct {
	data     *SheetData
	cachedAt time.Time
}

// sheetCache holds decoded workbooks by session public id.
type sheetCache struct {
	mu      sync.Mutex
	entries map[string]cachedSheet
}

func newSheetCache() *sheetCache { return &sheetCache{entries: map[string]cachedSheet{}} }

// get returns a warm sheet, or nil when there is none.
func (c *sheetCache) get(key string) *SheetData {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}
	if time.Since(entry.cachedAt) > sheetCacheTTL {
		delete(c.entries, key)
		return nil
	}
	return entry.data
}

// put stores a decoded sheet, evicting the oldest entry when full.
func (c *sheetCache) put(key string, data *SheetData) {
	if c == nil || data == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictLocked()
	c.entries[key] = cachedSheet{data: data, cachedAt: time.Now()}
}

// drop forgets one session, called when it is committed or cancelled and the
// bytes behind it are about to be released.
func (c *sheetCache) drop(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// evictLocked removes expired entries, then the oldest one if the cache is
// still at its limit.
func (c *sheetCache) evictLocked() {
	oldestKey, oldestAt := "", time.Now()
	for key, entry := range c.entries {
		if time.Since(entry.cachedAt) > sheetCacheTTL {
			delete(c.entries, key)
			continue
		}
		if entry.cachedAt.Before(oldestAt) {
			oldestKey, oldestAt = key, entry.cachedAt
		}
	}
	if len(c.entries) >= sheetCacheLimit && oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
