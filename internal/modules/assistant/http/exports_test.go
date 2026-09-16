package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type exportsByToken map[string]*assistant.Export

func (e exportsByToken) LoadExport(_ context.Context, token string) (*assistant.Export, error) {
	return e[token], nil
}

// The download token is a capability, not the authority: only the user who
// produced the file, in the organisation it was produced in, may fetch it.
func TestExportDownloadIsOwnerOnly(t *testing.T) {
	store := exportsByToken{"tok": {UserID: 7, OrganizationID: 3, ExportFile: assistant.ExportFile{
		Filename: "طلباتي.csv", MIMEType: "text/csv", Content: []byte("a,b\n"),
	}}}
	h := &Handler{log: slog.Default()}
	h.SetExports(store)

	cases := []struct {
		name   string
		token  string
		actor  authctx.Actor
		status int
	}{
		{"owner", "tok", authctx.Actor{UserID: 7, OrgID: 3}, http.StatusOK},
		{"colleague in the same organisation", "tok", authctx.Actor{UserID: 8, OrgID: 3}, http.StatusNotFound},
		{"same user, another organisation", "tok", authctx.Actor{UserID: 7, OrgID: 4}, http.StatusNotFound},
		{"no session", "tok", authctx.Actor{}, http.StatusNotFound},
		{"unknown token", "nope", authctx.Actor{UserID: 7, OrgID: 3}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/assistant/exports/"+tc.token, nil)
			rc := chi.NewRouteContext()
			rc.URLParams.Add("token", tc.token)
			ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rc)
			if tc.actor.UserID != 0 {
				ctx = authctx.WithActor(ctx, tc.actor)
			}
			w := httptest.NewRecorder()
			h.DownloadExport(w, r.WithContext(ctx))
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			if tc.status == http.StatusOK {
				if w.Body.String() != "a,b\n" || w.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatalf("body %q headers %v", w.Body.String(), w.Header())
				}
				if w.Header().Get("Content-Disposition") == "" {
					t.Fatal("non-ASCII filename produced no Content-Disposition")
				}
			} else if w.Body.String() == "a,b\n" {
				t.Fatal("file content served to a non-owner")
			}
		})
	}
}
