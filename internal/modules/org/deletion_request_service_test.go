package org

import (
	"context"
	"testing"
)

func TestRequestOrganizationDeletion_EmptyReason(t *testing.T) {
	repo := newMockOrgRepo()
	svc := NewService(repo, nil)

	_, err := svc.RequestOrganizationDeletion(context.Background(), 1, 10, "   ")
	if err == nil {
		t.Fatal("expected error for empty reason, got nil")
	}
}

func TestRequestOrganizationDeletion_Success(t *testing.T) {
	repo := newMockOrgRepo()
	svc := NewService(repo, nil)

	req, err := svc.RequestOrganizationDeletion(context.Background(), 1, 10, "Closing pharmacy branch permanently")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ID <= 0 {
		t.Errorf("expected positive ID, got %d", req.ID)
	}
	if req.Status != OrgDeletionStatusPending {
		t.Errorf("expected pending status, got %s", req.Status)
	}
	if req.OrganizationID != 1 || req.RequestedBy != 10 {
		t.Errorf("expected org 1 and user 10, got org %d, user %d", req.OrganizationID, req.RequestedBy)
	}
}

func TestCancelOrganizationDeletion(t *testing.T) {
	repo := newMockOrgRepo()
	svc := NewService(repo, nil)

	err := svc.CancelOrganizationDeletion(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
