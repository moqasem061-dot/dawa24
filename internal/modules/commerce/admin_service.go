package commerce

import (
	"context"
)

// AdminSearchOrders provides cross-tenant order search.
func (s *Service) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*Order, error) {
	return s.repo.AdminSearchOrders(ctx, query, limit, offset)
}

// AdminSearchOrdersWithTotal provides paginated cross-tenant order search with tab filter and total count.
func (s *Service) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*Order, int, error) {
	return s.repo.AdminSearchOrdersWithTotal(ctx, query, tab, limit, offset)
}

// AdminOrderStats returns aggregated counts of all, direct, and negotiation orders.
func (s *Service) AdminOrderStats(ctx context.Context) (allCount, directCount, negotiationCount int, err error) {
	return s.repo.AdminOrderStats(ctx)
}

// AdminSearchOrdersFiltered provides paginated cross-tenant order search with rich filters and metadata.
func (s *Service) AdminSearchOrdersFiltered(ctx context.Context, filter AdminOrderFilter) ([]*Order, int, error) {
	return s.repo.AdminSearchOrdersFiltered(ctx, filter)
}

// AdminOrderKPIs returns aggregated metrics for the admin orders dashboard.
func (s *Service) AdminOrderKPIs(ctx context.Context) (AdminOrderKPIs, error) {
	return s.repo.AdminOrderKPIs(ctx)
}

// AdminOrderPartiesBackend supplies the buyer and seller filter selects.
type AdminOrderPartiesBackend interface {
	AdminOrderParties(ctx context.Context) (AdminOrderParties, error)
}

// AdminOrderParties returns the organisations that appear on an order.
func (s *Service) AdminOrderParties(ctx context.Context) (AdminOrderParties, error) {
	backend, ok := s.repo.(AdminOrderPartiesBackend)
	if !ok {
		return AdminOrderParties{}, nil
	}
	return backend.AdminOrderParties(ctx)
}
