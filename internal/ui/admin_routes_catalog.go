package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The catalogue and inventory surface.
//
// The gates are one per section rather than one per module. "catalog.product.view"
// used to open the drug catalogue, the vendor items list, the promoted
// products screen, both importers, the warehouses, the temporary warehouses,
// the saving products and the AI match-decision memory — nine screens over
// eight different tables. There was no way to give a catalogue editor the
// catalogue without also giving them every vendor's stock.
func (h *UIHandler) registerAdminCatalogRoutes(r chi.Router) {
	h.registerAdminProductRoutes(r)
	h.registerAdminImportRoutes(r)
	h.registerAdminWarehouseRoutes(r)
	h.registerAdminCatalogMutations(r)
}

func (h *UIHandler) registerAdminProductRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.view"))
		g.Get("/admin/products", h.AdminProductsPage)
		g.Get("/admin/products/{id}", h.AdminProductDetailPage)
		g.Get("/admin/products/sample.csv", h.AdminProductsSampleCSV)
		g.Get("/admin/products/sample.xlsx", h.AdminProductsSampleXLSX)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.vendor_product.view"))
		g.Get("/admin/product-child", h.AdminProductChildrenPage)
		g.Get("/admin/product-child/{id}", h.AdminProductDetailPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.adv_product.view"))
		g.Get("/admin/adv-products", h.AdminAdvProductsPage)
		g.Get("/admin/adv-products/{id}", h.AdminAdvProductDetailPage)
		g.Post("/admin/adv-products/{id}/approve", h.AdminAdvProductApproveSubmit)
		g.Post("/admin/adv-products/{id}/reject", h.AdminAdvProductRejectSubmit)
		g.Post("/admin/adv-products/new", h.AdminAdvProductCreateSubmit)
		g.Get("/admin/import/temparte-warehouses", h.AdminAdvProductsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.saving_product.view"))
		g.Get("/admin/saving-products", h.AdminSavingProductsPage)
		g.Get("/admin/saveing-products", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/saving-products", http.StatusMovedPermanently)
		})
		g.Get("/admin/saving-products/search-products", h.AdminSavingProductSearchJSON)
		g.Post("/admin/saving-products/{id}/link", h.AdminSavingProductLinkSubmit)
		g.Get("/admin/saving-products/user/{userId}", h.AdminSavingProductsPage)
		g.Get("/admin/saving-products/org/{organizationId}", h.AdminSavingProductsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.view"))
		g.Get("/admin/match-decisions", h.AdminMatchDecisionsPage)
		g.Get("/admin/match-decisions/export", h.AdminMatchDecisionsExportXLSX)
		g.Get("/admin/match-decisions/search-products", h.AdminSavingProductSearchJSON)
		g.Post("/admin/match-decisions/toggle-state", h.AdminMatchDecisionToggleStateSubmit)
		g.Post("/admin/match-decisions/{id}/promote", h.AdminMatchDecisionPromoteSubmit)
		g.Post("/admin/match-decisions/{id}/demote", h.AdminMatchDecisionDemoteSubmit)
		g.Post("/admin/match-decisions/{id}/relink", h.AdminMatchDecisionRelinkSubmit)
		g.Post("/admin/match-decisions/bulk", h.AdminMatchDecisionBulkSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.delete"))
		g.Post("/admin/match-decisions/{id}/delete", h.AdminMatchDecisionDeleteSubmit)
		g.Post("/admin/match-decisions/clear", h.AdminMatchDecisionsClearSubmit)
	})
}

