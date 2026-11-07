package ui

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// benchmarkFileOwned reports whether a compare file named in the market
// benchmark query belongs to the caller. Another organization's file is not
// shown, even though the benchmark itself loads files as system (Batch 3, S05).
func (h *UIHandler) benchmarkFileOwned(ctx context.Context, actor authctx.Actor, fileID int64) bool {
	if h.compareSvc == nil {
		return false
	}
	file, err := h.compareSvc.GetFile(ctx, fileID)
	return err == nil && h.checkFileOwnership(actor, file)
}
