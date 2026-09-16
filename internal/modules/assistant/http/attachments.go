package http

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// resolveAttachments delegates attachment conversion, tabular extraction,
// and multimodal packaging to the assistant service.
func (h *Handler) resolveAttachments(
	ctx context.Context, actor authctx.Actor, refs []string,
) ([]assistant.Attachment, []assistant.AttachmentDigest, []gateway.ContentPart) {
	return h.svc.ResolveAttachments(ctx, actor, refs)
}
