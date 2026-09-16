// Package tools is the only path from something a model said to something the
// database reads.
//
// The whole security posture of the assistant rests on one property of this
// package: a tool call is data, not an instruction. What arrives from the model
// is a name and a blob of JSON, both of which a prompt injection, a poisoned
// product description, or a user simply asking can control. Nothing in that
// blob decides who the caller is, which organisation is read, or whether the
// call is allowed. Those come from the request's own authenticated session,
// every time, after the model has spoken.
//
// Concretely, Dispatch always runs the same sequence and there is no other way
// in:
//
//  1. the caller is read from the request context   (never from arguments)
//  2. the assistant gate permission is re-checked   (owner's per-role switch)
//  3. the tool is looked up by exact name           (no fuzzy matching)
//  4. the tool's dashboard scope must match         (pharmacy ≠ vendor ≠ admin)
//  5. the caller must hold the tool's permission    (the same key the screen needs)
//  6. arguments are decoded strictly                (unknown fields are refused)
//  7. handle arguments are verified and bound       (forgery and enumeration die here)
//  8. the call runs under its own timeout
//  9. results are truncated to a row and byte ceiling
//  10. the decision is written to the audit log, allowed or denied
//
// A tool added tomorrow inherits all ten by existing. That is the point of the
// registry: there is no per-tool place to forget a check.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Decision is the outcome recorded for one tool call.
type Decision string

const (
	DecisionAllowed    Decision = "allowed"
	DecisionGate       Decision = "denied_gate"
	DecisionScope      Decision = "denied_scope"
	DecisionPermission Decision = "denied_permission"
	DecisionHandle     Decision = "denied_handle"
	DecisionInvalid    Decision = "invalid_args"
	DecisionUnknown    Decision = "unknown_tool"
	DecisionFailed     Decision = "failed"
)

// maxResultBytes caps what one tool call may add to the conversation.
//
// Twenty-four kilobytes allows rich data structures, multi-line order details,
// and complete product lists without aggressive truncation, while remaining
// well within the primary model's context window.
const maxResultBytes = 24 << 10

// Result is what a tool hands back to the model.
type Result struct {
	// Data is marshalled to JSON and becomes the tool message content.
	Data any
	// Note is a short sentence for the model when there is nothing to return,
	// or when a refusal needs explaining. It is model-facing, never shown raw.
	Note string
	// Rows is how many records Data holds, for the audit log.
	Rows int
	// Entities are references the tool produced that are not discoverable by
	// walking Data: dataset rows are plain arrays, exports and proposals are
	// not records at all.
	Entities []assistant.Entity
}

// Handler runs one tool. It receives the live actor; it must never take an
// identity from args.
type Handler func(ctx context.Context, actor authctx.Actor, args json.RawMessage) (Result, error)

// Tool is one read-only capability offered to a model.
type Tool struct {
	Name        string
	Description string
	// Params is the JSON Schema shown to the model. Build it with objectSchema
	// so additionalProperties:false is never forgotten.
	Params map[string]any
	// Scopes are the dashboards this tool belongs to. A tool with no scope is
	// unreachable, which is the correct default for a mistake.
	Scopes []rbac.Scope
	// Permissions are the dashboard permission keys, any one of which admits
	// the caller. These are the SAME keys the corresponding screen is gated on,
	// so the assistant can never show an employee something they could not
	// already open themselves.
	Permissions []string
	Timeout     time.Duration
	Handler     Handler
}

// AuditSink records tool decisions. Failures to record are logged and dropped:
// the audit trail must not be able to fail a read.
type AuditSink interface {
	RecordToolCall(ctx context.Context, entry assistant.ToolAudit)
}

var _ assistant.ToolRunner = (*Registry)(nil)

// Registry holds every tool and performs dispatch.
type Registry struct {
	byName      map[string]Tool
	order       []string
	signer      *handles.Signer
	reader      assistant.Reader
	projections assistant.ProjectionReader
	audit       AuditSink
	memories    MemoryStore
	// coverage answers "who delivers to this branch". It is optional: with no
	// probe wired the coverage tool reports itself unavailable rather than
	// guessing, which is the only safe behaviour for a rule that decides what
	// checkout will accept.
	coverage assistant.CoverageProbe

	datasets      *datasets.Catalog
	datasetRunner DatasetRunner
	exports       ExportStore

	flow   *actions.Flow
	offers OfferFinder

	log *slog.Logger
}

