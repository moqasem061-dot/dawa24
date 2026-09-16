package aicapabilities

// Match enhancement.
//
// This capability answers one question and only one: for a pharmacy line the
// deterministic engine could not settle, which product in the catalogue window
// supplied with the request is the same medicine — or none of them?
//
// Everything about its shape follows from three facts.
//
// **The catalogue does not fit in a prompt.** Twenty thousand products at forty
// Arabic characters each is roughly a megabyte and several hundred thousand
// tokens, re-sent on every import of every pharmacy. So the request carries a
// *window*: the union of the candidates retrieved for the lines in it. Two
// hundred lines about antihypertensives retrieve heavily overlapping products,
// and de-duplicating that union is what turns a five-thousand-row catalogue
// block into a fifteen-hundred-row one.
//
// **The window is shared, not per item.** Each item names its own shortlist by
// id, but the model may answer with any id in the window. That costs nothing
// extra and repairs the most common retrieval failure there is: the right
// product was retrieved — for the line above.
//
// **The prompt is generated, never authored by a model.** RenderEnhanceInput is
// a pure function of the request. The same lines and the same window always
// produce byte-identical input, which is what makes the decision cache sound and
// what makes a prompt regression show up in a diff rather than in a bill.

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// EnhancePromptVersion is the version of the question this capability asks.
//
// It is declared once, in internal/shared/matchflow, and re-exported here for
// call sites that already name it. It used to be declared here *and* in the
// smart order pipeline *and* in the vendor import, and the three had drifted:
// the vendor filed its answers under v3 while rendering the v4 prompt, so two
// features maintained two disjoint caches for one question.
const EnhancePromptVersion = matchflow.PromptVersion

// CatalogEntry is one catalogue product as the model sees it.
//
// Arabic name first, because that is what the pharmacy wrote and what the
// catalogue is authored in. The English name is carried too and earns its
// tokens: transliteration is where Arabic pharmacy matching actually fails, and
// i18n.TDefault("w4_mod.s_203_203") against i18n.TDefault("w4_mod.s_204_204") is obvious the moment both rows show `abilify`.
type CatalogEntry = matchflow.CatalogEntry

// EnhanceItem is one line the deterministic engine could not settle.
type EnhanceItem = matchflow.Item

// EnhanceRequest is one batch: a catalogue window and the lines to resolve
// against it.
type EnhanceRequest = matchflow.Batch

// EnhanceDecision is one answer. A nil ProductID means "none of these", which is
// a correct and useful answer and is recorded as such.
type EnhanceDecision = matchflow.Decision

// enhanceSchema constrains the response at the protocol level where the Gateway
// supports it. It is a belt to the prompt's braces: the prompt still specifies
// the shape in full, because a Gateway or model that ignores the schema must
// still produce something parseable.
func enhanceSchema() map[string]any {
	return map[string]any{
		"name":   "dawa24_match_enhancement",
		"strict": true,
		"schema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"results"},
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"ref", "product_id", "confidence"},
						"properties": map[string]any{
							"ref":        map[string]any{"type": "integer"},
							"product_id": map[string]any{"type": []string{"integer", "null"}},
							"confidence": map[string]any{"type": "number"},
							"reason":     map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}

// EnhanceMatches resolves one batch.
//
// There is no deterministic fallback, and that is deliberate: the deterministic
// engine has already run and produced the result this is trying to improve on.
// Returning an error leaves that result standing, which is the correct
// degradation — a pharmacy must be able to order when the Gateway is down.
func (s *Service) EnhanceMatches(ctx context.Context, req EnhanceRequest) ([]EnhanceDecision, error) {
	if len(req.Items) == 0 {
		return nil, nil
	}
	if s.gw == nil {
		return nil, fmt.Errorf("aicapabilities: gateway not configured")
	}
	if len(req.Catalog) == 0 {
		// Nothing to choose from. Asking anyway would invite the model to
		// invent an id, which is the one failure that must never happen.
		return nil, fmt.Errorf("aicapabilities: empty catalogue window")
	}

	orgID := req.OrganizationID
	userID := req.UserID
	if orgID <= 0 {
		if actor, ok := authctx.From(ctx); ok {
			orgID = actor.OrgID
			if orgID <= 0 {
				orgID = actor.OrganizationID
			}
			if userID <= 0 {
				userID = actor.UserID
			}
		}
	}
	if orgID <= 0 {
		if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
			orgID = tid
		}
	}
	var vKey string
	if s.keyResolver != nil && orgID > 0 {
		if k, err := s.keyResolver(ctx, orgID); err == nil && k != "" {
			vKey = k
		}
	}

	gwReq := gateway.Request{
		Capability:     gateway.CapMatchEnhance,
		System:         enhanceSystemPrompt,
		Input:          RenderEnhanceInput(req),
		Schema:         enhanceSchema(),
		OrganizationID: orgID,
		UserID:         userID,
		VirtualKey:     vKey,
		Feature:        req.Feature,
		MaxTokens:      enhanceMaxTokens(len(req.Items)),
	}

	// Do not retry the same logical batch with a different wire format. The
	// prompt already specifies strict JSON, and the deterministic result is a
	// safe fallback when a deployment rejects schemas.
	resp, err := s.gw.Invoke(ctx, gwReq)
	if err != nil {
		s.log.WarnContext(ctx, "match enhancement failed; deterministic results stand",
			"items", len(req.Items), "catalog", len(req.Catalog), "err", err)
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("aicapabilities: empty enhancement response")
	}

	decisions, err := DecodeEnhanceResponse(resp.Content)
	if err != nil {
		s.log.WarnContext(ctx, "could not read enhancement response",
			"items", len(req.Items), "err", err)
		return nil, err
	}

	s.log.InfoContext(ctx, "match enhancement resolved",
		"items", len(req.Items), "catalog", len(req.Catalog), "decisions", len(decisions),
		"req_id", resp.RequestID, "model", resp.Model,
		"in_tok", resp.InputTok, "out_tok", resp.OutputTok)
	return decisions, nil
}

// enhanceMaxTokens sizes the completion budget to the batch.
//
// A budget that is too small truncates the JSON mid-array and loses every
// decision in the batch, including the ones already written; one that is too
// large costs nothing, because the model stops when it stops. So this is
// deliberately generous.
func enhanceMaxTokens(items int) int {
	n := items*28 + 512
	if n > 60000 {
		n = 60000
	}
	return n
}
