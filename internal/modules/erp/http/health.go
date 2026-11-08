package http

import (
	"net/http"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_INVALID", 401, nil))
		return
	}
	info, err := h.svc.HealthData(r.Context(), principal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	orgType := principal.OrgType
	switch orgType {
	case "supplier", "company", "agency":
		orgType = "vendor"
	case "pharmacy", "chain_pharmacy", "individual":
		orgType = "customer"
	}
	data := map[string]any{
		"apiVersion": "1.0", "serverTime": time.Now().UTC().Format(time.RFC3339),
		"organization": map[string]any{"id": principal.OrganizationID, "name": info.OrganizationName, "type": orgType},
		"integration":  map[string]any{"provider": principal.Provider, "status": info.IntegrationStatus},
		"key":          map[string]any{"prefix": principal.KeyPrefix, "scopes": principal.Scopes, "warehouse": healthWarehouse(principal.WarehouseID, info.WarehouseName), "branch": map[string]any{"id": principal.BranchID}, "rateLimitPerMinute": principal.RatePerMinute, "expiresAt": info.ExpiresAt},
	}
	writeOK(w, r, http.StatusOK, data)
}

// healthWarehouse is the key's warehouse, or null for a key bound to none (a
// pharmacy key, which never uploads stock).
func healthWarehouse(id int64, name string) any {
	if id == 0 {
		return nil
	}
	return map[string]any{"id": id, "name": name}
}