func (h *UIHandler) registerAdminImportRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.import.view"))
		g.Get("/admin/products/import", h.AdminProductsImportPage)
		g.Get("/admin/products/import/{id}", h.AdminProductsImportReviewPage)
		g.Get("/admin/products/import/{id}/mapping", h.AdminProductsImportMappingPage)
		g.Get("/admin/products/import/{id}/progress", h.AdminProductsImportProgress)
		g.Get("/admin/products/import/{id}/stream", h.AdminProductsImportProgressStream)
		g.Get("/admin/import", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/products/import", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.import.run"))
		g.Post("/admin/products/import", h.AdminProductsImportSubmit)
		g.Post("/admin/upload-warehouse-file", h.AdminProductsImportSubmit)
		// The staged review: re-run the file under corrected settings, adjust
		// which rows are included, then commit or discard. Only the commit
		// route writes to the catalogue.
		g.Post("/admin/products/import/{id}/preview", h.AdminProductsImportPreview)
		g.Post("/admin/products/import/{id}/prepare", h.AdminProductsImportPrepare)
		g.Post("/admin/products/import/{id}/rows/{rowID}", h.AdminProductsImportRowToggle)
		g.Post("/admin/products/import/{id}/select", h.AdminProductsImportSelect)
		g.Post("/admin/products/import/{id}/commit", h.AdminProductsImportCommit)
		g.Post("/admin/products/import/{id}/cancel", h.AdminProductsImportCancel)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.image_import.view"))
		g.Get("/admin/products/images/import", h.AdminProductImagesImportPage)
		g.Get("/admin/products/images/import/{id}", h.AdminProductImagesSessionPage)
		g.Get("/admin/products/images/import/{id}/progress", h.AdminProductImagesProgressJSON)
		g.Get("/admin/products/images/import/sample.xlsx", h.AdminProductImagesSampleXLSX)
		g.Get("/admin/products/images/import/sample.csv", h.AdminProductImagesSampleCSV)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.image_import.run"))
		g.Post("/admin/products/images/import/upload", h.AdminProductImagesUploadSubmit)
		g.Post("/admin/products/images/import/{id}/mapping", h.AdminProductImagesMappingSubmit)
		g.Post("/admin/products/images/import/{id}/cancel", h.AdminProductImagesCancelSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.org_import.view"))
		g.Get("/admin/organizations/import", h.AdminOrgImportPage)
		g.Get("/admin/organizations/import/search-products", h.AdminSavingProductSearchJSON)
		g.Get("/admin/organizations/import/{orgID}/saving/search-products", h.AdminSavingProductSearchJSON)
		g.Get("/admin/organizations/import/{orgID}/saving", h.AdminOrgImportSavingUploadPage)
		g.Get("/admin/organizations/import/{orgID}/saving/{id}", h.AdminOrgImportSavingSessionPage)
		g.Get("/admin/organizations/import/{orgID}/saving/{id}/mapping", h.AdminOrgImportSavingMappingPage)
		g.Get("/admin/organizations/import/{orgID}/saving/{id}/review", h.AdminOrgImportSavingReviewPage)
		g.Get("/admin/organizations/import/{orgID}/runs/{runID}/mapping", h.AdminOrgImportSavingMappingPage)
		g.Get("/admin/organizations/import/{orgID}/runs/{runID}/review", h.AdminOrgImportSavingReviewPage)
		g.Get("/admin/organizations/import/{orgID}/compare", h.AdminOrgImportComparePage)
		g.Get("/admin/organizations/import/{orgID}/ingest", h.AdminOrgImportVendorIngestPage)
		g.Get("/admin/organizations/import/{orgID}/ingest/{id}", h.AdminOrgImportVendorIngestSessionPage)
		g.Get("/admin/organizations/import/{orgID}/ingest/{id}/progress", h.AdminOrgImportVendorIngestProgress)
		g.Get("/admin/organizations/import/{orgID}/ingest/{id}/stream", h.AdminOrgImportVendorIngestProgressStream)
		g.Get("/admin/organizations/import/{orgID}/ingest/{id}/catalog-search", h.VendorIngestCatalogSearchJSON)
		g.Get("/admin/organizations/import/{orgID}/ingest/{id}/export", h.VendorIngestRowsExport)
		g.Get("/admin/organizations/import/saving/{id}", h.AdminOrgImportSavingSessionPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.org_import.run"))
		g.Post("/admin/organizations/import/saving/upload", h.AdminOrgImportSavingsUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/upload", h.AdminOrgImportSavingsUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/runs/{runID}/mapping", h.AdminOrgImportSavingMappingSubmit)
		g.Post("/admin/organizations/import/{orgID}/runs/{runID}/commit", h.AdminOrgImportSavingCommitSubmit)
		g.Post("/admin/organizations/import/{orgID}/runs/{runID}/cancel", h.AdminOrgImportSavingCancelSubmit)
		g.Post("/admin/organizations/import/saving/{id}/map", h.AdminOrgImportSavingMappingSubmit)
		g.Post("/admin/organizations/import/saving/{id}/commit", h.AdminOrgImportSavingCommitSubmit)
		g.Post("/admin/organizations/import/saving/{id}/cancel", h.AdminOrgImportSavingCancelSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/map", h.AdminOrgImportSavingMappingSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/mapping", h.AdminOrgImportSavingMappingSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/commit", h.AdminOrgImportSavingCommitSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/cancel", h.AdminOrgImportSavingCancelSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/items/{itemIndex}/update", h.AdminOrgImportSavingItemUpdateSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/items/{itemIndex}/match", h.AdminOrgImportSavingItemMatchSubmit)
		g.Post("/admin/organizations/import/{orgID}/saving/{id}/items/{itemIndex}/toggle", h.AdminOrgImportSavingItemToggleSubmit)
		g.Post("/admin/organizations/import/temp-warehouse/upload", h.AdminOrgImportTempWarehouseUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/temp-warehouse/upload", h.AdminOrgImportTempWarehouseUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/compare/upload", h.AdminOrgImportTempWarehouseUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/upload", h.AdminOrgImportVendorIngestUploadSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/map", h.AdminOrgImportVendorIngestMapSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/mapping", h.AdminOrgImportVendorIngestMapSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/settings", h.AdminOrgImportVendorIngestSettingsSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/back", h.AdminOrgImportVendorIngestBackSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/back-settings", h.AdminOrgImportVendorIngestBackToSettingsSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/rows/{rowID}/update", h.VendorIngestRowUpdateSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/rows/{rowID}/match", h.VendorIngestRowMatchSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/rows/{rowID}/toggle", h.VendorIngestRowToggleSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/batch-quantity", h.VendorIngestBatchQuantitySubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/rows/bulk", h.VendorIngestBulkSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/confirm", h.AdminOrgImportVendorIngestConfirmSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/commit", h.AdminOrgImportVendorIngestCommitSubmit)
		g.Post("/admin/organizations/import/{orgID}/ingest/{id}/cancel", h.AdminOrgImportVendorIngestCancelSubmit)
	})
}

