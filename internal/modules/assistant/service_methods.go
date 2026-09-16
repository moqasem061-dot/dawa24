package assistant

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/safe"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// RunTurn answers one question, streaming as it goes.
//
// ctx here is NOT the HTTP request's context. The caller detaches it, so the
// turn keeps running when the browser goes away: the answer is finished, the
// tokens that were already bought are used, and the result is persisted where
// a reconnecting client can find it.
func (s *Service) RunTurn(
	ctx context.Context,
	actor authctx.Actor,
	cfg AgentConfig,
	turn *Turn,
	in TurnInput,
	em Emitter,
) {
	ctx, cancel := context.WithTimeout(ctx, turnDeadline)
	defer cancel()
	channel := in.Channel
	if channel == "" {
		channel = actions.ChannelWeb
	}
	ctx = actions.WithTurn(ctx, actions.Turn{ConversationID: turn.ConversationID, Channel: channel})

	window := s.ContextWindow(ctx)
	messages := s.BuildMessages(ctx, actor, cfg, turn.ConversationID, in, window)

	var (
		answer     strings.Builder
		lastUsage  *gateway.Usage
		toolsUsed  int
		virtualKey string
		entities   []Entity
	)
	if s.keys != nil && actor.OrgID > 0 {
		if vk, err := s.keys(ctx, actor.OrgID); err == nil {
			virtualKey = vk
		}
	}

	schemas := s.tools.Schemas(actor)

	// An image costs input tokens before the model has written a word, and a
	// reasoning model bills its chain of thought against the same ceiling as
	// its answer. A turn carrying a photograph on the standard budget is the
	// case that reliably came back empty — the budget went on looking, and
	// there was nothing left to say what was seen.
	maxTokens := 6000
	if len(in.Parts) > 0 {
		maxTokens = 8000
	}

	for round := 0; round <= maxToolRounds; round++ {
		// The last permitted round drops the tools entirely. Left in, a model
		// that has decided to call something keeps calling it, hits the cap and
		// returns nothing; taken away, it answers from what it has already
		// read, which is what the user wanted three rounds ago.
		roundTools := schemas
		if round == maxToolRounds {
			roundTools = nil
		}

		events, err := s.gateway.Stream(ctx, gateway.ChatRequest{
			Role:     gateway.RolePrimary,
			Messages: messages,
			Tools:    roundTools,
			// Reasoning models bill their chain of thought against this budget
			// and, when they exhaust it, return an EMPTY answer — a total
			// failure that looks exactly like a model with nothing to say. One
			// real turn did precisely that: 2048 tokens spent, no text, no tool
			// call. Six thousand leaves room to think and still answer.
			MaxTokens:   maxTokens,
			Temperature: 0.3,
			OrgID:       actor.OrgID,
			UserID:      actor.UserID,
			VirtualKey:  virtualKey,
			Feature:     matchflow.FeatureAssistant,
		})
		if err != nil {
			s.log.WarnContext(ctx, "assistant stream failed",
				"user_id", actor.UserID, "org_id", actor.OrgID, "error", err)
			s.fail(ctx, turn, em, ClassifyGateway(err), answer.String())
			return
		}

		text, calls, usage, streamErr := s.consume(events, em, &answer)
		if usage != nil {
			lastUsage = usage
		}
		if streamErr != nil {
			// Partial text is kept. An answer that was two thirds written is
			// worth more to the reader than an error message, and it has
			// already been paid for.
			s.fail(ctx, turn, em, ClassifyGateway(streamErr), answer.String())
			return
		}

		if len(calls) == 0 {
			s.succeed(ctx, actor, turn, in, em, answer.String(), lastUsage, toolsUsed, entities)
			return
		}

		messages = append(messages, gateway.ChatMessage{
			Role:      "assistant",
			Text:      text,
			ToolCalls: calls,
		})
		runnable := calls
		if room := maxToolCalls - toolsUsed; len(runnable) > room {
			runnable = runnable[:max(room, 0)]
		}
		toolsUsed += len(runnable)
		outcomes := s.dispatchRound(ctx, actor, turn.ID, runnable, em)
		for i, call := range calls {
			content := `{"error":"tool call limit reached for this turn; answer with what you have"}`
			if i < len(outcomes) {
				content = outcomes[i].Content
				entities = MergeEntities(entities, outcomes[i].Entities...)
			}
			messages = append(messages, gateway.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Text:       content,
			})
		}
	}

	// Round budget exhausted with the model still asking. Answer with what was
	// collected rather than with nothing.
	s.succeed(ctx, actor, turn, in, em, answer.String(), lastUsage, toolsUsed, entities)
}

