package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// bilingualJSON builds the {"ar":...,"en":...} text the JSONB name columns use.
// Building it with json.Marshal keeps quotes and Arabic text valid JSON.
func bilingualJSON(ar, en string) string {
	b, _ := json.Marshal(map[string]string{"ar": ar, "en": en}) // a string map cannot fail to marshal
	return string(b)
}

// runSeed inserts baseline reference data: countries, Egyptian governorates/cities,
// currencies, languages, roles, and system settings.
//
// It runs on every migrate, so it must change nothing that already exists: every
// insert is conditional, and nothing an administrator has set (an inactive
// country, a chosen default language or currency, a rate) is overwritten.
func runSeed(ctx context.Context, db *database.DB, log *slog.Logger) error {
	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		log.InfoContext(txCtx, "seeding reference data")

		// 1. Roles & Permissions
		roles := []struct {
			key         string
			description string
			scope       string
			isSystem    bool
		}{
			{"super_admin", "Global platform super administrator", "platform", true},
			{"admin", "Platform administrator", "platform", true},
			{"supplier_admin", "Vendor organization owner", "organization", false},
			{"pharmacy_admin", "Pharmacy organization owner", "organization", false},
			{"staff", "Organization staff member", "organization", false},
			{"customer", "Standard customer buyer", "organization", false},
		}

		for _, r := range roles {
			// $1 is cast in both places: jsonb_build_object takes "any", so
			// Postgres cannot infer an untyped parameter from it (SQLSTATE 42P08).
			_, err := tx.Exec(txCtx, `
				INSERT INTO identity.roles (key, name, scope, is_system, description)
				VALUES ($1, jsonb_build_object('ar', $1::text, 'en', $1::text), $2, $3, $4)
				ON CONFLICT (key) DO NOTHING;
			`, r.key, r.scope, r.isSystem, r.description)
			if err != nil {
				return err
			}
		}

		// 2. Currencies
		// Only the Egyptian pound is active. The other two are placeholders an
		// administrator must price and switch on: the rates here are not real.
		currencies := []struct {
			code   string
			name   string
			symbol string
			rate   string
			isDef  bool
			active bool
		}{
			{"EGP", bilingualJSON(i18n.TDefault("w4_mod.s_302_302"), "Egyptian Pound"), i18n.TDefault("w4_ui.s_50_50"), "1.0000", true, true},
			{"USD", bilingualJSON(i18n.TDefault("w4_mod.s_457_457"), "US Dollar"), "$", "48.5000", false, false},
			{"SAR", bilingualJSON(i18n.TDefault("w4_mod.s_458_458"), "Saudi Riyal"), i18n.TDefault("w4_mod.s_303_303"), "12.9000", false, false},
		}
		for _, c := range currencies {
			_, err := tx.Exec(txCtx, `
				INSERT INTO platform_admin.currencies (code, name, symbol, exchange_rate_egp, is_active, is_default)
				VALUES ($1, $2::jsonb, $3, $4::numeric, $6,
					$5 AND NOT EXISTS (SELECT 1 FROM platform_admin.currencies WHERE is_default))
				ON CONFLICT (code) DO NOTHING;
			`, c.code, c.name, c.symbol, c.rate, c.isDef, c.active)
			if err != nil {
				return err
			}
		}

		// 3. Languages
		languages := []struct {
			code  string
			name  string
			dir   string
			isDef bool
		}{
			{"ar", i18n.TDefault("w4_mod.s_459_459"), "rtl", true},
			{"en", "English", "ltr", false},
		}
		for _, l := range languages {
			_, err := tx.Exec(txCtx, `
				INSERT INTO platform_admin.languages (code, name, dir, is_active, is_default)
				VALUES ($1, $2, $3, true,
					$4 AND NOT EXISTS (SELECT 1 FROM platform_admin.languages WHERE is_default))
				ON CONFLICT (code) DO NOTHING;
			`, l.code, l.name, l.dir, l.isDef)
			if err != nil {
				return err
			}
		}

		// 4. Country & Egyptian Cities
		var countryID int64
		err := tx.QueryRow(txCtx, `
			INSERT INTO platform_admin.countries (code, name, phone_code, currency, is_active)
			VALUES ('EG', $1::jsonb, '+20', 'EGP', true)
			ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
			RETURNING id;
		`, bilingualJSON(i18n.TDefault("w4_ui.s_176_176"), "Egypt")).Scan(&countryID)
		if err != nil {
			return err
		}

		cities := []struct {
			ar string
			en string
		}{
			{i18n.TDefault("w4_mod.s_460_460"), "Cairo"},
			{i18n.TDefault("w4_mod.s_461_461"), "Giza"},
			{i18n.TDefault("w4_mod.s_462_462"), "Alexandria"},
			{i18n.TDefault("w4_mod.s_463_463"), "Mansoura"},
			{i18n.TDefault("w4_mod.s_464_464"), "Tanta"},
			{i18n.TDefault("w4_mod.s_465_465"), "Asyut"},
			{i18n.TDefault("w4_mod.s_466_466"), "Sohag"},
			{i18n.TDefault("w4_mod.s_467_467"), "Port Said"},
			{i18n.TDefault("w4_mod.s_468_468"), "Suez"},
			{i18n.TDefault("w4_mod.s_469_469"), "Zagazig"},
		}

		// The cities table has no unique key, so ON CONFLICT cannot guard it. The
		// official list comes with the migrations; these are only a fallback for
		// a country that has none, and are skipped once it has any city.
		var existing int
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM platform_admin.cities WHERE country_id = $1`, countryID).Scan(&existing); err != nil {
			return err
		}
		if existing == 0 {
			for _, city := range cities {
				_, err := tx.Exec(txCtx, `
					INSERT INTO platform_admin.cities (country_id, name, is_active)
					VALUES ($1, jsonb_build_object('ar', $2::text, 'en', $3::text), true);
				`, countryID, city.ar, city.en)
				if err != nil {
					return err
				}
			}
		}

		// 5. Default Public Settings
		settings := []struct {
			key   string
			value string
			desc  string
		}{
			{"platform_name", bilingualJSON(i18n.TDefault("w4_cmd.24_262"), "Dawa 24"), "Platform public title"},
			{"maintenance_mode", `{"enabled":false}`, "Platform maintenance state"},
		}
		for _, s := range settings {
			_, err := tx.Exec(txCtx, `
				INSERT INTO platform_admin.system_settings (key, value, description, is_public, updated_at)
				VALUES ($1, $2::jsonb, $3, true, now())
				ON CONFLICT (key) DO NOTHING;
			`, s.key, s.value, s.desc)
			if err != nil {
				return err
			}
		}

		log.InfoContext(txCtx, "reference data seeding completed successfully")
		return nil
	})
}