func (h *UIHandler) registerAdminWarehouseRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.view"))
		g.Get("/admin/stocks", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/warehouses", http.StatusMovedPermanently)
		})
		g.Get("/admin/warehouses", h.AdminWarehousesPage)
		g.Get("/admin/warehouses/{id}", h.AdminWarehouseDetailPage)
		g.Get("/admin/warehouses/{id}/stocks-json", h.AdminWarehouseStocksJSON)
		// The platform's warehouse administration. An administrator has no
		// tenant of their own, so these run AsSystem against the owning
		// organisation rather than through the vendor's tenant-scoped path.
		g.Post("/admin/warehouses/new", h.AdminWarehouseCreateSubmit)
		g.Post("/admin/warehouses/{id}/edit", h.AdminWarehouseEditSubmit)
		g.Post("/admin/warehouses/{id}/toggle", h.AdminWarehouseToggleSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.temp_warehouse.view"))
		g.Get("/admin/temporary-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/temporary-warehouses/upload", h.AdminTempWarehouseUploadPage)
		g.Get("/admin/temporary-warehouses/{id}/items-json", h.AdminTempWarehouseItemsJSON)
		g.Get("/admin/temporary-warehouses/{id}/mapping-json", h.AdminTempWarehouseMappingJSON)
		g.Get("/admin/temporary-warehouses/{id}/export", h.AdminTempWarehouseExportXLSX)

		g.Get("/admin/user/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/user/temparte-warehouses/upload", h.AdminTempWarehouseUploadPage)
		// Readiness of a freshly uploaded batch. The upload detaches the parse,
		// so the mapping wizard waits on this instead of opening on a file whose
		// columns nobody has read yet.
		g.Get("/admin/user/temparte-warehouses/staging", h.CompareStagingStatus)
		g.Get("/admin/user/temparte-warehouses/{id}/items-json", h.AdminTempWarehouseItemsJSON)
		g.Get("/admin/user/temparte-warehouses/{id}/mapping-json", h.AdminTempWarehouseMappingJSON)
		g.Get("/admin/user/temparte-warehouses/{id}/export", h.AdminTempWarehouseExportXLSX)
		g.Get("/admin/user/temparte-warehouses/{id}", h.AdminTempWarehousesPage)

		g.Get("/admin/admins/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/admins/temparte-warehouses/{id}/items-json", h.AdminTempWarehouseItemsJSON)
		g.Get("/admin/admins/temparte-warehouses/{id}/mapping-json", h.AdminTempWarehouseMappingJSON)

		g.Get("/admin/plan/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/plan/temparte-warehouses/{id}/items-json", h.AdminTempWarehouseItemsJSON)
		g.Get("/admin/plan/temparte-warehouses/{id}/mapping-json", h.AdminTempWarehouseMappingJSON)
		g.Get("/admin/plan/temparte-warehouses/{id}", h.AdminTempWarehousesPage)
		g.Get("/admin/user-plan/temparte-warehouses", h.AdminTempWarehousesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.update"))
		g.Post("/admin/temporary-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/user/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/admins/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/plan/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)

		g.Post("/admin/temporary-warehouses/{id}/mapping", h.AdminTempWarehouseMappingSubmit)
		g.Post("/admin/user/temparte-warehouses/{id}/mapping", h.AdminTempWarehouseMappingSubmit)
		g.Post("/admin/admins/temparte-warehouses/{id}/mapping", h.AdminTempWarehouseMappingSubmit)
		g.Post("/admin/plan/temparte-warehouses/{id}/mapping", h.AdminTempWarehouseMappingSubmit)

		g.Post("/admin/temporary-warehouses/{id}/toggle-archive", h.AdminTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/user/temparte-warehouses/{id}/toggle-archive", h.AdminTempWarehouseToggleArchiveSubmit)

		g.Post("/admin/temporary-warehouses/bulk", h.AdminTempWarehouseBulkSubmit)
		g.Post("/admin/user/temparte-warehouses/bulk", h.AdminTempWarehouseBulkSubmit)
		registerTempWarehouseRunRoutes(g, "/admin/user/temparte-warehouses", h)
		registerTempWarehouseRunRoutes(g, "/admin/temporary-warehouses", h)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.delete"))
		g.Post("/admin/temporary-warehouses/items/{id}/delete", h.AdminTempWarehouseItemDeleteSubmit)
		g.Post("/admin/temporary-warehouses/{id}/delete", h.AdminTempWarehouseDeleteSubmit)
	})

	// "مستودعاتي المرفوعة" — same screen, scoped to the signed-in user's own
	// uploads. Its own permissions so a moderator can be given just this.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.my_temp_warehouse.view"))
		g.Get("/admin/my/temparte-warehouses", h.AdminMyTempWarehousesPage)
		g.Get("/admin/my/temparte-warehouses/upload", h.AdminTempWarehouseUploadPage)
		g.Get("/admin/my/temparte-warehouses/staging", h.CompareStagingStatus)
		g.Get("/admin/my/temparte-warehouses/{id}/items-json", h.AdminMyTempWarehouseItemsJSON)
		g.Get("/admin/my/temparte-warehouses/{id}/mapping-json", h.AdminMyTempWarehouseMappingJSON)
		g.Get("/admin/my/temparte-warehouses/{id}/export", h.AdminMyTempWarehouseExportXLSX)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.my_temp_warehouse.manage"))
		g.Post("/admin/my/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/mapping", h.AdminMyTempWarehouseMappingSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/toggle-archive", h.AdminMyTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/delete", h.AdminMyTempWarehouseDeleteSubmit)
		g.Post("/admin/my/temparte-warehouses/items/{id}/delete", h.AdminMyTempWarehouseItemDeleteSubmit)
		g.Post("/admin/my/temparte-warehouses/bulk", h.AdminMyTempWarehouseBulkSubmit)
		registerTempWarehouseRunRoutes(g, "/admin/my/temparte-warehouses", h)
	})

	// "مستودعات المشرفين تحت إدارتي" — a main moderator's view of the
	// moderators assigned under them.
	//
	// The permission gate answers "may this person manage a team's
	// warehouses?". It does NOT answer "whose team?" — that is decided per
	// request against identity.users.moderator_parent_id, inside every handler
	// below, because a route gate cannot know which files belong to which
	// hierarchy. See admin_team_temp_warehouse_handlers.go.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.team_temp_warehouse.view"))
		g.Get("/admin/team/temparte-warehouses", h.AdminTeamTempWarehousesPage)
		g.Get("/admin/team/temparte-warehouses/upload", h.AdminTempWarehouseUploadPage)
		g.Get("/admin/team/temparte-warehouses/staging", h.CompareStagingStatus)
		g.Get("/admin/team/temparte-warehouses/{id}/items-json", h.AdminTeamTempWarehouseItemsJSON)
		g.Get("/admin/team/temparte-warehouses/{id}/mapping-json", h.AdminTeamTempWarehouseMappingJSON)
		g.Get("/admin/team/temparte-warehouses/{id}/export", h.AdminTeamTempWarehouseExportXLSX)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.team_temp_warehouse.manage"))
		g.Post("/admin/team/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/team/temparte-warehouses/{id}/mapping", h.AdminTeamTempWarehouseMappingSubmit)
		g.Post("/admin/team/temparte-warehouses/{id}/toggle-archive", h.AdminTeamTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/team/temparte-warehouses/{id}/delete", h.AdminTeamTempWarehouseDeleteSubmit)
		g.Post("/admin/team/temparte-warehouses/items/{id}/delete", h.AdminTeamTempWarehouseItemDeleteSubmit)
		g.Post("/admin/team/temparte-warehouses/bulk", h.AdminTeamTempWarehouseBulkSubmit)
		registerTempWarehouseRunRoutes(g, "/admin/team/temparte-warehouses", h)
	})
}

