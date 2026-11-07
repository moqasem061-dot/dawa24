package erp

import "context"

func (s *Service) HealthData(ctx context.Context, principal Principal) (HealthInfo, error) {
	return s.repo.HealthData(ctx, principal)
}
