package http

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
)

// resolveAttachments delegates attachment conversion, tabular extraction,
// and multimodal packaging to the assistant service.
func (h *Handler) resolveAttachments(
	ctx context.Context, actor authctx.Actor, refs []string,
) ([]assistant.Attachment, []assistant.AttachmentDigest, []gateway.ContentPart) {
	return h.svc.ResolveAttachments(ctx, actor, refs)
}
