package postgres

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// systemCtx marks a sweep as exempt from row-level security.
//
// It is a named function rather than an inline database.AsSystem call so the
// two places that legitimately cross tenants — the retention sweeps — read as
// what they are, and so a grep for AsSystem in this package lands on a comment
// explaining why rather than on a bare call.
func systemCtx(ctx context.Context) context.Context {
	return database.AsSystem(ctx)
}

// ownCtx scopes a read or write of the assistant's OWN tables.
//
// It exists because platform staff have no organisation. The assistant tables
// carry a RESTRICTIVE policy of the form "is_system() OR (organization_id IS
// NOT NULL AND tenant_visible(...))", so a caller with no tenant matches
// neither branch and is refused everything — including the INSERT that opens
// their conversation. The admin agent could not have held a single conversation.
//
// A tenant caller therefore stays fully under row-level security, exactly as
// before. A caller with no tenant is platform staff, and their rows are scoped
// instead by an explicit user_id predicate that every query in this package
// carries (see GetOwnedConversation, GetTurn, GetAttachment). That is a
// deliberate, narrow hole: it applies only to the assistant's own bookkeeping,
// never to business data, where a staff read goes through requireStaff and its
// own explicit AsSystem in read_admin.go.
func ownCtx(ctx context.Context) context.Context {
	if _, ok := database.TenantFrom(ctx); ok {
		return ctx
	}
	return database.AsSystem(ctx)
}

// logAuditFailure records that the audit trail itself failed.
//
// A missing audit row is worth knowing about — it is the record of who asked
// for what — but it is never worth failing a read the caller was entitled to
// make, so this is the loudest thing that happens.
func (r *Repository) logAuditFailure(ctx context.Context, entry assistant.ToolAudit, err error) {
	slog.ErrorContext(ctx, "assistant tool audit write failed",
		"tool", entry.ToolName,
		"decision", entry.Decision,
		"user_id", entry.UserID,
		"org_id", entry.OrganizationID,
		"error", err)
}
