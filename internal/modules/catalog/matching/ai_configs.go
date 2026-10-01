package matching

import (
	"context"
)

// ListAIConfigurations returns all registered AI matching configurations (US10 T134).
func (s *serviceImpl) ListAIConfigurations(ctx context.Context) ([]AIConfiguration, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	return s.repo.ListAIConfigurations(ctx)
}

// GetAIConfiguration retrieves an AI configuration by ID (US10 T134).
func (s *serviceImpl) GetAIConfiguration(ctx context.Context, id string) (*AIConfiguration, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	return s.repo.GetAIConfiguration(ctx, id)
}

// UpdateAIConfigurationStatus transitions an AI configuration status with audit logging (US10 T134).
func (s *serviceImpl) UpdateAIConfigurationStatus(ctx context.Context, id string, newStatus string, changedBy string, reason string) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	return s.repo.UpdateAIConfigurationStatus(ctx, id, newStatus, changedBy, reason)
}