// NewRegistry builds the registry and declares every tool.
func NewRegistry(reader assistant.Reader, signer *handles.Signer, audit AuditSink, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	r := &Registry{
		byName: make(map[string]Tool),
		signer: signer,
		reader: reader,
		audit:  audit,
		log:    log.With("component", "capsule_tools"),
	}
	if pr, ok := reader.(assistant.ProjectionReader); ok {
		r.projections = pr
	}
	if ms, ok := reader.(MemoryStore); ok {
		r.memories = ms
	}
	r.declare(dataTools(r)...)
	r.declare(actionTools(r)...)
	r.declare(guideTools(r)...)
	r.declare(sharedTools(r)...)
	r.declare(memoryTools(r)...)
	r.declare(adminTools(r)...)
	r.declare(coverageTools(r)...)
	r.declare(buyingStage3Tools(r)...)
	r.declare(pharmacyStage3Tools(r)...)
	r.declare(vendorStage3Tools(r)...)
	r.declare(adminStage3Tools(r)...)
	return r
}

// SetCoverageProbe wires the platform's coverage rule into the coverage tool.
//
// It is a setter rather than a constructor argument because the probe is
// assembled from another module's service in the composition root, and
// NewRegistry is called from tests that have neither.
func (r *Registry) SetCoverageProbe(p assistant.CoverageProbe) {
	r.coverage = p
}

// SetMemoryStore explicitly configures the organization memory store.
func (r *Registry) SetMemoryStore(ms MemoryStore) {
	r.memories = ms
}

func (r *Registry) declare(tools ...Tool) {
	for _, t := range tools {
		if _, exists := r.byName[t.Name]; exists {
			panic("assistant tools: duplicate tool name " + t.Name)
		}
		if t.Timeout == 0 {
			t.Timeout = 12 * time.Second
		}
		r.byName[t.Name] = t
		r.order = append(r.order, t.Name)
	}
}

// Names returns every declared tool name, in declaration order. Used by tests
// that assert schema hygiene across the whole registry.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Lookup returns a declared tool. Reported for tests and diagnostics.
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// Schemas returns the tools this caller may use.
//
// Filtering here is not the security boundary — Dispatch re-checks everything —
// but it matters for two other reasons. A model cannot be talked into calling a
// tool it was never shown, and a prompt that lists twenty irrelevant tools
// produces worse answers than one that lists the six that apply.
func (r *Registry) Schemas(actor authctx.Actor) []gateway.ToolSpec {
	scope := actor.DashboardScope()
	if scope == "" {
		return nil
	}
	if _, ok := assistant.Allowed(actor); !ok {
		return nil
	}
	out := make([]gateway.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		if !scopeAllows(t.Scopes, scope) || !actor.CanAny(t.Permissions...) {
			continue
		}
		if name == "propose_action" {
			if spec, ok := r.actionSpec(actor); ok {
				out = append(out, spec)
			}
			continue
		}
		out = append(out, gateway.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Params,
		})
	}
	return out
}

