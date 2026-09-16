package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

var pharmacyScope = []rbac.Scope{rbac.ScopePharmacy}

// buyingKeys is a buying capability as both dashboards grant it: a pharmacy
// holds the pharmacy key and a supplier that buys holds the vendor.buying key,
// so a tool offered to both scopes names the pair.
func buyingKeys(caps ...rbac.Capability) []string {
	out := make([]string, 0, 2*len(caps))
	for _, c := range caps {
		out = append(out, c.Pharmacy, c.Vendor)
	}
	return out
}

// buyingStage3Tools are the buying-side reads a pharmacy and a supplier that
// buys share, each gated by the capability of the screen it mirrors.
//
// What can be bought, where and at what price is not among them: find_offers
// and list_promotions answer that with the cart's own availability and offer
// rules. The projections they replaced listed every active listing and called
// it available.
func buyingStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionReorderSuggestions, key: "reorder_suggestions",
			description: "Items this company buys regularly that may need restocking, oldest last purchase first.",
			scopes:      buyingScopes, permissions: buyingKeys(rbac.BuyOrderView), handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("Product name or code.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSmartOrderDetails, key: "smart_order_run_details",
			description: "One smart-order run in detail: each line's outcome and reason, and the estimated total. run is a ref from the smart_order_runs dataset.",
			scopes:      buyingScopes, permissions: buyingKeys(rbac.BuySmartOrderView), handleKind: handles.KindSmartRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSupplierProfile, key: "supplier_profile",
			description: "An approved supplier's profile: rating, contact details and number of branches.",
			scopes:      buyingScopes, permissions: buyingKeys(rbac.BuySupplierView), handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("Supplier name or number.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFavourites, key: "favourites_list",
			description: "The user's saved favourite products.",
			scopes:      buyingScopes, permissions: buyingKeys(rbac.BuyFavoriteView), handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionNotifications, key: "notifications_list",
			description: "The user's own recent notifications and whether they are read.",
			scopes:      buyingScopes, permissions: []string{"pharmacy.dashboard.view", "vendor.dashboard.view"},
		}, projectionListSchema(map[string]any{"status": enumProp("Notification state.", "read", "unread")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFinancialObligations, key: "financial_obligations_summary",
			description: "What this company owes as a buyer: open and overdue purchase invoices, and orders still in progress. For the balance to compare them with, call wallet_summary.",
			scopes:      buyingScopes, permissions: []string{"pharmacy.wallet.view", "vendor.wallet.view"},
		}, projectionListSchema(nil)),
	}
}

// pharmacyStage3Tools read screens only a pharmacy has.
func pharmacyStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSavingProducts, key: "saving_products_list",
			description: "Saving products and cheaper equivalents the pharmacy has recorded, with prices and quantities.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.saving_product.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDecisionMemory, key: "decision_memory_search",
			description: "Saved matching decisions: which catalogue product a drug name was matched to.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.decision_memory.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Drug name or matching key.")})),
	}
}
