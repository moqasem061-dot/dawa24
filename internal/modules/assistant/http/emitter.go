package http

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
)

// emitter writes a turn's events into the durable buffer.
//
// It is the only thing the agent loop talks to about output, and it deliberately
// knows nothing about HTTP. That is what lets the same turn be read by a client
// that connected after it started, by one that reconnected halfway through, and
// by none at all.
//
// Every method swallows its error. A buffer write failing is worth logging and
// is not worth aborting a turn over: the answer is still being produced and
// still gets persisted to Postgres at the end, which is the copy that matters.
type emitter struct {
	buffer stream.Buffer
	turnID string
	log    *slog.Logger
	// ctx is detached from any request. Emissions must keep working after the
	// reader that triggered the turn has gone.
	ctx context.Context
}

func newEmitter(buffer stream.Buffer, turnID string, log *slog.Logger) *emitter {
	return &emitter{buffer: buffer, turnID: turnID, log: log, ctx: context.Background()}
}

func (e *emitter) append(c stream.Chunk) {
	if err := e.buffer.Append(e.ctx, e.turnID, c); err != nil {
		e.log.Error("assistant: stream append failed", "turn", e.turnID, "error", err)
	}
}

func (e *emitter) Delta(text string) {
	if text == "" {
		return
	}
	e.append(stream.Chunk{Kind: "delta", Text: text})
}

func (e *emitter) Reasoning(text string) {
	if text == "" {
		return
	}
	e.append(stream.Chunk{Kind: "reasoning", Text: text})
}

func (e *emitter) Status(stage string, data map[string]any) {
	payload := map[string]any{"stage": stage}
	for k, v := range data {
		payload[k] = v
	}
	e.append(stream.Chunk{Kind: "status", Data: payload})
}

func (e *emitter) Usage(input, output int) {
	e.append(stream.Chunk{Kind: "usage", Data: map[string]any{
		"input_tokens":  input,
		"output_tokens": output,
	}})
}

// Entities publishes the records this answer names.
//
// It is a frame of its own rather than a field on "done" because the client
// uses it the moment it arrives: the answer is already on screen, and the links
// appear in place without a re-render of the whole message. It is repeated in
// the terminal frame all the same, for a reader that attached late.
func (e *emitter) Entities(list []assistant.Entity) {
	if len(list) == 0 {
		return
	}
	e.append(stream.Chunk{Kind: "entities", Data: map[string]any{"entities": list}})
}

// Done ends the turn and repeats the finished answer.
//
// Repeating it is deliberate. The client uses it only when it received no
// deltas, which is exactly the case that used to render an empty bubble: a turn
// that spent its whole output budget reasoning produced no delta at all, and
// the reader was told "finished" with nothing to show.
func (e *emitter) Done(answer string, conversationID int64) {
	e.append(stream.Chunk{Kind: "done", Data: map[string]any{
		"answer":          answer,
		"conversation_id": conversationID,
	}})
}

// Terminal frames carry no entities: they are already their own frame above,
// and the polling fallback reads them from the turn row.

// Failed reports a failure by code. The message the user sees is looked up
// here, from the one table that holds them, so no handler can invent its own
// wording or leak an internal one.
// Failed reports a failure by code, and hands back whatever was written before
// it. Two thirds of an answer is worth more to the reader than an apology.
func (e *emitter) Failed(code assistant.Code, partial string, conversationID int64) {
	f := assistant.Fail(code)
	e.append(stream.Chunk{Kind: "error", Data: map[string]any{
		"code":            string(f.Code),
		"message":         f.Message,
		"retryable":       f.Retryable,
		"answer":          partial,
		"conversation_id": conversationID,
	}})
}

// close starts the buffer's retention clock for this turn.
func (e *emitter) close() {
	if err := e.buffer.Close(e.ctx, e.turnID); err != nil {
		e.log.Warn("assistant: stream close failed", "turn", e.turnID, "error", err)
	}
}
