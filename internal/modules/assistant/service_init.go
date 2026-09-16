package assistant

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

// The agent loop.
//
// It is deliberately small. The model asks for data, the registry decides
// whether it may have it, the answer comes back, and the loop repeats until the
// model stops asking or the round budget runs out. There is no planner, no
// scratchpad, no self-critique step: every one of those costs a full round trip
// and none of them makes an answer about last month's spend more correct than
// reading last month's spend.
//
// What the loop does own is the two bounds that keep it from becoming
// expensive: a hard cap on rounds, and a hard cap on wall time. Both exist
// because an agent that cannot answer usually cannot answer with more tries
// either — it is asking the wrong question, and the useful response is to say
// so rather than to keep spending.

// maxToolRounds is how many times the model may call tools before it must
// answer. Ten covers "describe, query, open a record, check a rule, propose"
// with room to correct one refused call.
const maxToolRounds = 10

// maxToolCalls caps fan-out inside a round as well as repeated rounds. A model
// may ask for several independent rows at once, so a round limit alone is not
// enough to bound database work or prompt growth.
const maxToolCalls = 30

// maxParallelTools bounds how many calls of one round run at once.
const maxParallelTools = 4

// turnDeadline bounds one whole question, tool calls included.
const turnDeadline = 150 * time.Second

// ToolOutcome is one dispatched tool call, ready to hand back to the model.
type ToolOutcome struct {
	CallID   string
	Name     string
	Content  string
	Decision string
	Rows     int
	// Entities are the records this call read that the caller has a screen
	// for. They are collected from the rows themselves, never from anything
	// the model wrote, and become the clickable references in the answer.
	Entities []Entity
}

// ToolRunner is the assistant's view of the tool registry.
//
// Declared here and implemented in assistant/tools so that this package never
// imports that one: the loop knows there are tools, and knows nothing about
// what any of them do.
type ToolRunner interface {
	Schemas(actor authctx.Actor) []gateway.ToolSpec
	Dispatch(ctx context.Context, actor authctx.Actor, turnID int64, call gateway.ToolCall) ToolOutcome
}

// Emitter receives a turn's events as they happen. The HTTP layer implements it
// over the durable stream buffer, so a disconnect loses nothing.
//
// Done and Failed both carry the FINAL text, not just a marker. That is the
// difference between a reliable stream and one that mostly works: if every
// delta is lost — a proxy that buffered them, a model that answered in one
// piece after its deltas were dropped, a turn that produced no deltas at all
// because it spent its budget reasoning — the terminal frame still carries the
// answer, and the reader still shows it.
type Emitter interface {
	Delta(text string)
	Reasoning(text string)
	Status(stage string, data map[string]any)
	Usage(input, output int)
	// Entities carries the records this turn read, resolved to dashboard
	// links. Sent once, after the tools have run and before the answer is
	// finished, so the reader can linkify the text as it settles.
	Entities(list []Entity)
	Done(answer string, conversationID int64)
	Failed(code Code, partial string, conversationID int64)
}

// KeyResolver returns the tenant's own Gateway virtual key, so consumption is
// billed to the منشأة that spent it rather than to the platform.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Service runs turns.
type Service struct {
	repo    Repository
	reader  Reader
	gateway gateway.Client
	tools   ToolRunner
	keys    KeyResolver
	refs    RefIssuer
	actions *actions.Flow
	storage *storage.Client
	log     *slog.Logger
}

// SetStorage installs object storage client for attachments.
func (s *Service) SetStorage(c *storage.Client) { s.storage = c }

// RefIssuer mints a branch reference for the caller, so the session context
// can name branches the way the tools accept them.
type RefIssuer func(actor authctx.Actor, branchID int64) string

// SetRefIssuer installs the branch reference minting.
func (s *Service) SetRefIssuer(r RefIssuer) { s.refs = r }

// NewService constructs the assistant service.
func NewService(repo Repository, gw gateway.Client, runner ToolRunner, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		repo:    repo,
		gateway: gw,
		tools:   runner,
		log:     log.With("module", "assistant"),
	}
	if r, ok := repo.(Reader); ok {
		s.reader = r
	}
	return s
}

// SetKeyResolver installs the tenant key lookup.
func (s *Service) SetKeyResolver(k KeyResolver) { s.keys = k }

// Repo exposes the repository to the HTTP layer, which owns conversation
// lifecycle. The service owns turns.
func (s *Service) Repo() Repository { return s.repo }

// ContextWindow reports the primary model's context size, for the usage meter.
func (s *Service) ContextWindow(ctx context.Context) int {
	if s.gateway == nil {
		return defaultContextWindow
	}
	caps, err := s.gateway.Capabilities(ctx, gateway.RolePrimary)
	if err != nil || caps.ContextWindow <= 0 {
		return defaultContextWindow
	}
	return caps.ContextWindow
}