func registerTempWarehouseRunRoutes(g chi.Router, base string, h *UIHandler) {
	g.Get(base+"/runs/{runID}", h.AdminTempWarehouseRunDispatcher)
	g.Get(base+"/runs/{runID}/mapping", h.AdminTempWarehouseRunMappingPage)
	g.Post(base+"/runs/{runID}/mapping", h.AdminTempWarehouseRunMappingSubmit)
	g.Get(base+"/runs/{runID}/review", h.AdminTempWarehouseRunReviewPage)
	g.Post(base+"/runs/{runID}/commit", h.AdminTempWarehouseRunCommitSubmit)
	g.Get(base+"/runs/{runID}/progress", h.AdminTempWarehouseRunProgressPage)
	g.Post(base+"/runs/{runID}/cancel", h.AdminTempWarehouseRunCancelSubmit)
}

func (h *UIHandler) registerAdminCatalogMutations(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.create"))
		g.Post("/admin/products/new", h.AdminProductCreateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.update"))
		g.Post("/admin/products/{id}/edit", h.AdminProductEditSubmit)
		g.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
		g.Post("/admin/product-child/{id}/status", h.AdminProductChildStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.delete"))
		g.Post("/admin/products/delete-all", h.AdminProductsDeleteAllSubmit)
		g.Post("/admin/products/{id}/delete", h.AdminProductDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.brand.update"))
		g.Post("/admin/brands/new", h.AdminBrandCreateSubmit)
		g.Post("/admin/brands/{id}/edit", h.AdminBrandEditSubmit)
		g.Post("/admin/brands/{id}/status", h.AdminBrandStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.brand.delete"))
		g.Post("/admin/brands/{id}/delete", h.AdminBrandDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.category.update"))
		g.Post("/admin/categories/new", h.AdminCategoryCreateSubmit)
		g.Post("/admin/categories/{id}/edit", h.AdminCategoryEditSubmit)
		g.Post("/admin/categories/{id}/toggle", h.AdminCategoryToggleSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.category.delete"))
		g.Post("/admin/categories/{id}/delete", h.AdminCategoryDeleteSubmit)
	})
}
