package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// GetPreferences returns a user's preferences, inserting defaults on first read.
func (r *Repository) GetPreferences(ctx context.Context, userID int64) (*identity.UserPreferences, error) {
	p := &identity.UserPreferences{
		UserID:               userID,
		Theme:                "light",
		NotificationChannels: map[string]bool{"email": true, "sms": false, "push": true},
		NotificationTopics:   map[string]bool{"offers": true, "blog": false, "newsletter": true},
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT theme, notification_channels, notification_topics, marketing_consent, updated_at FROM profile.user_preferences WHERE user_id = $1;`
		err := tx.QueryRow(txCtx, query, userID).Scan(&p.Theme, &p.NotificationChannels, &p.NotificationTopics, &p.MarketingConsent, &p.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return nil // defaults returned
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdatePreferences upserts a user's preferences.
func (r *Repository) UpdatePreferences(ctx context.Context, p *identity.UserPreferences) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO profile.user_preferences (user_id, theme, notification_channels, notification_topics, marketing_consent, updated_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (user_id) DO UPDATE SET theme = EXCLUDED.theme, notification_channels = EXCLUDED.notification_channels, notification_topics = EXCLUDED.notification_topics, marketing_consent = EXCLUDED.marketing_consent, updated_at = now();
		`
		_, err := tx.Exec(txCtx, query, p.UserID, p.Theme, p.NotificationChannels, p.NotificationTopics, p.MarketingConsent)
		return err
	})
}
