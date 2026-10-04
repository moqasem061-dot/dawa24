package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestWarehouseRunRejectsOtherActorAndForeignFile(t *testing.T) {
	h, _, _ := setupResumableTestHandler(t)
	owner := authctx.Actor{UserID: 41, OrganizationID: 10}
	ctx := authctx.WithActor(context.Background(), owner)
	run, err := h.CreateTempWarehouseRun(ctx, owner, []int64{201}, nil, "/admin/temporary-warehouses")
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []authctx.Actor{{}, {UserID: 42, OrganizationID: 10}, {UserID: 41, OrganizationID: 11}} {
		if _, err := h.ResolveTempWarehouseRun(authctx.WithActor(context.Background(), actor), run.PublicID); err == nil {
			t.Fatalf("actor %+v accessed another session", actor)
		}
	}
	form := url.Values{"file_id": {"999"}, "col_name": {"1"}, "col_price": {"2"}}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/admin/temporary-warehouses/runs/1/mapping", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rc := chi.NewRouteContext()
	rc.URLParams.Add("runID", "1")
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	h.AdminTempWarehouseRunMappingSubmit(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("foreign file status=%d", w.Code)
	}
	current, _ := h.ResolveTempWarehouseRun(ctx, "1")
	payload, _ := ui.DecodeTempWarehousePayload(current.Payload)
	if len(payload.Mappings) != 0 {
		t.Fatal("foreign file mutated session")
	}
}
