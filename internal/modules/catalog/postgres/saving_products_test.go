package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestSavingProductsIntegration(t *testing.T) {
	db := getTestDB(t)
	repo := catalogPostgres.NewRepository(db)
	ctx := context.Background()

	orgID := int64(51)
	userID := int64(41)

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
