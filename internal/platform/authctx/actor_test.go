package authctx_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func TestUserIDRequiresAnAuthenticatedActor(t *testing.T) {
	// The whole point of this package: with no actor in context there is no
	// user id to be had. Handlers previously fell back to a query parameter,
	// which meant an anonymous caller could name any user.
	if _, err := authctx.UserID(context.Background()); err == nil {
		t.Fatal("UserID succeeded without an authenticated actor")
	}

	ctx := authctx.WithActor(context.Background(), authctx.Actor{UserID: 42})
	id, err := authctx.UserID(ctx)
	if err != nil || id != 42 {
		t.Fatalf("UserID = %d, %v; want 42, nil", id, err)
	}
}

func TestZeroUserIDIsNotAnActor(t *testing.T) {
	// A zero-value Actor must not count as authenticated, or a struct that was
	// never populated would read as "user 0".
	ctx := authctx.WithActor(context.Background(), authctx.Actor{})
	if _, ok := authctx.From(ctx); ok {
		t.Error("a zero-value actor was treated as authenticated")
	}
}

func TestSameUserOrForbidden(t *testing.T) {
	owner := authctx.WithActor(context.Background(), authctx.Actor{UserID: 7})
	support := authctx.WithActor(context.Background(), authctx.Actor{
		UserID: 9, Permissions: []string{"support.read_any_user"},
	})
	other := authctx.WithActor(context.Background(), authctx.Actor{UserID: 9})

	if err := authctx.SameUserOrForbidden(owner, 7, "support.read_any_user"); err != nil {
		t.Errorf("owner denied access to own data: %v", err)
	}
	if err := authctx.SameUserOrForbidden(support, 7, "support.read_any_user"); err != nil {
		t.Errorf("permitted override denied: %v", err)
	}

	err := authctx.SameUserOrForbidden(other, 7, "support.read_any_user")
	if err == nil {
		t.Fatal("a different user reached someone else's data")
	}
	if apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("kind = %s, want forbidden", apperr.KindOf(err))
	}

	if err := authctx.SameUserOrForbidden(context.Background(), 7, ""); err == nil {
		t.Error("anonymous caller reached another user's data")
	}
}
