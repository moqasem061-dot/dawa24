package aicapabilities

import (
	"context"
	"encoding/json"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// KeyResolver resolves tenant virtual key by organization ID.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Service coordinates AI capabilities with robust deterministic fallbacks.
type Service struct {
	gw          gateway.Client
	log         *slog.Logger
	keyResolver KeyResolver
}

// NewService creates a new AI capabilities service.
func NewService(gw gateway.Client, log *slog.Logger) *Service {
	return &Service{
		gw:  gw,
		log: log,
	}
}

// SetKeyResolver configures the tenant key resolver for per-organization Gateway billing and authorization.
func (s *Service) SetKeyResolver(r KeyResolver) {
	s.keyResolver = r
}

// MatchProduct matches an input product string against candidates using AI gateway with deterministic fallback.
func (s *Service) MatchProduct(ctx context.Context, req MatchRequest) MatchResponse {
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

	feature := req.Feature
	if feature == "" {
		feature = matchflow.FeatureCompareTool
	}

	if s.gw != nil {
		inputBytes, err := json.Marshal(req)
		if err == nil {
			gwReq := gateway.Request{
				Capability:     gateway.CapProductMatch,
				System:         "You are an expert pharmaceutical matching AI. You will be given a query medicine name and a list of candidate medicines from the official catalog. Identify the single best matching medicine from the candidate list, taking into account brand names, generic/scientific names, dosage forms (tablets, syrup, capsules), and concentrations (mg, ml, etc.).\n\nReturn ONLY a JSON object with this exact format:\n{\"matched_candidate\":\"<exact candidate name or empty string if no match>\",\"confidence_score\":<number between 0.0 and 1.0>,\"reason\":\"<brief Arabic explanation>\"}",
				Input:          string(inputBytes),
				OrganizationID: orgID,
				UserID:         userID,
				VirtualKey:     vKey,
				Feature:        feature,
			}
			resp, err := s.gw.Invoke(ctx, gwReq)
			if err == nil && resp != nil && resp.Content != "" {
				cleanContent := strings.TrimSpace(resp.Content)
				cleanContent = strings.TrimPrefix(cleanContent, "```json")
				cleanContent = strings.TrimPrefix(cleanContent, "```")
				cleanContent = strings.TrimSuffix(cleanContent, "```")
				cleanContent = strings.TrimSpace(cleanContent)

				var matchResp MatchResponse
				if err := json.Unmarshal([]byte(cleanContent), &matchResp); err == nil {
					matchResp.Source = "ai_gateway"
					s.log.InfoContext(ctx, "ai gateway product match succeeded",
						"query", req.QueryName,
						"matched", matchResp.MatchedCandidate,
						"score", matchResp.ConfidenceScore,
						"req_id", resp.RequestID,
						"model", resp.Model,
					)
					return matchResp
				} else {
					s.log.WarnContext(ctx, "failed to unmarshal gateway match response", "content", resp.Content, "err", err)
				}
			} else {
				s.log.WarnContext(ctx, "gateway capability execution failed, falling back to deterministic", "capability", gateway.CapProductMatch, "err", err)
			}
		}
	}

	// Deterministic Fallback using Arabic string similarity
	normQuery := arabic.Normalize(req.QueryName)
	var bestCandidate string
	var highestScore float64

	for _, cand := range req.Candidates {
		normCand := arabic.Normalize(cand)
		score := arabic.Similarity(normQuery, normCand)
		if score > highestScore {
			highestScore = score
			bestCandidate = cand
		}
	}

	return MatchResponse{
		MatchedCandidate: bestCandidate,
		ConfidenceScore:  highestScore,
		Source:           "deterministic_fallback",
	}
}

// MatchCandidate satisfies the Ingest matcher interface.
func (s *Service) MatchCandidate(ctx context.Context, query string, candidateNames []string) (string, float64) {
	resp := s.MatchProduct(ctx, MatchRequest{
		QueryName:  query,
		Candidates: candidateNames,
	})
	return resp.MatchedCandidate, resp.ConfidenceScore
}

// ExpandSearch expands pharmaceutical query keywords using AI gateway with deterministic fallback.
func (s *Service) ExpandSearch(ctx context.Context, req QueryExpansionRequest) QueryExpansionResponse {
	if s.gw != nil && s.gw.Enabled() {
		gwReq := gateway.Request{
			Capability: gateway.CapSearchExpand,
			Input:      req.Query,
		}
		resp, err := s.gw.Invoke(ctx, gwReq)
		if err == nil && resp != nil && resp.Content != "" {
			var expResp QueryExpansionResponse
			if err := json.Unmarshal([]byte(resp.Content), &expResp); err == nil {
				expResp.Source = "ai_gateway"
				return expResp
			}
		}
	}

	// Deterministic Fallback: normalize Arabic keywords and create basic variations
	clean := arabic.Normalize(req.Query)
	terms := strings.Fields(clean)
	var expanded []string
	for _, t := range terms {
		expanded = append(expanded, t)
		if strings.HasPrefix(t, i18n.TDefault("w4_mod.s_229_229")) && len(t) > 4 {
			expanded = append(expanded, strings.TrimPrefix(t, i18n.TDefault("w4_mod.s_229_229")))
		}
	}

	return QueryExpansionResponse{
		OriginalTerms: req.Query,
		ExpandedTerms: expanded,
		Source:        "deterministic_fallback",
	}
}
