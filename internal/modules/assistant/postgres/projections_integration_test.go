package postgres_test

import (
	"context"
	"testing"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestEveryProjectionRuns executes every fixed projection against a migrated
// schema, as the dashboard that is offered it. The SQL is hand-written per
// kind and nothing else runs it before production; a column that was renamed
// or a parameter that was added fails here instead of in a user's answer.
func TestEveryProjectionRuns(t *testing.T) {
	db := openTestDB(t)
	repo := assistantPostgres.NewRepository(db)
	// platform_health reads River's job table, which the worker creates at
	// start; a freshly migrated test database has not run one.
	migrator, err := rivermigrate.New(riverpgxv5.New(db.Pool()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(context.Background(), rivermigrate.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}

	pharmacy := ownerActor(rbac.ScopePharmacy, 1, 1)
	vendor := ownerActor(rbac.ScopeVendor, 1, 1)
	admin := authctx.Actor{UserID: 1, IsStaff: true, Scope: rbac.ScopeAdmin}
	admin.Grants(rbac.Default().KeysFor(rbac.ScopeAdmin))

	buying := []assistant.ProjectionKind{
		assistant.ProjectionReorderSuggestions, assistant.ProjectionSmartOrderDetails,
		assistant.ProjectionSupplierProfile, assistant.ProjectionFavourites,
		assistant.ProjectionNotifications, assistant.ProjectionFinancialObligations,
		assistant.ProjectionAccountProfile,
	}
	cases := map[string]struct {
		actor authctx.Actor
		kinds []assistant.ProjectionKind
	}{
		"pharmacy": {pharmacy, append([]assistant.ProjectionKind{
			assistant.ProjectionSavingProducts, assistant.ProjectionDecisionMemory}, buying...)},
		"vendor buying": {vendor, buying},
		"vendor": {vendor, []assistant.ProjectionKind{
			assistant.ProjectionQuotaReport, assistant.ProjectionImportRuns, assistant.ProjectionImportRunDetails,
			assistant.ProjectionSponsorshipStatus, assistant.ProjectionInventoryHealth, assistant.ProjectionSalesInsights,
			assistant.ProjectionBatchExpiryReport, assistant.ProjectionDispatchSchedule}},
		"admin": {admin, []assistant.ProjectionKind{
			assistant.ProjectionApprovals, assistant.ProjectionDeletionRequests, assistant.ProjectionFinance,
			assistant.ProjectionVisitors, assistant.ProjectionHealth, assistant.ProjectionMatchDecisions,
			assistant.ProjectionInstitutionalGraph, assistant.ProjectionSecurityEvents}},
	}
	for name, tc := range cases {
		for _, kind := range tc.kinds {
			ctx := authctx.WithActor(database.WithTenant(context.Background(), tc.actor.OrgID), tc.actor)
			if _, err := repo.ReadProjection(ctx, tc.actor, assistant.ProjectionQuery{Kind: kind, ID: 1, Search: "x", Limit: 5}); err != nil {
				t.Errorf("%s %s: %v", name, kind, err)
			}
		}
	}
}
