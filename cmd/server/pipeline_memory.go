package main

// The import pipelines' decision memory, wired to the store that owns it.
//
// Three pipelines keep a decision cache — the vendor catalogue import, the smart
// order and the saving list — and all three were handed a repository whose
// SaveDecisions is a documented no-op: catalog.match_decisions became read-only
// when catalog.match_knowledge replaced it (US8 T114, and
// test/match_decisions_readonly_test.go enforces it). The comment on those
// no-ops said writes "route through matching.Service", but nothing did the
// routing for the import pipelines: only the catalog import and the saving list
// got the matchflow.Memory adapter below, so an answer the vendor's upload paid
// for was applied to the row and then filed nowhere.
//
// That had three visible costs: ذاكرة قرارات المطابقة showed nothing after an
// upload, the next upload asked the model the same questions again, and a vendor
// who corrected a row by hand taught the platform nothing — the correction was
// dropped with the rest.
//
// These two adapters are that routing. They are the only place that knows both
// an import module's vocabulary and the matching service, which is what the
// composition root is for: `modules/A may not import modules/B`, and the
// alternative — teaching each repository to write knowledge — would put a second
// module's table in its SQL.

import (
	"context"
	"log/slog"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ingestMemoryStore is the half of the ingest repository that reads the legacy
// cache and writes the alias ledger.
type ingestMemoryStore interface {
	LookupDecisions(ctx context.Context, keys []string) (map[string]ingest.CachedDecision, error)
	SaveAlias(ctx context.Context, productID int64, alias string, source string, confidence float64) error
}

// knowledgeWriter files one answer in the shared match knowledge.
//
// The matching service satisfies it and is what production passes; it is
// declared here so the routing can be tested with a recorder instead of standing
// up a database and a catalogue index.
type knowledgeWriter interface {
	Remember(ctx context.Context, sc matching.Scope, k matching.KnowledgeWrite) error
}

// ingestDecisionMemory is the vendor catalogue import's MatchMemory.
type ingestDecisionMemory struct {
	store  ingestMemoryStore
	writer knowledgeWriter
	log    *slog.Logger
}

func newIngestDecisionMemory(store ingestMemoryStore, writer knowledgeWriter, log *slog.Logger) ingest.MatchMemory {
	return &ingestDecisionMemory{store: store, writer: writer, log: log}
}

func (m *ingestDecisionMemory) LookupDecisions(
	ctx context.Context, keys []string,
) (map[string]ingest.CachedDecision, error) {
	return m.store.LookupDecisions(ctx, keys)
}

func (m *ingestDecisionMemory) SaveAlias(
	ctx context.Context, productID int64, alias, source string, confidence float64,
) error {
	return m.store.SaveAlias(ctx, productID, alias, source, confidence)
}

func (m *ingestDecisionMemory) SaveDecisions(ctx context.Context, decisions []ingest.CachedDecision) error {
	scope, ok := memoryScope(ctx, matching.WorkflowVendor, "ingest.ai_enhance")
	if !ok {
		return nil
	}
	for _, d := range decisions {
		m.remember(ctx, scope, d.Signature, d.NormName, d.ChosenProductID, d.Source)
	}
	return nil
}

// remember files one answer as knowledge, in the vocabulary the matcher reads.
func (m *ingestDecisionMemory) remember(
	ctx context.Context, sc matching.Scope, signature, name string, productID *int64, source string,
) {
	if m.writer == nil {
		return
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		// No signature is a row the stage never saw parsed. Filing it under the
		// cache key would be invisible to every future lookup, so it is counted
		// and dropped rather than written as dead weight.
		m.log.DebugContext(ctx, "import decision memory: no question signature, not remembered",
			"name", name, "source", source)
		return
	}
	if err := m.writer.Remember(ctx, sc, matching.KnowledgeWrite{
		SourceNamespace:   string(sc.Workflow),
		QuestionSignature: signature,
		Kind:              memoryKind(source),
		TargetProductID:   productID,
		PolicyVersion:     productmatch.DefaultPolicy().Version,
		NormalizerVersion: productmatch.NormalizerVersion,
		DisplayText:       name,
	}); err != nil {
		// A memory write that fails must not fail the import: the rows were
		// applied, and the next upload simply pays for the answer again.
		m.log.WarnContext(ctx, "import decision memory: could not remember an answer",
			"name", name, "error", err)
	}
}

// memoryKind maps a decision's provenance onto a knowledge kind. A person's
// choice is a human confirmation — the highest-precedence memory there is — and
// anything else here came from a model.
func memoryKind(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual", "admin":
		return "human_confirm"
	default:
		return "ai_answer"
	}
}

// smartOrderDecisionMemory is the smart order's DecisionMemoryWriter.
type smartOrderDecisionMemory struct {
	writer knowledgeWriter
	log    *slog.Logger
}

func newSmartOrderDecisionMemory(writer knowledgeWriter, log *slog.Logger) smartorder.DecisionMemoryWriter {
	return &smartOrderDecisionMemory{writer: writer, log: log}
}

func (m *smartOrderDecisionMemory) SaveDecisions(ctx context.Context, decisions []smartorder.CachedDecision) error {
	scope, ok := memoryScope(ctx, matching.WorkflowSmartOrder, "smartorder.ai_enhance")
	if !ok || m.writer == nil {
		return nil
	}
	for _, d := range decisions {
		signature := strings.TrimSpace(d.Signature)
		if signature == "" {
			continue
		}
		if err := m.writer.Remember(ctx, scope, matching.KnowledgeWrite{
			SourceNamespace:   string(scope.Workflow),
			QuestionSignature: signature,
			Kind:              memoryKind(d.Source),
			TargetProductID:   d.ChosenProductID,
			PolicyVersion:     productmatch.DefaultPolicy().Version,
			NormalizerVersion: productmatch.NormalizerVersion,
			DisplayText:       d.NormName,
		}); err != nil {
			m.log.WarnContext(ctx, "smart order decision memory: could not remember an answer",
				"name", d.NormName, "error", err)
		}
	}
	return nil
}

// memoryScope is the organisation and workflow a remembered answer belongs to.
//
// The tenant first: both pipelines set it deliberately for the import or order
// they are answering. The actor is the fallback, for the paths that run on the
// request context. Without either there is nobody to own the memory, and writing
// it platform-wide would hand one tenant's answers to every other tenant.
func memoryScope(ctx context.Context, workflow matching.Workflow, entryPoint string) (matching.Scope, bool) {
	sc := matching.Scope{Workflow: workflow, EntryPoint: entryPoint}
	if actor, ok := authctx.From(ctx); ok {
		sc.ActorUserID = actor.UserID
	}
	if tid, ok := database.TenantFrom(ctx); ok {
		sc.EffectiveOrg = tid
	}
	if sc.EffectiveOrg <= 0 {
		if actor, ok := authctx.From(ctx); ok {
			sc.EffectiveOrg = actor.OrganizationID
		}
	}
	return sc, sc.EffectiveOrg > 0
}
