package identity

import (
	"context"
	"testing"
)

func TestModeratorService_ModeratorParentID(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)

	ctx := context.Background()
	parentID := int64(10)
	subordinateID := int64(20)

	// Initially nil
	p, err := svc.ModeratorParentID(ctx, subordinateID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil parent, got %v", p)
	}

	// Set parent
	if err := repo.SetModeratorParent(ctx, subordinateID, &parentID, 1); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	p, err = svc.ModeratorParentID(ctx, subordinateID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil || *p != parentID {
		t.Fatalf("expected parent %d, got %v", parentID, p)
	}
}
