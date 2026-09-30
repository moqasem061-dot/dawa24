# Contract: Matching Service and Consumer Ports

The service is `internal/modules/catalog/matching.Service`. Consumers never import it. Each declares its own port with the subset it needs, and `cmd/server/wire_matching.go` adapts. All methods run tenant-scoped via `db.InTx`/`InReadTx` with the **effective** organization resolved by the caller (FR-042). Admin-assisted flows pass `EffectiveOrg` explicitly, and the service rejects a context whose actor lacks cross-org permission for it.

```go
type Scope struct {
    EffectiveOrg   int64   // target organization (never the admin's own by default)
    ActorUserID    int64
    SourceNamespace string // e.g. "supplier:812"; "*" when not applicable
    Workflow       Workflow   // vendor|savings|smartorder|catalog|compare
    EntryPoint     string     // registry ID (research R1)
}

type PrepareRequest struct {
    Scope       Scope
    ConsumerRef string                 // "ingest.catalog_imports:123"
    SourceRev   int                    // mapping/source revision
    Rows        []productmatch.Row     // with Mapping + declared identifier namespaces
    Required    productmatch.Granularity
}
type PrepareResult struct {
    RunID     int64
    Decisions []matchflow.RowDecision     // same order as Rows
    Summary   matchflow.RunCounts      // reconciles 1:1 with rows (FR-094)
}

type Service interface {
    // Deterministic; no remote calls; saves nothing itself — the consumer
    // persists decisions in its own rows in the same tx, then calls MarkReviewable.
    Prepare(ctx context.Context, req PrepareRequest) (PrepareResult, error)
    MarkReviewable(ctx context.Context, runID int64) error

    Rematch(ctx context.Context, s Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error) // ≤1 s p95
    Search(ctx context.Context, s Scope, q string, row productmatch.Row, limit int) ([]matchflow.CandidateView, error) // FR-044/045
    Review(ctx context.Context, s Scope, a ReviewAction) (matchflow.RowDecision, error) // select/correct/reject/exclude/undo; gate-enforced
    PreviewBulk(ctx context.Context, s Scope, a ReviewAction, rows []RowRef) (BulkPreview, error)                   // FR-046

    // Optional AI. Returns immediately; applies via the consumer's Applier port.
    StartEnhancement(ctx context.Context, runID int64, rows []EnhanceRow) (EnhanceHandle, error)
    StopEnhancement(ctx context.Context, runID int64) error

    Revalidate(ctx context.Context, s Scope, ref RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) // commit-time

    // Knowledge
    Remember(ctx context.Context, s Scope, k KnowledgeWrite) error   // from confirmed/rejected review actions
    ListKnowledge(ctx context.Context, s Scope, f KnowledgeFilter) (Page[Knowledge], error)
    RevokeKnowledge(ctx context.Context, s Scope, id int64) error
    PromoteKnowledge(ctx context.Context, s Scope, id int64) error   // privileged; strips tenant context (FR-054)
}

// Implemented by each consumer so the service can apply AI results without
// knowing the consumer's tables.
type Applier interface {
    // Must use the compare-and-set rule in decision-record.md; returns false when obsolete.
    ApplyAI(ctx context.Context, ref RowRef, expectedRev int, runID int64, d matchflow.RowDecision) (bool, error)
}
```

## Behavioural guarantees

- `Prepare` with the AI gateway disabled or unconfigured returns in the same time as with AI enabled. AI is never awaited (FR-004, FR-081).
- Identical rows (same `QuestionSignature`) are evaluated once. Each row still receives its own `Decision` with its own revision (FR-034).
- Errors: a catalogue load failure → every row `unavailable` + `catalog_unavailable`, never `no_candidate` (FR-005, SC-004). A per-row panic → that row `invalid_input` + `numeric_uninterpretable`/`invalid_row`, and the rest continue (FR-014).
- `Review` rejects `select` on a candidate with hard conflicts unless the same action includes a fact correction that removes it (FR-045, US7-4).
