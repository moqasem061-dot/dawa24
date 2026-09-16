package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// mockAttachmentsRepo implements attachments.Repository for testing
type mockAttachmentsRepo struct {
	attachments.Repository
	docs     map[int64]*attachments.Document
	orgDocs  map[int64][]*attachments.Document
	verified map[int64]attachments.DocumentType
}

func newMockAttachmentsRepo() *mockAttachmentsRepo {
	return &mockAttachmentsRepo{
		docs:     make(map[int64]*attachments.Document),
		orgDocs:  make(map[int64][]*attachments.Document),
		verified: make(map[int64]attachments.DocumentType),
	}
}

func (m *mockAttachmentsRepo) ListByOrganization(ctx context.Context, orgID int64) ([]*attachments.Document, error) {
	return m.orgDocs[orgID], nil
}

func (m *mockAttachmentsRepo) GetByID(ctx context.Context, id int64) (*attachments.Document, error) {
	return m.docs[id], nil
}

func (m *mockAttachmentsRepo) UpdateTypeAndStatus(ctx context.Context, id int64, docType attachments.DocumentType, status attachments.DocumentStatus, notes string, reviewerID *int64) error {
	m.verified[id] = docType
	if d, ok := m.docs[id]; ok {
		d.DocumentType = docType
		d.Status = status
		d.ReviewNotes = notes
	}
	return nil
}

func (m *mockAttachmentsRepo) UpdateStatus(ctx context.Context, id int64, status attachments.DocumentStatus, notes string, reviewerID *int64) error {
	if d, ok := m.docs[id]; ok {
		d.Status = status
		d.ReviewNotes = notes
	}
	return nil
}

func (m *mockAttachmentsRepo) FulfillRequestByDoc(ctx context.Context, orgID int64, docType attachments.DocumentType, docID int64) error {
	return nil
}

func TestVerifyOrgDocumentsOnApproval_PreservesDistinctDocumentTypes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockAttachmentsRepo()
	attSvc := attachments.NewService(repo, nil, logger)

	orgID := int64(100)
	doc1 := &attachments.Document{
		ID:             1,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocCommercialRegister,
		OriginalName:   "commercial_reg.pdf",
		Status:         attachments.StatusPending,
	}
	doc2 := &attachments.Document{
		ID:             2,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocTaxCard,
		OriginalName:   "tax_card.png",
		Status:         attachments.StatusPending,
	}
	doc3 := &attachments.Document{
		ID:             3,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocPharmacyLicense,
		OriginalName:   "pharmacy_license.jpg",
		Status:         attachments.StatusPending,
	}
	doc4 := &attachments.Document{
		ID:             4,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocAuthorizationLetter,
		OriginalName:   "auth_letter.pdf",
		Status:         attachments.StatusPending,
	}

	repo.docs[1] = doc1
	repo.docs[2] = doc2
	repo.docs[3] = doc3
	repo.docs[4] = doc4
	repo.orgDocs[orgID] = []*attachments.Document{doc1, doc2, doc3, doc4}

	h := &UIHandler{
		attSvc: attSvc,
		log:    logger,
	}

	actor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}

	// Run verifyOrgDocumentsOnApproval without overrides: all documents must preserve their distinct types
	h.verifyOrgDocumentsOnApproval(context.Background(), actor, orgID, "Verified by Admin", nil)

	if repo.verified[1] != attachments.DocCommercialRegister {
		t.Errorf("doc1 type overwritten: got %s, want %s", repo.verified[1], attachments.DocCommercialRegister)
	}
	if repo.verified[2] != attachments.DocTaxCard {
		t.Errorf("doc2 type overwritten: got %s, want %s", repo.verified[2], attachments.DocTaxCard)
	}
	if repo.verified[3] != attachments.DocPharmacyLicense {
		t.Errorf("doc3 type overwritten: got %s, want %s", repo.verified[3], attachments.DocPharmacyLicense)
	}
	if repo.verified[4] != attachments.DocAuthorizationLetter {
		t.Errorf("doc4 type overwritten: got %s, want %s", repo.verified[4], attachments.DocAuthorizationLetter)
	}

	// Verify all statuses were set to verified
	for id, doc := range repo.docs {
		if doc.Status != attachments.StatusVerified {
			t.Errorf("doc %d status = %s, want %s", id, doc.Status, attachments.StatusVerified)
		}
	}
}

func TestVerifyOrgDocumentsOnApproval_WithSpecificTypeOverride(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockAttachmentsRepo()
	attSvc := attachments.NewService(repo, nil, logger)

	orgID := int64(200)
	docA := &attachments.Document{
		ID:             10,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocOther,
		OriginalName:   "unknown_scan.pdf",
		Status:         attachments.StatusPending,
	}
	docB := &attachments.Document{
		ID:             20,
		OrganizationID: &orgID,
		DocumentType:   attachments.DocTaxCard,
		OriginalName:   "tax_card.pdf",
		Status:         attachments.StatusPending,
	}

	repo.docs[10] = docA
	repo.docs[20] = docB
	repo.orgDocs[orgID] = []*attachments.Document{docA, docB}

	h := &UIHandler{
		attSvc: attSvc,
		log:    logger,
	}

	actor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}

	// Admin reclassified docA to DocCommercialRegister, but kept docB as Tax Card
	overrides := map[int64]attachments.DocumentType{
		10: attachments.DocCommercialRegister,
	}

	h.verifyOrgDocumentsOnApproval(context.Background(), actor, orgID, "Custom reclassification", overrides)

	if repo.verified[10] != attachments.DocCommercialRegister {
		t.Errorf("docA override failed: got %s, want %s", repo.verified[10], attachments.DocCommercialRegister)
	}
	if repo.verified[20] != attachments.DocTaxCard {
		t.Errorf("docB type unexpectedly changed: got %s, want %s", repo.verified[20], attachments.DocTaxCard)
	}
}
