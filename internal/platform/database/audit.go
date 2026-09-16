package database

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

type auditIPKey struct{}

// WithAuditClientIP attaches the client IP to context for audit attribution.
func WithAuditClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, auditIPKey{}, strings.TrimSpace(ip))
}

// AuditClientIPFrom retrieves the client IP attached to context, if any.
func AuditClientIPFrom(ctx context.Context) string {
	if v, ok := ctx.Value(auditIPKey{}).(string); ok {
		return v
	}
	return ""
}

type auditActorKey struct{}

// WithAuditActorID attaches the actor user ID to context for audit attribution.
func WithAuditActorID(ctx context.Context, actorID int64) context.Context {
	return context.WithValue(ctx, auditActorKey{}, actorID)
}

// AuditActorIDFrom retrieves the actor user ID attached to context, if any.
func AuditActorIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(auditActorKey{}).(int64); ok {
		return v
	}
	return 0
}

// AuditEntry is one row of platform.audit_log.
//
// Before and After hold the entity state either side of the change. They are
// what makes an audit trail answer "what did this actually do", rather than
// only "something happened".
type AuditEntry struct {
	OrganizationID *int64
	ActorUserID    int64
	Action         string
	EntityType     string
	EntityID       string
	Before         any
	After          any
	IP             string
	RequestID      string
}

// WriteAudit records an administrative change.
//
// It takes the transaction rather than opening its own, deliberately: the audit
// row has to commit or roll back with the change it describes. Written in a
// separate transaction it can survive a rolled-back change and claim something
// happened that did not, or be lost while the change lands and leave a
// privileged mutation with no record at all.
func WriteAudit(ctx context.Context, tx pgx.Tx, e AuditEntry) error {
	if e.RequestID == "" {
		e.RequestID = observability.RequestIDFrom(ctx)
	}
	if e.IP == "" {
		e.IP = AuditClientIPFrom(ctx)
	}
	if e.ActorUserID == 0 {
		e.ActorUserID = AuditActorIDFrom(ctx)
	}

	beforeJSON, err := marshalAudit(e.Before)
	if err != nil {
		return fmt.Errorf("audit before: %w", err)
	}
	afterJSON, err := marshalAudit(e.After)
	if err != nil {
		return fmt.Errorf("audit after: %w", err)
	}

	var ipVal *string
	if trimmed := strings.TrimSpace(e.IP); trimmed != "" {
		if parsed := net.ParseIP(trimmed); parsed != nil {
			s := parsed.String()
			ipVal = &s
		}
	}

	const query = `
		INSERT INTO platform.audit_log (
			organization_id, actor_user_id, action, entity_type, entity_id,
			before, after, ip, request_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, NULLIF($9, ''));
	`
	if _, err := tx.Exec(ctx, query,
		e.OrganizationID, e.ActorUserID, e.Action, e.EntityType, e.EntityID,
		beforeJSON, afterJSON, ipVal, e.RequestID,
	); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// marshalAudit renders a value as JSONB, or NULL when there is nothing to say —
// a creation has no before state and a deletion has no after state.
func marshalAudit(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
