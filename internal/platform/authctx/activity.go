package authctx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
)

// ActivityAudit records request outcomes separately from transactional entity changes.
// The payload is allowlisted: passwords, tokens, document contents and arbitrary JSON never enter the log.
func ActivityAudit(record func(context.Context, database.AuditEntry) error, trustedHops int, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, authenticated := From(r.Context())
			if !authenticated || actor.UserID <= 0 || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			ctx := database.WithAuditActorID(r.Context(), actor.UserID)
			ctx = database.WithAuditClientIP(ctx, httpx.ClientIP(r, trustedHops))
			r = r.WithContext(ctx)
			response := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()
			next.ServeHTTP(response, r)
			entry := operationAuditEntry(r, response, started)
			if record != nil {
				if err := record(ctx, entry); err != nil {
					log.ErrorContext(ctx, "record operation audit", "route", entry.EntityType, "error", err)
				}
			}
		})
	}
}

func operationAuditEntry(r *http.Request, response middleware.WrapResponseWriter, started time.Time) database.AuditEntry {
	actor := FromContext(r.Context())
	route := activityRoute(r)
	status := response.Status()
	if status == 0 {
		status = http.StatusOK
	}
	return database.AuditEntry{
		OrganizationID: activityOrganization(actor), ActorUserID: actor.UserID,
		Action: "http.operation", EntityType: route, EntityID: activityEntityID(r),
		After: map[string]any{"method": r.Method, "route": route, "status": status,
			"outcome": activityOutcome(status, response.Header()), "duration_ms": time.Since(started).Milliseconds(),
			"parameters": activityParameters(r), "user_agent": boundedAuditText(r.UserAgent()), "request_id": observability.RequestIDFrom(r.Context())},
	}
}

func activityRoute(r *http.Request) string {
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		if route := routeContext.RoutePattern(); route != "" {
			return route
		}
	}
	return r.URL.Path
}

func activityOrganization(actor Actor) *int64 {
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 {
		return nil
	}
	return &orgID
}

func activityEntityID(r *http.Request) string {
	for _, key := range []string{"id", "shipmentID", "sessionID", "rid", "rowID", "branchID"} {
		if id := chi.URLParam(r, key); id != "" {
			return id
		}
	}
	return ""
}

func activityParameters(r *http.Request) map[string][]string {
	parameters := make(map[string][]string)
	remaining, count := 4096, 0
	for _, key := range []string{"id", "product_id", "variant_id", "offer_id", "vendor_org_id", "branch_id", "warehouse_id", "request_id", "row_ids", "qty", "quantity", "price", "discount", "status", "mode", "document_type"} {
		for _, submitted := range r.Form[key] {
			if count == 100 || remaining == 0 {
				parameters["_truncated"] = []string{"true"}
				return parameters
			}
			bounded := auditTextLimit(submitted, min(512, remaining))
			parameters[key] = append(parameters[key], bounded)
			remaining -= len([]rune(bounded))
			count++
		}
	}
	return parameters
}

func boundedAuditText(s string) string {
	return auditTextLimit(s, 512)
}

func auditTextLimit(s string, limit int) string {
	count := 0
	for offset := range s {
		if count == limit {
			return s[:offset]
		}
		count++
	}
	return s
}

func activityOutcome(status int, headers http.Header) string {
	if status >= 400 {
		return "rejected"
	}
	for _, key := range []string{"Location", "HX-Redirect"} {
		if redirect, err := url.Parse(headers.Get(key)); err == nil {
			if strings.EqualFold(redirect.Query().Get("notice"), "error") || strings.EqualFold(redirect.Query().Get("notice_type"), "error") {
				return "rejected"
			}
		}
	}
	return fmt.Sprintf("response_%d", status)
}
