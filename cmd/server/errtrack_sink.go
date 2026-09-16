package main

import (
	"context"
	"log/slog"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/errtrack"
)

// errorLogSink writes captured errors into platform_admin.error_logs, which is
// what the admin diagnostics screen reads.
//
// It lives in cmd/server rather than in either package it joins: errtrack is
// platform code and may not import a business module, and platform_admin has no
// business knowing how the HTTP layer discovers failures. Start-up is where the
// two are allowed to meet.
type errorLogSink struct {
	svc *platformadmin.Service
	log *slog.Logger
}

// Capture stores one event.
//
// database.AsSystem is justified here for the same reason the audit writer uses
// it: an error record belongs to the platform, not to the tenant whose request
// produced it, and the request's own transaction is usually gone by the time
// this runs. Without it a failure inside a tenant context would be written
// under row-level security and then be invisible to the admin who needs it.
func (s *errorLogSink) Capture(ctx context.Context, e errtrack.Event) error {
	if s == nil || s.svc == nil {
		return nil
	}

	entry := &platformadmin.ErrorLog{
		UserID:           e.UserID,
		UserName:         e.UserName,
		UserEmail:        e.UserEmail,
		OrganizationName: e.OrganizationName,
		ErrorLevel:       e.Level,
		ErrorMessage:     truncate(e.Message, 4000),
		ExceptionClass:   e.ExceptionClass,
		StackTrace:       truncate(e.StackTrace, 16000),
		FilePath:         e.FilePath,
		LineNumber:       e.LineNumber,
		HTTPMethod:       e.HTTPMethod,
		URLPath:          truncate(e.URLPath, 1000),
		IPAddress:        e.IPAddress,
		UserAgent:        truncate(e.UserAgent, 500),
		Status:           "NEW",
		CreatedAt:        e.OccurredAt,
	}

	// The payload column holds the shape of the request, never its body: a body
	// can carry a password or a token, and this table is read by staff and
	// kept. Status and request id are what an operator actually correlates on.
	entry.RequestPayload = map[string]any{
		"status_code": e.StatusCode,
		"request_id":  e.RequestID,
	}

	return s.svc.LogError(database.AsSystem(ctx), entry)
}

// truncate keeps a value inside something a table cell can display. A stack
// trace is the only field that is genuinely long, and its first frames are the
// ones that matter.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}

// installErrorTracking wires error capture for the process and returns the
// stop function the caller defers.
func installErrorTracking(svc *platformadmin.Service, log *slog.Logger) func() {
	if svc == nil {
		log.Warn("error tracking disabled: platform admin service unavailable")
		return func() {}
	}

	tracker := errtrack.New(&errorLogSink{svc: svc, log: log}, log, errtrack.Config{})
	errtrack.Install(tracker)

	// errtrack cannot read authctx itself without closing an import cycle, so
	// the resolver is handed in here.
	errtrack.SetActorResolver(func(ctx context.Context) (int64, string, string, string, bool) {
		actor, ok := authctx.From(ctx)
		if !ok {
			return 0, "", "", "", false
		}
		return actor.UserID, actor.Name, actor.Email, actor.OrgType, true
	})

	log.Info("error tracking installed", "sink", "platform_admin.error_logs")

	return func() {
		errtrack.Install(nil)
		tracker.Stop()
	}
}
