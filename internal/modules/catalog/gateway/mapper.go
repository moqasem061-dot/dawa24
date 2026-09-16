// Package gateway adapts the platform AI Gateway to the catalogue's AIMapper
// port.
//
// It lives beside the catalogue module rather than inside it so the domain
// depends on the capability it needs and not on a transport. The catalogue
// knows nothing about virtual keys, budgets, or circuit breakers; this package
// knows nothing about products beyond the request shape it is handed.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// KeyResolver resolves an organisation's Gateway virtual key, so AI spend is
// attributed and capped per tenant rather than against one platform key.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Mapper answers the import's three mapping questions through the Gateway.
type Mapper struct {
	client      gateway.Client
	log         *slog.Logger
	keyResolver KeyResolver
}

// NewMapper wires the Gateway client into the catalogue's AIMapper port.
func NewMapper(client gateway.Client, log *slog.Logger) *Mapper {
	return &Mapper{client: client, log: log.With("component", "catalog_ai_mapper")}
}

// SetKeyResolver installs per-organisation Gateway billing.
func (m *Mapper) SetKeyResolver(r KeyResolver) { m.keyResolver = r }

// Available reports whether the Gateway can be called at all right now. The
// wizard uses it to explain why the AI switch is unavailable rather than
// offering a toggle that silently does nothing.
func (m *Mapper) Available(context.Context) bool {
	return m != nil && m.client != nil && m.client.Enabled()
}

// MapColumns asks which spreadsheet column holds which product field.
//
// It runs on the fast tier: the request is a header and a few sample rows, and
// the admin is waiting on it before anything else can start.
func (m *Mapper) MapColumns(
	ctx context.Context, req catalog.ColumnMapRequest,
) (catalog.ColumnMapResult, error) {
	content, err := m.invoke(ctx, aiCall{
		capability: gateway.CapColumnDetect,
		system:     catalog.ColumnMapPrompt(),
		schema:     catalog.ColumnMapSchema(),
		payload:    req,
		orgID:      req.OrganizationID,
		userID:     req.UserID,
		label:      "column mapping",
	})
	if err != nil {
		return catalog.ColumnMapResult{}, err
	}

	result, err := catalog.DecodeColumnMap(content)
	if err != nil {
		m.log.WarnContext(ctx, "column map response unreadable", "error", err)
		return catalog.ColumnMapResult{}, err
	}
	m.log.InfoContext(ctx, "columns mapped by ai", "assigned", len(result.Columns))
	return result, nil
}

// MapValues translates a file's distinct taxonomy words onto the catalogue's.
//
// One request per taxonomy per import, regardless of how many rows use those
// words — which is what keeps a fifty-thousand-row file to three AI calls in
// total.
func (m *Mapper) MapValues(
	ctx context.Context, req catalog.ValueMapRequest,
) (catalog.ValueMapResult, error) {
	content, err := m.invoke(ctx, aiCall{
		capability: gateway.CapProductMatch,
		system:     catalog.ValueMapPrompt(),
		schema:     catalog.ValueMapSchema(),
		payload:    req,
		orgID:      req.OrganizationID,
		userID:     req.UserID,
		label:      "value mapping " + string(req.Kind),
	})
	if err != nil {
		return catalog.ValueMapResult{}, err
	}

	result, err := catalog.DecodeValueMap(content)
	if err != nil {
		m.log.WarnContext(ctx, "value map response unreadable", "kind", req.Kind, "error", err)
		return catalog.ValueMapResult{}, err
	}
	m.log.InfoContext(ctx, "values mapped by ai",
		"kind", req.Kind, "sources", len(req.Sources), "matches", len(result.Matches))
	return result, nil
}

// aiCall is one Gateway invocation's parameters, bundled so the shared plumbing
// below takes an argument rather than seven.
type aiCall struct {
	capability gateway.Capability
	system     string
	schema     map[string]any
	payload    any
	orgID      int64
	userID     int64
	label      string
}

// invoke performs one call and returns its raw content.
func (m *Mapper) invoke(ctx context.Context, call aiCall) (string, error) {
	if m == nil || m.client == nil {
		return "", gateway.ErrDisabled
	}

	input, err := catalog.EncodeJSON(call.payload)
	if err != nil {
		return "", err
	}

	resp, err := m.client.Invoke(ctx, gateway.Request{
		Capability:     call.capability,
		System:         call.system,
		Input:          input,
		Schema:         call.schema,
		OrganizationID: call.orgID,
		UserID:         call.userID,
		VirtualKey:     m.virtualKey(ctx, call.orgID),
		// Attribution for the AI usage ledger: these are the column and
		// taxonomy mapping calls an import makes before it reads a single row.
		Feature: matchflow.FeatureColumnDetect,
	})
	if err != nil {
		m.log.WarnContext(ctx, "ai mapping unavailable, falling back to exact matching",
			"call", call.label, "error", err)
		return "", translateGatewayError(err)
	}
	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("catalog gateway: empty %s response", call.label)
	}
	return resp.Content, nil
}

func (m *Mapper) virtualKey(ctx context.Context, orgID int64) string {
	if m.keyResolver == nil || orgID <= 0 {
		return ""
	}
	key, err := m.keyResolver(ctx, orgID)
	if err != nil {
		m.log.WarnContext(ctx, "tenant gateway key unavailable, using platform key",
			"org_id", orgID, "error", err)
		return ""
	}
	return key
}

// translateGatewayError maps the Gateway's closed error set onto the conditions
// the import knows how to explain.
//
// Two of them are worth telling an admin apart from an outage: a spent budget
// and a rejected key are both things an operator can go and fix, and reporting
// either as "the service is unavailable" sends them looking in the wrong place.
func translateGatewayError(err error) error {
	switch {
	case errors.Is(err, gateway.ErrQuotaExceeded):
		return fmt.Errorf("%w: %v", catalog.ErrAIQuotaExceeded, err)
	case errors.Is(err, gateway.ErrUnauthorized), errors.Is(err, gateway.ErrDisabled):
		return fmt.Errorf("%w: %v", catalog.ErrAIUnauthorized, err)
	default:
		return err
	}
}
