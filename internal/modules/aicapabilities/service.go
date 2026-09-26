package aicapabilities

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/arabic"
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
			if err := json.Unmarshal([]byte(resp.Content), &expResp); err == nil { // nolint:errswallow: unparseable model output falls through to the deterministic fallback below
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
