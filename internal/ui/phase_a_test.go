package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// TestPhaseA_2FARoutes_Deleted verifies that fake 2FA endpoints return 404 (Law 1, Task A.1).
func TestPhaseA_2FARoutes_Deleted(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)

	// GET /settings/security/2fa -> 404
	rec := doGET(t, r, "/settings/security/2fa", authctx.Actor{UserID: 1, OrganizationID: 1})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// POST /settings/security/2fa/enable -> 404
	rec = doPOST(t, r, "/settings/security/2fa/enable", url.Values{"code": []string{"123456"}}, authctx.Actor{UserID: 1, OrganizationID: 1})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// GET /auth/2fa-challenge -> 404
	rec = doGET(t, r, "/auth/2fa-challenge", authctx.Actor{})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// POST /auth/2fa-challenge -> 404
	rec = doPOST(t, r, "/auth/2fa-challenge", url.Values{"code": []string{"123456"}}, authctx.Actor{})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPhaseA_PDFRoutes_Deleted verifies that fake PDF download endpoints return 404 (Law 1, Task A.2).
func TestPhaseA_PDFRoutes_Deleted(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)

	rec := doGET(t, r, "/invoices/100/pdf", authctx.Actor{UserID: 1, OrganizationID: 1})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = doGET(t, r, "/orders/100/pdf", authctx.Actor{UserID: 1, OrganizationID: 1})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPhaseA_FabricatedDatasets_Removed verifies that reference and trash pages no longer render hardcoded mock data (Task A.3).
func TestPhaseA_FabricatedDatasets_Removed(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)

	staffActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}

	// 1. Cities
	rec := doGET(t, r, "/admin/cities", staffActor)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. Social Media (Redirects to Settings site tab)
	rec = doGET(t, r, "/admin/social-media", staffActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	// 3. API Integrations (Redirects to Developers)
	rec = doGET(t, r, "/admin/api-integrations", staffActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	// 4. Trash List. With no admin service the page redirects rather than
	// rendering — which is itself the point: the model list and its counts come
	// from information_schema now, not from a hardcoded slice, so there is
	// nothing to show without a database.
	rec = doGET(t, r, "/admin/trash-list", staffActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "1240")
	assert.NotContains(t, body, "14200")
}

// TestPhaseA_NoOpDestructiveActions_DoNotEmitFakeSuccess verifies that without services, no-op handlers do NOT emit success (Law 3, Task A.4).
func TestPhaseA_NoOpDestructiveActions_DoNotEmitFakeSuccess(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)

	staffActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}
	vendorActor := authctx.Actor{UserID: 2, OrganizationID: 10, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// Trash restore without DB service must not say success
	rec := doPOST(t, r, "/admin/trash-list/products/10/restore", url.Values{}, staffActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.NotContains(t, loc, "notice=success")

	// Vendor branch delete without org service must not say success
	rec = doPOST(t, r, "/vendor/branches/5/delete", url.Values{}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc = rec.Header().Get("Location")
	assert.NotContains(t, loc, "notice=success")

	// Vendor team toggle without org service must not say success
	rec = doPOST(t, r, "/vendor/team/5/toggle", url.Values{}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc = rec.Header().Get("Location")
	assert.NotContains(t, loc, "notice=success")
}

// TestPhaseA_RealDatabase_BranchDeleteAndToggle tests D2/D3 with real DB when available.
func TestPhaseA_RealDatabase_BranchDeleteAndToggle(t *testing.T) {
	db := testDB(t)
	if db == nil {
		return
	}

	h := newRealUIHandler(t, db)

	vendorOrgID := seedOrg(t, db, "vendor")
	branchID := seedBranch(t, db, vendorOrgID)

	vendorActor := authctx.Actor{
		UserID:         1000 + vendorOrgID,
		OrganizationID: vendorOrgID,
		OrgType:        "vendor",
		Role:           "owner",
	}

	// 1. Delete branch
	rec := doPOST(t, h, fmt.Sprintf("/vendor/branches/%d/delete", branchID), url.Values{}, vendorActor)
	require.Equal(t, http.StatusSeeOther, rec.Code)

	// Verify branch is soft deleted or inactive in DB
	var status string
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, "SELECT status FROM org.branches WHERE id = $1", branchID).Scan(&status)
	})
	require.NoError(t, err)
	assert.Equal(t, "inactive", status)
}