func scopeAllows(scopes []rbac.Scope, want rbac.Scope) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// Dispatch runs one tool call from the model.
//
// It never returns an error: a refusal is an answer the model must be able to
// read and work around ("you cannot see that"), not a transport failure. What
// it does return is always safe to put in a prompt — no SQL, no stack, no
// column names, no other tenant's anything.
func (r *Registry) Dispatch(ctx context.Context, actor authctx.Actor, turnID int64, call gateway.ToolCall) assistant.ToolOutcome {
	started := time.Now()
	name := strings.TrimSpace(call.Name)

	record := func(d Decision, permission, detail string, rows int) {
		if r.audit == nil {
			return
		}
		r.audit.RecordToolCall(ctx, assistant.ToolAudit{
			TurnID:         turnID,
			OrganizationID: actor.OrgID,
			UserID:         actor.UserID,
			AgentRole:      string(actor.DashboardScope()),
			ToolName:       name,
			Decision:       string(d),
			Permission:     permission,
			Detail:         detail,
			LatencyMS:      int(time.Since(started).Milliseconds()),
			RowCount:       rows,
		})
	}

	deny := func(d Decision, permission, detail, note string) assistant.ToolOutcome {
		record(d, permission, detail, 0)
		if d != DecisionInvalid {
			r.log.WarnContext(ctx, "assistant tool refused",
				"tool", name, "decision", d, "detail", detail, "user_id", actor.UserID,
				"org_id", actor.OrgID, "scope", actor.DashboardScope())
		}
		return assistant.ToolOutcome{
			CallID: call.ID, Name: name, Decision: string(d), Content: encodeNote(note),
		}
	}

	// 1–2. Who is calling, and may they use the assistant at all.
	cfg, allowed := assistant.Allowed(actor)
	if !allowed {
		return deny(DecisionGate, "", "gate_not_granted",
			"لا يملك المستخدم صلاحية استخدام المساعد الذكي.")
	}

	// 3. Exact name. Never guess a near match: a model that misremembers a
	// tool must be corrected, not helpfully redirected to a different one.
	tool, ok := r.byName[name]
	if !ok {
		return deny(DecisionUnknown, "", "no_such_tool",
			"لا توجد أداة بهذا الاسم. استخدم فقط الأدوات المتاحة لك.")
	}

	// 4. Dashboard scope.
	if !scopeAllows(tool.Scopes, cfg.Role) {
		return deny(DecisionScope, "", "scope:"+string(cfg.Role),
			"هذه الأداة ليست ضمن لوحة تحكم هذا المستخدم.")
	}

	// 5. The dashboard permission the equivalent screen requires.
	if !actor.CanAny(tool.Permissions...) {
		return deny(DecisionPermission, strings.Join(tool.Permissions, ","), "permission_not_held",
			"هذه البيانات خارج صلاحيات المستخدم الحالي.")
	}

	// 6. Arguments. An empty argument string is a valid no-argument call.
	args := json.RawMessage(strings.TrimSpace(call.Arguments))
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	if !json.Valid(args) {
		return deny(DecisionInvalid, "", "malformed_json", "صيغة المعطيات غير صالحة.")
	}

	// 7–8. Run, bounded.
	runCtx, cancel := context.WithTimeout(ctx, tool.Timeout)
	defer cancel()

	res, err := tool.Handler(runCtx, actor, args)
	if err != nil {
		switch {
		case errors.Is(err, errBadArgs):
			return deny(DecisionInvalid, "", "invalid_arguments", err.Error())
		case errors.Is(err, errBadHandle):
			return deny(DecisionHandle, "", "handle_rejected",
				"المرجع المستخدم غير صالح أو لا يخص هذا الحساب.")
		default:
			// The underlying error may name a table or a column. It goes to the
			// log; the model gets a sentence.
			r.log.ErrorContext(ctx, "assistant tool failed",
				"tool", name, "user_id", actor.UserID, "error", err)
			return deny(DecisionFailed, "", failureClass(runCtx, err),
				"تعذّر قراءة البيانات المطلوبة.")
		}
	}

	// 9. Shape. The rows go to the model as JSON, and separately to the reader
	// as clickable references: CollectEntities walks whatever this tool
	// returned and finds every row that names a record with a dashboard page.
	// A tool does not opt in — it is found by the shape of what it returns —
	// so a tool added later is clickable without touching this file.
	content := encodeResult(res)
	record(DecisionAllowed, strings.Join(tool.Permissions, ","), allowedDetail(res), res.Rows)
	return assistant.ToolOutcome{
		CallID:   call.ID,
		Name:     name,
		Decision: string(DecisionAllowed),
		Rows:     res.Rows,
		Content:  content,
		Entities: assistant.MergeEntities(res.Entities, assistant.CollectEntities(res.Data)...),
	}
}

func encodeNote(note string) string {
	b, err := json.Marshal(map[string]string{"error": note})
	if err != nil {
		return `{"error":"refused"}`
	}
	return string(b)
}
