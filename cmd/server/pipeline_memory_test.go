package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// recorder stands in for the matching service: it records what would be filed
// and applies no rules of its own.
type recorder struct {
	writes []matching.KnowledgeWrite
	scopes []matching.Scope
}

func (r *recorder) Remember(_ context.Context, sc matching.Scope, k matching.KnowledgeWrite) error {
	r.scopes = append(r.scopes, sc)
	r.writes = append(r.writes, k)
	return nil
}

type noStore struct{}

func (noStore) LookupDecisions(context.Context, []string) (map[string]ingest.CachedDecision, error) {
	return map[string]ingest.CachedDecision{}, nil
}
func (noStore) SaveAlias(context.Context, int64, string, string, float64) error { return nil }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func importCtx(orgID int64) context.Context {
	return authctx.WithActor(context.Background(), authctx.Actor{UserID: 7, OrganizationID: orgID})
}

// An answer the vendor's upload accepted is filed as knowledge, in the
// vocabulary and the namespace the matcher reads back.
//
// This is the bug the adapter exists for: the ingest service used to be handed a
// repository whose SaveDecisions is a no-op (catalog.match_decisions became
// read-only in US8 T114), so every accepted answer was applied to the row and
// then forgotten — the supplier's memory stayed empty and the next upload paid
// for the same question again.
func TestIngestAnswersAreFiledAsKnowledge(t *testing.T) {
	rec := &recorder{}
	mem := newIngestDecisionMemory(noStore{}, rec, quiet())

	productID := int64(42)
	if err := mem.SaveDecisions(importCtx(9), []ingest.CachedDecision{{
		Key: "cache-key", NormName: "بانادول ٥٠٠", ChosenProductID: &productID,
		Confidence: 0.93, Reason: "اسم مطابق", PromptVersion: "sm-enh-v1",
		Source: "ai", Signature: "sig-1",
	}}); err != nil {
		t.Fatalf("SaveDecisions: %v", err)
	}

	if len(rec.writes) != 1 {
		t.Fatalf("filed %d answers, want 1", len(rec.writes))
	}
	got := rec.writes[0]
	if got.QuestionSignature != "sig-1" {
		t.Errorf("signature = %q, want the row's own question", got.QuestionSignature)
	}
	if got.Kind != "ai_answer" {
		t.Errorf("kind = %q, want ai_answer for a model's answer", got.Kind)
	}
	if got.TargetProductID == nil || *got.TargetProductID != productID {
		t.Errorf("target product = %v, want %d", got.TargetProductID, productID)
	}
	// The namespace has to be the workflow the vendor pipeline matches under, or
	// KnowledgeResolver skips the row when the next upload asks the question.
	if got.SourceNamespace != string(matching.WorkflowVendor) {
		t.Errorf("namespace = %q, want %q", got.SourceNamespace, matching.WorkflowVendor)
	}
	if rec.scopes[0].EffectiveOrg != 9 {
		t.Errorf("scope org = %d, want the uploader's 9", rec.scopes[0].EffectiveOrg)
	}
}

// A person's correction is filed as a human confirmation, the highest-precedence
// memory there is — that is what makes the next upload trust it.
func TestManualCorrectionsAreFiledAsHumanConfirmations(t *testing.T) {
	rec := &recorder{}
	mem := newIngestDecisionMemory(noStore{}, rec, quiet())

	productID := int64(42)
	if err := mem.SaveDecisions(importCtx(9), []ingest.CachedDecision{{
		Key: "manual:بانادول", NormName: "بانادول", ChosenProductID: &productID,
		Confidence: 1, Source: "manual", Signature: "sig-2",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 1 || rec.writes[0].Kind != "human_confirm" {
		t.Fatalf("manual correction filed as %+v, want one human_confirm", rec.writes)
	}
}

// An answer with no question signature is not filed: the cache key is not a
// signature, so a row written under it could never be looked up again.
func TestAnswersWithoutASignatureAreNotFiled(t *testing.T) {
	rec := &recorder{}
	mem := newIngestDecisionMemory(noStore{}, rec, quiet())

	productID := int64(42)
	if err := mem.SaveDecisions(importCtx(9), []ingest.CachedDecision{{
		Key: "cache-key", NormName: "بانادول", ChosenProductID: &productID, Source: "ai",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 0 {
		t.Errorf("filed %d answers with no signature; nothing would ever find them", len(rec.writes))
	}
}

// Without an organisation there is nobody to own the memory.
func TestAnswersAreNotFiledWithoutATenant(t *testing.T) {
	rec := &recorder{}
	mem := newIngestDecisionMemory(noStore{}, rec, quiet())

	productID := int64(42)
	if err := mem.SaveDecisions(context.Background(), []ingest.CachedDecision{{
		NormName: "بانادول", ChosenProductID: &productID, Source: "ai", Signature: "sig-3",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 0 {
		t.Error("an answer was filed with no organisation, i.e. visible to every tenant")
	}
}

// The tenant set on the context wins over the actor: the pipeline sets it for
// the import it is committing, which is the organisation the answers belong to.
func TestTheTenantWinsOverTheActor(t *testing.T) {
	rec := &recorder{}
	mem := newIngestDecisionMemory(noStore{}, rec, quiet())

	ctx := database.WithTenant(importCtx(11), 12)
	productID := int64(42)
	if err := mem.SaveDecisions(ctx, []ingest.CachedDecision{{
		NormName: "بانادول", ChosenProductID: &productID, Source: "ai", Signature: "sig-4",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.scopes) != 1 || rec.scopes[0].EffectiveOrg != 12 {
		t.Fatalf("filed under %+v, want the tenant 12", rec.scopes)
	}
}

// The smart order writes to the same memory, under its own workflow, so its
// answers are found by the next order and not by a vendor import.
func TestSmartOrderAnswersAreFiledUnderItsOwnWorkflow(t *testing.T) {
	rec := &recorder{}
	mem := newSmartOrderDecisionMemory(rec, quiet())

	productID := int64(42)
	if err := mem.SaveDecisions(importCtx(9), []smartorder.CachedDecision{{
		NormName: "كونجستال", ChosenProductID: &productID, Source: "ai", Signature: "sig-5",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 1 {
		t.Fatalf("filed %d answers, want 1", len(rec.writes))
	}
	if rec.writes[0].SourceNamespace != string(matching.WorkflowSmartOrder) {
		t.Errorf("namespace = %q, want %q", rec.writes[0].SourceNamespace, matching.WorkflowSmartOrder)
	}
}