// dispatchRound runs one round's tool calls concurrently and returns their
// outcomes in call order.
//
// Calls in one round are independent by construction — the model issued them
// together, before seeing any result — so a turn that asks for three datasets
// waits for the slowest, not the sum. Concurrency is bounded so one round
// cannot take more than a few pooled connections. Status frames are emitted
// here, on the turn's goroutine, so the emitter is never written concurrently.
func (s *Service) dispatchRound(
	ctx context.Context, actor authctx.Actor, turnID int64, calls []gateway.ToolCall, em Emitter,
) []ToolOutcome {
	for _, call := range calls {
		em.Status("tool", map[string]any{"tool": call.Name, "state": "running"})
	}
	outcomes := make([]ToolOutcome, len(calls))
	sem := make(chan struct{}, maxParallelTools)
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c gateway.ToolCall) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				outcomes[idx] = ToolOutcome{Name: c.Name, Decision: "error", Content: "context cancelled"}
				return
			}
			defer func() { <-sem }()
			outcomes[idx] = s.dispatchSafely(ctx, actor, turnID, c)
		}(i, call)
	}
	wg.Wait()
	for _, o := range outcomes {
		em.Status("tool", map[string]any{"tool": o.Name, "state": o.Decision, "rows": o.Rows})
	}
	return outcomes
}

// dispatchSafely keeps a panicking tool from taking the process down with it:
// it runs on its own goroutine, where the turn's recovery cannot reach.
func (s *Service) dispatchSafely(ctx context.Context, actor authctx.Actor, turnID int64, call gateway.ToolCall) (out ToolOutcome) {
	defer func() {
		if p := recover(); p != nil {
			s.log.ErrorContext(ctx, "assistant tool panicked", "tool", call.Name, "panic", p)
			out = ToolOutcome{CallID: call.ID, Name: call.Name, Decision: "failed",
				Content: `{"error":"تعذّر قراءة البيانات المطلوبة."}`}
		}
	}()
	return s.tools.Dispatch(ctx, actor, turnID, call)
}

// consume drains one gateway stream, forwarding deltas as they arrive.
func (s *Service) consume(
	events <-chan gateway.StreamEvent, em Emitter, answer *strings.Builder,
) (string, []gateway.ToolCall, *gateway.Usage, error) {
	var (
		round strings.Builder
		calls []gateway.ToolCall
		usage *gateway.Usage
	)
	for ev := range events {
		switch {
		case ev.Err != nil:
			return round.String(), nil, usage, ev.Err
		case ev.Reasoning != "":
			em.Reasoning(ev.Reasoning)
		}
		if ev.Delta != "" {
			round.WriteString(ev.Delta)
			answer.WriteString(ev.Delta)
			em.Delta(ev.Delta)
		}
		if ev.Usage != nil {
			usage = ev.Usage
			em.Usage(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
		}
		if ev.Done {
			calls = ev.ToolCalls
		}
	}
	return round.String(), calls, usage, nil
}

// succeed persists the turn and tells the reader it is finished.
func (s *Service) succeed(
	ctx context.Context,
	actor authctx.Actor,
	turn *Turn,
	in TurnInput,
	em Emitter,
	answer string,
	usage *gateway.Usage,
	toolsUsed int,
	entities []Entity,
) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		// Distinguish "produced nothing" from "spent the budget producing
		// nothing". The second is the reasoning-burn case, and "rephrase" is
		// useless advice for it; a narrower question is the fix that works.
		if usage != nil && usage.CompletionTokens > 0 {
			answer = "استغرق التحليل مساحة أكبر من المتاحة قبل أن يكتمل الرد. " +
				"اسأل عن فترة أقصر أو نقطة واحدة محددة."
		} else {
			answer = "لم أتمكن من صياغة إجابة لهذا السؤال. جرّب صياغة أوضح."
		}
		s.log.WarnContext(ctx, "assistant produced an empty answer",
			"user_id", actor.UserID, "tools", toolsUsed)
	}
	turn.Status = TurnDone
	turn.Answer = answer
	turn.ToolCalls = toolsUsed
	turn.Entities = s.linkEntities(actor, answer, entities)
	if len(turn.Entities) > 0 {
		em.Entities(turn.Entities)
	}
	if usage != nil {
		turn.InputTokens = usage.PromptTokens
		turn.OutputTokens = usage.CompletionTokens
	}
	s.persist(ctx, actor, turn, in, answer)
	em.Done(answer, turn.ConversationID)
}

