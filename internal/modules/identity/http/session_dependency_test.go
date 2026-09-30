package http_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	identityhttp "github.com/moqasem061-dot/dawa24/internal/modules/identity/http"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
)

type failingSessionRepo struct {
	identity.Repository
	lookupErr error
}

func (r *failingSessionRepo) GetUserByID(context.Context, int64) (*identity.User, error) {
	return &identity.User{ID: 101, Status: identity.StatusActive}, r.lookupErr
}

func TestSessionMiddlewarePreservesCookieOnDependencyFailure(t *testing.T) {
	for _, required := range []bool{true, false} {
		for _, path := range []string{"/market-discounts", "/api/v1/auth/me"} {
			t.Run(path+"/required="+strconv.FormatBool(required), func(t *testing.T) {
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				store := identity.NewSessionStore(nil, config.Session{TTL: time.Hour})
				sess := &identity.Session{UserID: 101, ActiveOrgID: 1}
				if err := store.Create(context.Background(), sess); err != nil {
					t.Fatal(err)
				}
				repo := &failingSessionRepo{lookupErr: errors.New("database temporarily unavailable")}
				svc := identity.NewService(repo, store, logger)
				reached := false
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(http.StatusNoContent) })
				middleware := identityhttp.OptionalAuth(svc, nil, "session", logger)
				if required {
					middleware = identityhttp.RequireAuth(svc, nil, "session", logger)
				}
				handler := middleware(next)
				request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
				request.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
				request.Header.Set("HX-Request", "true")
				failed := httptest.NewRecorder()
				handler.ServeHTTP(failed, request)
				if failed.Code != http.StatusServiceUnavailable || reached {
					t.Fatalf("got %d, handler reached=%v", failed.Code, reached)
				}
				if len(failed.Result().Cookies()) != 0 || failed.Header().Get("HX-Redirect") != "" || failed.Header().Get("Location") != "" {
					t.Fatal("dependency failure logged the browser out")
				}
				repo.lookupErr = nil
				recovered := httptest.NewRecorder()
				handler.ServeHTTP(recovered, request)
				if recovered.Code != http.StatusNoContent || !reached {
					t.Fatalf("same session did not recover: %d", recovered.Code)
				}
			})
		}
	}
}
