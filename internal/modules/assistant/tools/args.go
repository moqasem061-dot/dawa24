package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Argument handling, and why it is this strict.
//
// Tool arguments are the one place a model's output reaches a query, so they
// are treated the way any other untrusted input is: parsed into a fixed shape,
// range-checked, and refused rather than coerced. Nothing here interpolates a
// string into SQL — every value below ends up as a bound parameter — but the
// discipline matters anyway, because a silently-clamped nonsense value produces
// a confidently wrong answer, which is worse than a refusal.

var (
	// errBadArgs is a refusal the model can act on: it says what was wrong.
	errBadArgs = errors.New("invalid arguments")
	// errBadHandle means a reference did not verify. The model is told nothing
	// beyond "invalid", because the difference between "forged", "expired" and
	// "belongs to someone else" is exactly what a prober wants to learn.
	errBadHandle = errors.New("invalid handle")
)

func badArgs(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errBadArgs, fmt.Sprintf(format, a...))
}

// decode parses tool arguments into dst, refusing anything unexpected.
//
// DisallowUnknownFields is the load-bearing line. It turns a hallucinated
// "organization_id" or a smuggled "limit_override" into a visible refusal
// instead of a field that is silently dropped and an answer that looks fine.
func decode(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return badArgs("تعذّر قراءة المعطيات: %s", sanitizeDecodeError(err))
	}
	// Reject trailing content: {"a":1}{"b":2} must not read as the first object.
	if dec.More() {
		return badArgs("المعطيات تحتوي على بيانات زائدة.")
	}
	return nil
}

// sanitizeDecodeError keeps the field name (useful to the model) and drops
// everything else the encoding/json error carries.
func sanitizeDecodeError(err error) string {
	msg := err.Error()
	if idx := strings.Index(msg, "json: unknown field "); idx >= 0 {
		return "حقل غير معروف " + strings.TrimPrefix(msg[idx:], "json: unknown field ")
	}
	if strings.Contains(msg, "cannot unmarshal") {
		return "نوع القيمة غير صحيح"
	}
	return "صيغة غير صالحة"
}

// ---------------------------------------------------------------------------
// Shared argument shapes
// ---------------------------------------------------------------------------

// dateRangeArgs is the period filter every aggregate accepts.
type dateRangeArgs struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// parse turns the two date strings into a range, defaulting the window rather
// than leaving it unbounded: "how much did I spend" without a period means the
// last ninety days, not the whole history of the company.
func (a dateRangeArgs) parse(defaultDays int) (assistant.DateRange, error) {
	var out assistant.DateRange
	if a.From != "" {
		t, err := parseDay(a.From)
		if err != nil {
			return out, badArgs("قيمة from ليست تاريخاً بصيغة YYYY-MM-DD.")
		}
		out.From = t
	}
	if a.To != "" {
		t, err := parseDay(a.To)
		if err != nil {
			return out, badArgs("قيمة to ليست تاريخاً بصيغة YYYY-MM-DD.")
		}
		// Inclusive of the named day.
		out.To = t.Add(24*time.Hour - time.Second)
	}
	if out.From.IsZero() && out.To.IsZero() && defaultDays > 0 {
		out.From = time.Now().AddDate(0, 0, -defaultDays)
	}
	if !out.From.IsZero() && !out.To.IsZero() && out.To.Before(out.From) {
		return out, badArgs("نهاية الفترة قبل بدايتها.")
	}
	return out, nil
}

func parseDay(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.UTC)
}

// clampLimit keeps a model-supplied page size inside the server's ceiling.
func clampLimit(requested int) int {
	if requested <= 0 {
		return assistant.PageLimit
	}
	if requested > assistant.PageLimit {
		return assistant.PageLimit
	}
	return requested
}

// clampOffset refuses a negative page and caps how deep a model may page.
//
// A thousand rows is far past the point where a conversation should be reading
// a table; beyond that the honest answer is "use the screen, or narrow this".
func clampOffset(requested int) (int, error) {
	if requested < 0 {
		return 0, badArgs("قيمة offset لا يمكن أن تكون سالبة.")
	}
	if requested > 1000 {
		return 0, badArgs("تجاوزت حد التصفح. ضيّق البحث بدل الاستمرار في الصفحات.")
	}
	return requested, nil
}

// oneOf validates an enumerated argument. An empty value means "no filter".
func oneOf(field, value string, allowed ...string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return "", nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", badArgs("قيمة %s غير مقبولة. المسموح: %s.", field, strings.Join(allowed, ", "))
}

// trimSearch bounds a free-text filter.
//
// The value is always bound as a query parameter, never concatenated, so length
// is about cost and not injection: a multi-kilobyte ILIKE pattern is a table
// scan somebody talked the model into.
func trimSearch(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return "", badArgs("نص البحث طويل جداً.")
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Handles
// ---------------------------------------------------------------------------

// bindingFor is the caller a handle must have been issued to. It is read from
// the actor, so it cannot be influenced by anything in the arguments.
func bindingFor(actor authctx.Actor) handles.Binding {
	return handles.Binding{OrgID: actor.OrgID, UserID: actor.UserID}
}

// resolveHandle verifies one reference and returns the row id behind it.
func (r *Registry) resolveHandle(actor authctx.Actor, kind handles.Kind, token string) (int64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, badArgs("المرجع مطلوب.")
	}
	id, err := r.signer.Resolve(token, kind, bindingFor(actor))
	if err != nil {
		return 0, errBadHandle
	}
	return id, nil
}

// BranchRef mints a branch reference for the session context.
func (r *Registry) BranchRef(actor authctx.Actor, branchID int64) string {
	return r.issue(actor, handles.KindBranch, branchID)
}

// issue mints a reference for a row the caller has just been shown.
func (r *Registry) issue(actor authctx.Actor, kind handles.Kind, id int64) string {
	return r.signer.Issue(kind, id, bindingFor(actor))
}

// ---------------------------------------------------------------------------
// Schema helpers
// ---------------------------------------------------------------------------

// objectSchema builds a JSON Schema object with additionalProperties disabled.
//
// Every tool goes through here so that "no extra fields" is a property of the
// package rather than something each author has to remember. It pairs with
// DisallowUnknownFields in decode: one tells the model, the other enforces it.
func objectSchema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func enumProp(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func intProp(desc string, min, max int) map[string]any {
	return map[string]any{
		"type": "integer", "description": desc,
		"minimum": min, "maximum": max,
	}
}

// dateProps are the two period fields shared by every aggregate tool.
func dateProps(into map[string]any) map[string]any {
	if into == nil {
		into = map[string]any{}
	}
	into["from"] = strProp("بداية الفترة YYYY-MM-DD.")
	into["to"] = strProp("نهاية الفترة YYYY-MM-DD.")
	return into
}

// pageProps are the two paging fields shared by every listing tool.
func pageProps(into map[string]any) map[string]any {
	if into == nil {
		into = map[string]any{}
	}
	into["limit"] = intProp("عدد الصفوف، بحد أقصى 25.", 1, assistant.PageLimit)
	into["offset"] = intProp("تخطي صفوف للصفحة التالية.", 0, 1000)
	return into
}
