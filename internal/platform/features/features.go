// Package features provides real-time, high-performance in-memory feature flag
// evaluation and HTTP route middleware.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// FeatureFlag is the domain model representing a toggleable system feature.
type FeatureFlag struct {
	Key         string            `json:"key"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	IsEnabled   bool              `json:"is_enabled"`
	UpdatedBy   *int64            `json:"updated_by,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Engine manages cached flags with automatic background refresh.
type Engine struct {
	db    *database.DB
	log   *slog.Logger
	mu    sync.RWMutex
	flags map[string]bool
	list  []FeatureFlag
}

var globalEngine *Engine

// Init initializes the global feature flag engine.
func Init(ctx context.Context, db *database.DB, log *slog.Logger) (*Engine, error) {
	e := &Engine{
		db:    db,
		log:   log,
		flags: make(map[string]bool),
		list:  make([]FeatureFlag, 0),
	}

	if err := e.Reload(ctx); err != nil {
		log.Warn("features: initial flag load failed, using default true", "error", err)
	}

	globalEngine = e

	// Background ticker to reload every 60 seconds
	go func() {
		defer func() {
			if r := recover(); r != nil {
				e.log.Error("features reload ticker panicked", "panic", r)
			}
		}()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.Reload(ctx); err != nil {
					e.log.Warn("features: background reload failed", "error", err)
				}
			}
		}
	}()

	return e, nil
}

var defaultCoreFlags = []FeatureFlag{
	{Key: "jobs.enabled", Name: map[string]string{"ar": "قسم الوظائف والتوظيف", "en": "Jobs & Careers"}, Description: map[string]string{"ar": "تفعيل قسم الوظائف وإعلانات التوظيف في الموقع والفوتر", "en": "Job board and postings"}, IsEnabled: true},
	{Key: "jobs.seeker_accounts", Name: map[string]string{"ar": "حسابات باحث عن عمل", "en": "Job Seeker Accounts"}, Description: map[string]string{"ar": "السماح بالتسجيل في المنصة كباحث عن عمل صيدلاني أو مهني", "en": "Allow job seeker registration"}, IsEnabled: true},
	{Key: "reviews.enabled", Name: map[string]string{"ar": "التقييمات ومراجعات الجودة", "en": "Reviews & Quality Ratings"}, Description: map[string]string{"ar": "تفعيل التقييم متعدد المعايير للموردين والصيدليات", "en": "Multi-criteria reviews"}, IsEnabled: true},
	{Key: "offers.enabled", Name: map[string]string{"ar": "العروض والتخفيضات", "en": "Promotional Offers"}, Description: map[string]string{"ar": "تفعيل قسم العروض والخصومات الخاصة", "en": "Promotions and flash sales"}, IsEnabled: true},
	{Key: "finder.enabled", Name: map[string]string{"ar": "دليل وباحث الأدوية الذكي", "en": "Product Finder"}, Description: map[string]string{"ar": "تفعيل باحث الأدوية والمثائل والبدائل", "en": "Medicine and active ingredient finder"}, IsEnabled: true},
	{Key: "services.enabled", Name: map[string]string{"ar": "الخدمات المؤسسية", "en": "Institutional Services"}, Description: map[string]string{"ar": "تفعيل طلبات الخدمات المؤسسية للمستشفيات والشركات", "en": "Corporate and institutional services"}, IsEnabled: true},
	{Key: "compare.enabled", Name: map[string]string{"ar": "مقارنة خطط التوريد", "en": "Plan Comparison"}, Description: map[string]string{"ar": "تفعيل جداول مقارنة الخطط والاشتراكات", "en": "Subscription plan comparisons"}, IsEnabled: true},
	{Key: KeyERPEnabled, Name: map[string]string{"ar": "تكامل الأنظمة (ERP)", "en": "Systems integration (ERP)"}, Description: map[string]string{"ar": "تشغيل أو إيقاف تكامل الأنظمة لكل الصيدليات والموردين", "en": "Turns the ERP integration on or off for every pharmacy and supplier"}, IsEnabled: true},
}

// KeyERPEnabled is the runtime switch for the ERP integration. The deployment
// switch (ERP_API_ENABLED) still decides whether the integration exists at
// all; this key turns an existing integration off and on without a restart.
const KeyERPEnabled = "erp.enabled"

