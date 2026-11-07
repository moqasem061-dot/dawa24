package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestSavingProductsIntegration(t *testing.T) {
	db := getTestDB(t)
	repo := catalogPostgres.NewRepository(db)
	ctx := context.Background()

	// Create the organization and user this test writes for. The suite used to
	// assume legacy rows 51 and 41, which a fresh database does not have.
	orgID := int64(51)
	userID := int64(41)
	_, err := db.Pool().Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"ar":"مؤسسة","en":"Org"}') ON CONFLICT DO NOTHING`, orgID)
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, `INSERT INTO identity.users (id, email, password_hash, name) VALUES ($1, 'saving-test-41@example.com', 'x', '{"ar":"","en":""}') ON CONFLICT DO NOTHING`, userID)
	require.NoError(t, err)

	// 1. Batch Upsert
	items := []*catalog.SavingProduct{
		{
			OrganizationID: orgID,
			UserID:         &userID,
			NameProduct:    "بنادول اكسترا تيست",
			SKU:            "TEST-SKU-999",
			Quantity:       50,
			Price:          money.FromMajor(35),
		},
	}

	added, updated, err := repo.BatchUpsertSavingProducts(ctx, orgID, &userID, items)
	require.NoError(t, err)
	assert.True(t, added > 0 || updated > 0)

	// 2. List Enriched
	list, stats, err := repo.ListSavingProductsEnriched(ctx, orgID, "بنادول", "all", 10, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, list)
	assert.NotNil(t, stats)
	assert.True(t, stats.CountAll > 0)

	// 3. Update Saving Product
	sp := list[0]
	sp.Quantity = 75
	err = repo.UpdateSavingProduct(ctx, &sp.SavingProduct)
	require.NoError(t, err)

	// 4. Admin List Across Platform
	adminList, adminStats, err := repo.ListAllSavingProductsAdmin(ctx, nil, nil, "", "all", 50, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, adminList)
	assert.NotNil(t, adminStats)
	assert.True(t, adminStats.TotalProducts > 0)
}
