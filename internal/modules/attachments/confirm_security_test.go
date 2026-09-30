package attachments

import (
	"context"
	"testing"
)

type confirmationRepo struct {
	Repository
	document *Document
	deleted  bool
}

func (r *confirmationRepo) GetByID(context.Context, int64) (*Document, error) { return r.document, nil }
func (r *confirmationRepo) HardDelete(context.Context, int64) error           { r.deleted = true; return nil }

func TestConfirmUploadEnforcesOwnershipWithoutUploader(t *testing.T) {
	orgID := int64(11)
	repo := &confirmationRepo{document: &Document{ID: 1, OrganizationID: &orgID}}
	svc := NewService(repo, nil, nil)
	if _, err := svc.ConfirmUpload(context.Background(), actor(10, 12), 1); err == nil {
		t.Fatal("foreign document without user accepted")
	}
	if repo.deleted {
		t.Fatal("unauthorized confirmation deleted document")
	}
	if _, err := svc.ConfirmUpload(context.Background(), actor(11, 12), 1); err != nil {
		t.Fatalf("own organization document refused: %v", err)
	}
	userID := int64(12)
	repo.document = &Document{ID: 2, UserID: &userID}
	if _, err := svc.ConfirmUpload(context.Background(), actor(10, 12), 2); err != nil {
		t.Fatalf("own personal document refused: %v", err)
	}
	repo.document = &Document{ID: 3}
	if _, err := svc.ConfirmUpload(context.Background(), actor(10, 12), 3); err == nil {
		t.Fatal("unowned document accepted")
	}
}
