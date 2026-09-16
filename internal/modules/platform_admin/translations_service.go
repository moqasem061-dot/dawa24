package platformadmin

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ListTranslations returns paginated translations matching filter.
func (s *Service) ListTranslations(ctx context.Context, filter TranslationFilter) ([]*Translation, int, error) {
	return s.repo.ListTranslations(ctx, filter)
}

// GetTranslationByKey returns a translation entry by key.
func (s *Service) GetTranslationByKey(ctx context.Context, key string) (*Translation, error) {
	return s.repo.GetTranslationByKey(ctx, key)
}

// UpdateTranslation saves a translation override to DB and immediately applies it to the in-memory i18n engine.
func (s *Service) UpdateTranslation(ctx context.Context, key, textAR, textEN, desc string) error {
	parts := strings.SplitN(key, ".", 2)
	ns := "common"
	if len(parts) > 1 {
		ns = parts[0]
	}

	t := &Translation{
		Key:         key,
		Namespace:   ns,
		TextAR:      textAR,
		TextEN:      textEN,
		Description: desc,
		IsCustom:    true,
	}

	if err := s.repo.UpsertTranslation(ctx, t); err != nil {
		return err
	}

	// Apply immediately to the live running process
	i18n.SetOverride(key, i18n.New(textAR, textEN))
	s.log.InfoContext(ctx, "translation override updated", "key", key)
	return nil
}

// ResetTranslation removes a translation override and restores default code text.
func (s *Service) ResetTranslation(ctx context.Context, key string) error {
	if err := s.repo.DeleteTranslation(ctx, key); err != nil {
		return err
	}
	i18n.RemoveOverride(key)
	s.log.InfoContext(ctx, "translation override reset", "key", key)
	return nil
}

// SyncAllDefaultTranslations seeds all in-code default keys to the database.
func (s *Service) SyncAllDefaultTranslations(ctx context.Context) error {
	entries := i18n.GetAllKeyEntries()
	for _, e := range entries {
		existing, err := s.repo.GetTranslationByKey(ctx, e.Key)
		if err != nil {
			return err
		}
		if existing == nil {
			t := &Translation{
				Key:         e.Key,
				Namespace:   e.Namespace,
				TextAR:      e.TextAR,
				TextEN:      e.TextEN,
				Description: e.Description,
				IsCustom:    false,
			}
			if err := s.repo.UpsertTranslation(ctx, t); err != nil {
				return err
			}
		}
	}
	s.log.InfoContext(ctx, "synced default translations to database", "count", len(entries))
	return nil
}

// GetTranslationStats returns summary stats for translations.
func (s *Service) GetTranslationStats(ctx context.Context) (*TranslationStats, error) {
	return s.repo.GetTranslationStats(ctx)
}

// SyncRuntimeOverrides loads custom translations from database into memory engine.
func (s *Service) SyncRuntimeOverrides(ctx context.Context) error {
	overrides, err := s.repo.LoadAllCustomTranslations(ctx)
	if err != nil {
		return err
	}
	i18n.RegisterOverrides(overrides)
	s.log.InfoContext(ctx, "loaded custom translations into runtime engine", "count", len(overrides))
	return nil
}
