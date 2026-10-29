package compare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

const compareCacheTTL = 10 * time.Minute

// multiSupplierCacheKey generates a deterministic cache key derived from file IDs and their updated_at timestamps.
// If any file's rows or metadata are modified, its updated_at changes, automatically invalidating the cache.
func (s *Service) multiSupplierCacheKey(files []*CompareFile) string {
	if len(files) == 0 {
		return ""
	}
	sorted := make([]*CompareFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	h := sha256.New()
	for _, f := range sorted {
		if f == nil {
			continue
		}
		fmt.Fprintf(h, "%d:%d;", f.ID, f.UpdatedAt.UnixNano())
	}
	return "compare:multi:" + hex.EncodeToString(h.Sum(nil))
}

func (s *Service) getCachedMultiSupplier(ctx context.Context, files []*CompareFile) (*ComparisonResultSet, string) {
	if s.cache == nil || len(files) == 0 {
		return nil, ""
	}
	key := s.multiSupplierCacheKey(files)
	if key == "" {
		return nil, ""
	}
	var cached ComparisonResultSet
	if err := s.cache.GetJSON(ctx, key, &cached); err == nil && len(cached.Rows) > 0 {
		return &cached, key
	}
	return nil, key
}

func (s *Service) setCachedMultiSupplier(ctx context.Context, key string, res *ComparisonResultSet) {
	if s.cache == nil || key == "" || res == nil {
		return
	}
	_ = s.cache.SetJSON(ctx, key, res, compareCacheTTL)
}

func (s *Service) headToHeadCacheKey(source, target *CompareFile, filter HeadToHeadFilter) string {
	if source == nil || target == nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "src:%d:%d;tgt:%d:%d;q:%s;",
		source.ID, source.UpdatedAt.UnixNano(),
		target.ID, target.UpdatedAt.UnixNano(),
		filter.Query,
	)
	if filter.MinPrice != nil {
		fmt.Fprintf(h, "minp:%s;", filter.MinPrice.String())
	}
	if filter.MaxPrice != nil {
		fmt.Fprintf(h, "maxp:%s;", filter.MaxPrice.String())
	}
	if filter.MinDiscount != nil {
		fmt.Fprintf(h, "mind:%f;", *filter.MinDiscount)
	}
	if filter.MaxDiscount != nil {
		fmt.Fprintf(h, "maxd:%f;", *filter.MaxDiscount)
	}
	if filter.Outcome != nil {
		fmt.Fprintf(h, "out:%s;", *filter.Outcome)
	}
	return "compare:h2h:" + hex.EncodeToString(h.Sum(nil))
}

func (s *Service) getCachedHeadToHead(ctx context.Context, source, target *CompareFile, filter HeadToHeadFilter) (*HeadToHeadComparisonResult, string) {
	if s.cache == nil || source == nil || target == nil {
		return nil, ""
	}
	key := s.headToHeadCacheKey(source, target, filter)
	if key == "" {
		return nil, ""
	}
	var cached HeadToHeadComparisonResult
	if err := s.cache.GetJSON(ctx, key, &cached); err == nil && (len(cached.Rows) > 0 || cached.TotalShared > 0) {
		return &cached, key
	}
	return nil, key
}

func (s *Service) setCachedHeadToHead(ctx context.Context, key string, res *HeadToHeadComparisonResult) {
	if s.cache == nil || key == "" || res == nil {
		return
	}
	_ = s.cache.SetJSON(ctx, key, res, compareCacheTTL)
}