// Reload queries the database for all feature flags and updates in-memory cache.
func (e *Engine) Reload(ctx context.Context) error {
	newFlags := make(map[string]bool)
	var newList []FeatureFlag

	// Populate defaults first
	for _, df := range defaultCoreFlags {
		newFlags[df.Key] = df.IsEnabled
		newList = append(newList, df)
	}

	if e.db != nil {
		loadErr := e.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			query := `SELECT key, name, description, is_enabled, updated_by, updated_at FROM platform_admin.feature_flags;`
			rows, err := tx.Query(txCtx, query)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var key string
				var nameBytes, descBytes []byte
				var isEnabled bool
				var updatedBy *int64
				var updatedAt time.Time

				if err := rows.Scan(&key, &nameBytes, &descBytes, &isEnabled, &updatedBy, &updatedAt); err != nil {
					return err
				}

				newFlags[key] = isEnabled

				name := make(map[string]string)
				desc := make(map[string]string)
				_ = json.Unmarshal(nameBytes, &name)
				_ = json.Unmarshal(descBytes, &desc)

				// Update or add in list
				found := false
				for i := range newList {
					if newList[i].Key == key {
						newList[i].IsEnabled = isEnabled
						if len(name) > 0 {
							newList[i].Name = name
						}
						if len(desc) > 0 {
							newList[i].Description = desc
						}
						newList[i].UpdatedBy = updatedBy
						newList[i].UpdatedAt = updatedAt
						found = true
						break
					}
				}
				if !found {
					newList = append(newList, FeatureFlag{
						Key:         key,
						Name:        name,
						Description: desc,
						IsEnabled:   isEnabled,
						UpdatedBy:   updatedBy,
						UpdatedAt:   updatedAt,
					})
				}
			}
			return rows.Err()
		})
		if loadErr != nil {
			// Keep the flags already in memory: replacing them with the
			// defaults would switch a feature an admin turned off back on.
			return loadErr
		}
	}

	e.mu.Lock()
	e.flags = newFlags
	e.list = newList
	e.mu.Unlock()

	return nil
}

// Enabled checks if a feature flag is enabled in memory. Default is true for unknown flags.
func (e *Engine) Enabled(key string) bool {
	if e == nil {
		return true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if val, ok := e.flags[key]; ok {
		return val
	}
	return true
}

// Global accessor functions:

// Enabled checks if a feature is enabled on the global engine.
func Enabled(ctx context.Context, key string) bool {
	if globalEngine == nil {
		return true
	}
	return globalEngine.Enabled(key)
}

// SetGlobal replaces the global engine and returns a function that restores
// the previous one. It exists so tests can pin flags without a database.
func SetGlobal(e *Engine) (restore func()) {
	prev := globalEngine
	globalEngine = e
	return func() { globalEngine = prev }
}

// NewStatic builds an engine with fixed flags and no database.
func NewStatic(flags map[string]bool) *Engine {
	e := &Engine{flags: make(map[string]bool, len(flags))}
	for k, v := range flags {
		e.flags[k] = v
	}
	return e
}

// Set stores a flag and applies it to this process immediately. The change and
// its audit entry commit together; other server instances pick the new value
// up on their next reload (at most 60 seconds).
func (e *Engine) Set(ctx context.Context, key string, enabled bool, actorID int64) error {
	if e == nil || e.db == nil {
		return errors.New("features: no database")
	}
	name, desc := map[string]string{"ar": key, "en": key}, map[string]string{"ar": "", "en": ""}
	for _, df := range defaultCoreFlags {
		if df.Key == key {
			name, desc = df.Name, df.Description
			break
		}
	}
	nameJSON, _ := json.Marshal(name)
	descJSON, _ := json.Marshal(desc)
	var actor *int64
	if actorID > 0 {
		actor = &actorID
	}
	err := e.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var previous *bool
		var before bool
		switch err := tx.QueryRow(txCtx, `SELECT is_enabled FROM platform_admin.feature_flags WHERE key = $1`, key).Scan(&before); {
		case err == nil:
			previous = &before
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if _, err := tx.Exec(txCtx, `
			INSERT INTO platform_admin.feature_flags (key, name, description, is_enabled, updated_by, updated_at)
			VALUES ($1, $2::jsonb, $3::jsonb, $4, $5, now())
			ON CONFLICT (key) DO UPDATE
			SET is_enabled = EXCLUDED.is_enabled, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			key, nameJSON, descJSON, enabled, actor); err != nil {
			return err
		}
		entry := database.AuditEntry{
			ActorUserID: actorID,
			Action:      "platform.feature_flag.set",
			EntityType:  "feature_flag",
			EntityID:    key,
			After:       map[string]any{"is_enabled": enabled},
		}
		if previous != nil {
			entry.Before = map[string]any{"is_enabled": *previous}
		}
		return database.WriteAudit(txCtx, tx, entry)
	})
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.flags[key] = enabled
	for i := range e.list {
		if e.list[i].Key == key {
			e.list[i].IsEnabled = enabled
		}
	}
	e.mu.Unlock()
	return nil
}

// Set stores a flag on the global engine; see (*Engine).Set.
func Set(ctx context.Context, key string, enabled bool, actorID int64) error {
	return globalEngine.Set(ctx, key, enabled, actorID)
}
