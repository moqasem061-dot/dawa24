package assistant

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// ProjectionKind identifies one fixed, read-only assistant projection. The
// name is selected by server code; it is never accepted as SQL or a table name
// from the model.
type ProjectionKind string

const (
	ProjectionReorderSuggestions ProjectionKind = "reorder_suggestions"
	ProjectionSavingProducts     ProjectionKind = "saving_products_list"
	ProjectionSmartOrderDetails  ProjectionKind = "smart_order_run_details"
	ProjectionDecisionMemory     ProjectionKind = "decision_memory_search"
	ProjectionSupplierProfile    ProjectionKind = "supplier_profile"
	ProjectionFavourites         ProjectionKind = "favourites_list"
	ProjectionNotifications      ProjectionKind = "notifications_list"
	ProjectionQuotaReport        ProjectionKind = "quota_report"
	ProjectionImportRuns         ProjectionKind = "import_runs_list"
	ProjectionImportRunDetails   ProjectionKind = "import_run_details"
	ProjectionSponsorshipStatus  ProjectionKind = "sponsorship_status"
	ProjectionApprovals          ProjectionKind = "approvals_pending"
	ProjectionDeletionRequests   ProjectionKind = "deletion_requests"
	ProjectionFinance            ProjectionKind = "finance_overview"
	ProjectionVisitors           ProjectionKind = "visitors_report"
	ProjectionHealth             ProjectionKind = "platform_health"
	ProjectionMatchDecisions     ProjectionKind = "match_decisions_search"
	ProjectionInstitutionalGraph ProjectionKind = "institutional_graph"
	ProjectionAccountProfile     ProjectionKind = "account_profile"
	ProjectionInventoryHealth    ProjectionKind = "inventory_health"
	ProjectionSalesInsights      ProjectionKind = "sales_insights"
	// Enhanced Pharmacy Projections
	ProjectionFinancialObligations ProjectionKind = "financial_obligations_summary"
	// Enhanced Vendor Projections
	ProjectionBatchExpiryReport ProjectionKind = "batch_expiry_report"
	ProjectionDispatchSchedule  ProjectionKind = "dispatch_schedule"
	// Enhanced Admin Projections
	ProjectionSecurityEvents ProjectionKind = "platform_security_overview"
)

// ProjectionQuery is the bounded argument set shared by Stage-3 reads.
// Detail IDs are resolved from actor-bound handles before this reaches a
// repository.
type ProjectionQuery struct {
	Kind      ProjectionKind
	Range     DateRange
	Search    string
	Status    string
	Limit     int
	Offset    int
	ID        int64
	BranchID  int64
	ProductID int64
}

// ProjectionRow is a JSON-safe row returned by a fixed projection query.
// Database IDs stay server-side; Handle is the only identifier a model sees.
type ProjectionRow struct {
	ID     int64
	Handle string
	Values map[string]any
}

func (r ProjectionRow) MarshalJSON() ([]byte, error) {
	values := make(map[string]any, len(r.Values)+1)
	for key, value := range r.Values {
		values[key] = value
	}
	if r.Handle != "" {
		values["handle"] = r.Handle
	}
	return json.Marshal(values)
}

// ProjectionReader is optional so existing in-memory readers and security
// tests continue to exercise the original tool set without fake SQL data.
type ProjectionReader interface {
	ReadProjection(context.Context, authctx.Actor, ProjectionQuery) (Page[ProjectionRow], error)
}
