package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The supplier catalogue and its importers.
//
// Split from vendor_routes.go for the 400-line rule (AGENTS.md rule 6), along
// the seam the permissions already draw: what a warehouse keeper touches, as
// against what a sales representative or an accountant does.

func (h *UIHandler) registerVendorCatalogRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.product.view"))
		g.Get("/vendor/products", h.VendorProductsPage)
		g.Get("/vendor/products/variants", h.VendorProductsPage)
		g.Get("/vendor/variants", h.VendorProductsPage)
		g.Get("/vendor/products/new", h.VendorVariantNewPage)
		g.Get("/vendor/variants/new", h.VendorVariantNewPage)
		g.Get("/vendor/catalog/select", h.VendorCatalogSelectPage)
		g.Get("/vendor/catalog/search-json", h.VendorCatalogSearchJSON)
		// The supplier's own stock, searched one query at a time. The
		// advertisement wizard used to inline the whole inventory into its
		// x-data attribute instead.
		g.Get("/vendor/inventory/search-json", h.VendorStockSearchJSON)
		g.Get("/vendor/catalog/product-json/{id}", h.VendorProductDetailJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.product.create"))
		g.Post("/vendor/products/add-from-catalog", h.VendorProductAddFromCatalogSubmit)
		g.Post("/vendor/variants/new", h.VendorVariantNewSubmit)
		g.Post("/vendor/catalog/select", h.VendorCatalogSelectSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.product.update"))
		g.Post("/vendor/variants/{id}/update", h.VendorVariantUpdateSubmit)
		g.Post("/vendor/variants/{id}/toggle-status", h.VendorVariantToggleStatusSubmit)
		g.Post("/vendor/products/activate-all", h.VendorProductsActivateAllSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.product.delete"))
		g.Post("/vendor/products/delete-all", h.VendorProductsDeleteAllSubmit)
		g.Post("/vendor/variants/{id}/delete", h.VendorVariantDeleteSubmit)
	})

	// حصص الفروع: the per-branch purchase quota. Viewing is separate from
	// managing, so a warehouse keeper can see which branch has taken what
	// without being able to hand out more.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.quota.view"))
		g.Get("/vendor/quotas", h.VendorQuotasPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.quota.manage"))
		g.Post("/vendor/quotas/limit", h.VendorQuotaLimitSubmit)
		g.Post("/vendor/quotas/release", h.VendorQuotaReleaseSubmit)
		g.Post("/vendor/quotas/release/undo", h.VendorQuotaReleaseUndoSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.inventory.view"))
		g.Get("/vendor/inventory", h.VendorInventoryPage)
		g.Get("/vendor/inventory/alerts", h.VendorStockAlertsPage)
		g.Get("/vendor/warehouses/alerts", h.VendorStockAlertsPage)
		g.Get("/vendor/transfers", h.VendorTransfersPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.inventory.adjust"))
		g.Post("/vendor/inventory/{id}/adjust", h.VendorStockAdjustSubmit)
		g.Post("/vendor/warehouses/{id}/stocks/{stockID}/adjust", h.VendorWarehouseStockAdjustSubmit)
		g.Post("/vendor/inventory/transfer", h.VendorStockTransferSubmit)
		g.Post("/vendor/transfers/{id}/receive", h.VendorTransferReceiveSubmit)
		g.Post("/vendor/transfers/{id}/cancel", h.VendorTransferCancelSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.warehouse.view"))
		g.Get("/vendor/warehouses", h.VendorWarehousesPage)
		g.Get("/vendor/warehouses/{id}", h.VendorWarehouseDetailPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.warehouse.manage"))
		g.Post("/vendor/warehouses", h.VendorWarehouseCreateSubmit)
		g.Post("/vendor/warehouses/{id}", h.VendorWarehouseUpdateSubmit)
		g.Post("/vendor/warehouses/{id}/toggle", h.VendorWarehouseToggleSubmit)
		g.Post("/vendor/warehouses/{id}/clear-stocks", h.VendorWarehouseClearStocksSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.decision_memory.view"))
		g.Get("/vendor/decision-memory", h.VendorDecisionMemoryPage)
		g.Post("/vendor/decision-memory/add", h.VendorDecisionMemoryAddSubmit)
		g.Post("/vendor/decision-memory/toggle-platform", h.VendorDecisionMemoryTogglePlatformSubmit)
		g.Post("/vendor/decision-memory/{id}/relink", h.VendorDecisionMemoryRelinkSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.decision_memory.delete"))
		g.Post("/vendor/decision-memory/{id}/delete", h.VendorDecisionMemoryDeleteSubmit)
		g.Post("/vendor/decision-memory/clear", h.VendorDecisionMemoryClearSubmit)
	})
}

func (h *UIHandler) registerVendorIngestRoutes(r chi.Router) {
	// The catalogue import. The literal paths are registered before the
	// wildcard so "sample.csv" is a template download and not an import id.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ingest.view"))
		g.Get("/vendor/ingest", h.VendorIngestPage)
		g.Get("/vendor/ingest/sample.csv", h.VendorIngestSampleCSV)
		g.Get("/vendor/ingest/sample.xlsx", h.VendorIngestSampleXLSX)
		g.Get("/vendor/ingest/inventory.csv", h.VendorIngestExport)
		g.Get("/vendor/ingest/{id}", h.VendorIngestSessionPage)
		g.Get("/vendor/ingest/{id}/progress", h.VendorIngestProgress)
		g.Get("/vendor/ingest/{id}/stream", h.VendorIngestProgressStream)
		g.Get("/vendor/ingest/{id}/export", h.VendorIngestRowsExport)
		g.Get("/vendor/ingest/{id}/catalog-search", h.VendorIngestCatalogSearchJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ingest.run"))
		g.Post("/vendor/ingest/upload", h.VendorIngestUploadSubmit)
		g.Post("/vendor/ingest/{id}/mapping", h.VendorIngestMappingSubmit)
		g.Post("/vendor/ingest/{id}/settings", h.VendorIngestSettingsSubmit)
		g.Post("/vendor/ingest/{id}/back", h.VendorIngestBackSubmit)
		g.Post("/vendor/ingest/{id}/back-settings", h.VendorIngestBackToSettingsSubmit)
		g.Post("/vendor/ingest/{id}/rows/{rowID}/update", h.VendorIngestRowUpdateSubmit)
		g.Post("/vendor/ingest/{id}/rows/{rowID}/match", h.VendorIngestRowMatchSubmit)
		g.Post("/vendor/ingest/{id}/rows/{rowID}/toggle", h.VendorIngestRowToggleSubmit)
		g.Post("/vendor/ingest/{id}/batch-quantity", h.VendorIngestBatchQuantitySubmit)
		g.Post("/vendor/ingest/{id}/rows/bulk", h.VendorIngestBulkSubmit)
		g.Post("/vendor/ingest/{id}/confirm", h.VendorIngestConfirmSubmit)
		g.Post("/vendor/ingest/{id}/commit", h.VendorIngestCommitSubmit)
		g.Post("/vendor/ingest/{id}/cancel", h.VendorIngestCancelSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.saving_product.view"))
		g.Get("/vendor/saving-products", h.VendorSavingProductsPage)
		g.Get("/vendor/saving-products/import", h.VendorSavingProductsImportPage)
		g.Get("/vendor/saving-products/sample.xlsx", h.VendorSavingProductsSampleXLSX)
		g.Get("/vendor/saving-products/sample.csv", h.VendorSavingProductsSampleCSV)
		g.Get("/vendor/saving-products/import/{id}", h.VendorSavingProductsImportSessionPage)
		g.Get("/vendor/saving-products/import/session/{id}/progress", h.VendorSavingProductsImportProgressJSON)
		g.Get("/vendor/saving-products/export", h.VendorSavingProductsExport)
		g.Get("/vendor/saving-products/providers/{id}", h.VendorSavingProductProvidersJSON)
		g.Get("/vendor/saving-products/search-products", h.VendorSavingProductSearchJSON)
		g.Get("/vendor/saveing-products", h.VendorSavingProductsAlias)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.saving_product.manage"))
		g.Post("/vendor/saving-products", h.VendorSavingProductCreateSubmit)
		g.Post("/vendor/saving-products/{id}/update", h.VendorSavingProductUpdateSubmit)
		g.Post("/vendor/saving-products/{id}/delete", h.VendorSavingProductDeleteSubmit)
		g.Post("/vendor/saving-products/bulk-delete", h.VendorSavingProductsBulkDeleteSubmit)
		g.Post("/vendor/saving-products/delete-all", h.VendorSavingProductsDeleteAllSubmit)
		g.Post("/vendor/saving-products/import/upload", h.VendorSavingProductsImportUploadSubmit)
		g.Post("/vendor/saving-products/import/{id}/map", h.VendorSavingProductsImportMapSubmit)
		g.Post("/vendor/saving-products/import/{id}/items/{itemIndex}/update", h.VendorSavingProductsImportItemUpdateSubmit)
		g.Post("/vendor/saving-products/import/{id}/items/{itemIndex}/match", h.VendorSavingProductsImportItemMatchSubmit)
		g.Post("/vendor/saving-products/import/{id}/items/{itemIndex}/toggle", h.VendorSavingProductsImportItemToggleSubmit)
		g.Post("/vendor/saving-products/import/{id}/commit", h.VendorSavingProductsImportCommitSubmit)
		g.Post("/vendor/saving-products/import/{id}/cancel", h.VendorSavingProductsImportCancelSubmit)
		g.Post("/vendor/saving-products/import", h.VendorSavingProductsImportSubmit)
		g.Post("/vendor/saving-products/import/start", h.VendorSavingProductsImportStartJSON)
		g.Post("/vendor/saving-products/import/session/{id}/commit", h.VendorSavingProductsImportCommitJSON)
		g.Post("/vendor/saving-products/import/session/{id}/cancel", h.VendorSavingProductsImportCancelJSON)
		g.Post("/vendor/saving-products/preview-columns", h.VendorSavingProductsPreviewColumnsJSON)
	})
}
