package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Fitting a tool result into the budget, without throwing it away.
//
// There is a byte ceiling on what one tool call may hand back, and there has to
// be: a result is JSON in a prompt, and an unbounded one is an unbounded
// prompt. The ceiling is not the problem. What was the problem is what happened
// when a result crossed it.
//
// The previous behaviour was to discard the entire payload and substitute a
// sentence asking the user to narrow their search. Measured against real row
// shapes, a purchase-order listing crossed six kilobytes at **fifteen rows** —
// while the tools themselves are allowed to return twenty-five. So the common
// case, "list my orders", produced a result containing no orders at all, and
// the model, correctly, reported that it could not find anything. That is the
// reported failure: an assistant that returns almost no data.
//
// A truncated answer is worth incomparably more than no answer. So instead of
// dropping the payload, rows are dropped from it until it fits, and the result
// says so — how many were shown, how many were left, and that paging is how you
// get the rest. The model can then answer the question with fifteen of the
// twenty-five rows and tell the user there are more, which is what a person
// reading a screen does.
//
// The trimming works on the encoded JSON rather than on Go types on purpose.
// Tools return maps, structs and nested aggregates, and a reflection walk over
// all of those is a per-shape thing to get wrong. One decode into a generic
// tree finds the largest array wherever it lives, and a tool added tomorrow
// with a shape nobody anticipated is trimmed correctly without touching this
// file.

// trimFloor is the fewest rows worth returning. Below this the result has
// stopped being an answer and the honest response is to ask for a narrower
// question — one row out of two hundred tells the user nothing and invites the
// model to generalise from it, which is the one thing it must not do.
const trimFloor = 3

// encodeResult renders a tool result for the model, shrinking it to fit the
// byte ceiling rather than discarding it.
func encodeResult(res Result) string {
	payload := map[string]any{}
	if res.Data != nil {
		payload["data"] = res.Data
	}
	if res.Note != "" {
		payload["note"] = res.Note
	}
	if len(payload) == 0 {
		payload["note"] = "لا توجد نتائج مطابقة."
	}

	out, err := encodeJSON(payload)
	if err != nil {
		return `{"error":"تعذّر تجهيز النتيجة."}`
	}
	if len(out) <= maxResultBytes {
		return out
	}
	return shrinkToFit(out, res.Rows)
}

// encodeJSON marshals without HTML escaping, so Arabic and the few characters
// Go escapes by default reach the model as themselves.
func encodeJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

// shrinkToFit drops rows from the largest array in the payload until the whole
// encodes within the ceiling.
//
// It returns the oversize note only when there is nothing left to drop: a
// payload with no array in it, or one whose rows are individually so large that
// even trimFloor of them does not fit.
func shrinkToFit(encoded string, rows int) string {
	dec := json.NewDecoder(strings.NewReader(encoded))
	// Exact numerics. A decode into float64 and back would round order totals
	// and ids, and a subtly wrong number in a prompt is worse than a large one.
	dec.UseNumber()

	var tree any
	if err := dec.Decode(&tree); err != nil {
		return oversizeNote(rows)
	}

	parent, key, list := largestArray(tree)
	if parent == nil || len(list) <= trimFloor {
		return oversizeNote(rows)
	}

	// Halve until it fits. The encoded size is not linear in row count — the
	// wrapper, the totals and any sibling fields are fixed cost — so estimating
	// a target row count from the byte overshoot lands short and needs a loop
	// anyway. Halving gets there in a handful of encodes and cannot overshoot
	// into an empty list.
	keep := len(list)
	for keep > trimFloor {
		next := keep / 2
		if next < trimFloor {
			next = trimFloor
		}
		keep = next

		setArray(parent, key, list[:keep])
		annotate(tree, keep, len(list)-keep)

		out, err := encodeJSON(tree)
		if err != nil {
			return oversizeNote(rows)
		}
		if len(out) <= maxResultBytes {
			return out
		}
	}
	return oversizeNote(rows)
}

// largestArray finds the array holding the most elements anywhere in the tree,
// and returns the container that holds it so it can be replaced.
//
// "Largest" is by element count rather than by encoded size because it is the
// row list that is meant to shrink. A tool returning both a long list of rows
// and a short list of totals must lose rows, not totals.
func largestArray(tree any) (parent any, key any, list []any) {
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			for k, v := range n {
				if arr, ok := v.([]any); ok && len(arr) > len(list) {
					parent, key, list = n, k, arr
				}
				walk(v)
			}
		case []any:
			for i, v := range n {
				if arr, ok := v.([]any); ok && len(arr) > len(list) {
					parent, key, list = n, i, arr
				}
				walk(v)
			}
		}
	}
	walk(tree)
	return parent, key, list
}

// setArray writes the trimmed list back into whichever container held it.
func setArray(parent, key any, list []any) {
	switch p := parent.(type) {
	case map[string]any:
		if k, ok := key.(string); ok {
			p[k] = list
		}
	case []any:
		if i, ok := key.(int); ok && i >= 0 && i < len(p) {
			p[i] = list
		}
	}
}

// annotate records the trim at the top level, where the model will read it.
//
// It is stated as a fact about the result rather than as an apology, and it
// names paging as the remedy, because every listing tool takes an offset. A
// model told only "this was truncated" tends to re-ask the same question; one
// told "there are 210 more, use offset" either pages or says so in its answer.
func annotate(tree any, shown, omitted int) {
	root, ok := tree.(map[string]any)
	if !ok {
		return
	}
	root["truncated"] = true
	root["shown_rows"] = shown
	root["omitted_rows"] = omitted
	root["note"] = fmt.Sprintf(
		"عُرض %d صفاً فقط من النتيجة لضخامة حجمها، وحُذف %d صفاً. "+
			"أجب بما هو معروض وأشر إلى وجود المزيد، أو استخدم offset لجلب بقية الصفوف.",
		shown, omitted)
}

// oversizeNote is the last resort: nothing in this result could be trimmed into
// the budget, so the model is told what happened and what to do instead.
func oversizeNote(rows int) string {
	if rows > 0 {
		return fmt.Sprintf(
			`{"note":"النتيجة (%d سطر) أكبر من الحد المسموح ولا يمكن اختصارها. ضيّق نطاق البحث أو حدّد فترة أقصر."}`,
			rows)
	}
	return `{"note":"النتيجة أكبر من الحد المسموح. ضيّق نطاق البحث أو حدّد فترة أقصر."}`
}

// ---------------------------------------------------------------------------
// Audit detail
// ---------------------------------------------------------------------------

// failureClass turns a tool's error into a word safe to store.
//
// The error itself goes to the log, where it may name a table and a column
// because the reader is an engineer with a database. The audit trail is read on
// a dashboard screen, by a person who is not, and it is written on a path a
// model's arguments reach — so it gets a class, not a message. Anything
// unrecognised is "read_failed" rather than a truncated error string, because a
// truncated error string is how a column name ends up on a screen.
func failureClass(ctx context.Context, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		// The tool's own timeout fired. This is the class worth separating:
		// it means the query is too slow, not that the caller was refused.
		return "timeout"
	case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		return "canceled"
	default:
		return "read_failed"
	}
}

// allowedDetail records what an allowed call actually produced.
//
// "allowed" with no rows is the single most confusing line in the trail: the
// permission passed, the query ran, and the user still saw an answer with no
// data in it. Saying "empty" here separates that from a call that returned
// something, and separates both from a call that was trimmed to fit.
func allowedDetail(res Result) string {
	switch {
	case res.Rows == 0:
		return "empty"
	case res.Note != "":
		return "partial"
	default:
		return ""
	}
}