// fail persists whatever was produced and reports a user-facing code.
func (s *Service) fail(ctx context.Context, turn *Turn, em Emitter, code Code, partial string) {
	turn.Status = TurnFailed
	turn.ErrorCode = string(code)
	turn.Answer = strings.TrimSpace(partial)
	if s.repo != nil {
		// context.WithoutCancel: the turn may be failing precisely because the
		// deadline fired, and a cancelled context cannot write the record of
		// why.
		if err := s.repo.FinishTurn(context.WithoutCancel(ctx), turn); err != nil {
			s.log.ErrorContext(ctx, "assistant: finish failed turn", "error", err)
		}
	}
	em.Failed(code, turn.Answer, turn.ConversationID)
}

// persist writes the question, the answer and the turn record.
//
// Every write here uses a context detached from cancellation for the same
// reason: this runs at the end of a turn, which is exactly when the client is
// most likely to have gone.
func (s *Service) persist(
	ctx context.Context, actor authctx.Actor, turn *Turn, in TurnInput, answer string,
) {
	if s.repo == nil {
		return
	}
	saveCtx := context.WithoutCancel(ctx)

	question := strings.TrimSpace(in.Text)
	if question == "" && len(in.Attachments) > 0 {
		question = "مرفق: " + in.Attachments[0].Filename
	}

	if err := s.repo.SaveMessage(saveCtx, &Message{
		ConversationID: turn.ConversationID,
		OrganizationID: turn.OrganizationID,
		Role:           "user",
		Content:        question,
		Attachments:    in.Attachments,
		PromptVersion:  SystemPromptVersion,
		ModelRole:      string(gateway.RolePrimary),
	}); err != nil {
		s.log.ErrorContext(ctx, "assistant: save question", "error", err)
	}

	if err := s.repo.SaveMessage(saveCtx, &Message{
		ConversationID: turn.ConversationID,
		OrganizationID: turn.OrganizationID,
		Role:           "assistant",
		Content:        answer,
		PromptVersion:  SystemPromptVersion,
		ModelRole:      string(gateway.RolePrimary),
		InputTokens:    turn.InputTokens,
		OutputTokens:   turn.OutputTokens,
		Entities:       turn.Entities,
	}); err != nil {
		s.log.ErrorContext(ctx, "assistant: save answer", "error", err)
	}

	if ids := attachmentIDs(in.Attachments); len(ids) > 0 {
		if err := s.repo.MarkAttachmentsReferenced(saveCtx, ids, turn.ConversationID); err != nil {
			s.log.WarnContext(ctx, "assistant: mark attachments referenced", "error", err)
		}
	}

	if err := s.repo.FinishTurn(saveCtx, turn); err != nil {
		s.log.ErrorContext(ctx, "assistant: finish turn", "error", err)
	}

	if actor.OrgID > 0 && s.gateway != nil && s.gateway.Enabled() {
		safe.Go(s.log, "assistant-maybe-extract-memory", func() {
			s.maybeExtractMemory(saveCtx, actor, question, answer)
		})
	}
}

func attachmentIDs(atts []Attachment) []int64 {
	var ids []int64
	for _, a := range atts {
		if a.RowID > 0 {
			ids = append(ids, a.RowID)
		}
	}
	return ids
}

// PurgeExpiredConversations deletes conversations six months after they were
// created. Called by the worker daily; see cmd/worker.
func (s *Service) PurgeExpiredConversations(ctx context.Context) (int, error) {
	if s.repo == nil {
		return 0, errors.New("assistant: no repository")
	}
	return s.repo.PurgeExpiredConversations(ctx, time.Now())
}

// PurgeOrphanAttachments removes uploads that were never sent with a question.
func (s *Service) PurgeOrphanAttachments(ctx context.Context) ([]string, error) {
	if s.repo == nil {
		return nil, errors.New("assistant: no repository")
	}
	return s.repo.PurgeOrphanAttachments(ctx, time.Now().Add(-24*time.Hour))
}
