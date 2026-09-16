package attachments

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type stubRepo struct {
	created *Document
	docs    []*Document
	listErr error
}

func (s *stubRepo) Create(_ context.Context, doc *Document) (*Document, error) {
	s.created = doc
	doc.ID = 1
	doc.PublicID = uuid.New()
	return doc, nil
}
func (s *stubRepo) GetByID(context.Context, int64) (*Document, error)           { return nil, nil }
func (s *stubRepo) GetByPublicID(context.Context, uuid.UUID) (*Document, error) { return nil, nil }
func (s *stubRepo) ListByOrganization(context.Context, int64) ([]*Document, error) {
	return s.docs, s.listErr
}
func (s *stubRepo) ListByUser(context.Context, int64) ([]*Document, error) { return nil, nil }
func (s *stubRepo) ListAll(context.Context, DocumentFilter) ([]*Document, int, error) {
	return nil, 0, nil
}
func (s *stubRepo) UpdateStatus(context.Context, int64, DocumentStatus, string, *int64) error {
	return nil
}
func (s *stubRepo) UpdateTypeAndStatus(context.Context, int64, DocumentType, DocumentStatus, string, *int64) error {
	return nil
}
func (s *stubRepo) SoftDelete(context.Context, int64) error { return nil }
func (s *stubRepo) HardDelete(context.Context, int64) error { return nil }
func (s *stubRepo) CreateDocumentRequest(_ context.Context, req *DocumentRequest) (*DocumentRequest, error) {
	req.ID = 1
	return req, nil
}
func (s *stubRepo) ListRequestsByOrg(context.Context, int64) ([]*DocumentRequest, error) {
	return nil, nil
}
func (s *stubRepo) ListAllRequests(context.Context) ([]*DocumentRequest, error) {
	return nil, nil
}
func (s *stubRepo) UpdateRequestStatus(context.Context, int64, DocumentRequestStatus, *int64) error {
	return nil
}
func (s *stubRepo) FulfillRequestByDoc(context.Context, int64, DocumentType, int64) error {
	return nil
}

func actor(orgID, userID int64) authctx.Actor {
	// authctx.From() normalizes the OrgID alias onto OrganizationID; the test
	// mirrors that so the service sees a real request-shaped actor.
	return authctx.Actor{OrganizationID: orgID, OrgID: orgID, UserID: userID}
}

func TestRegisterUpload_RequiresOrganization(t *testing.T) {
	svc := NewService(&stubRepo{}, nil, nil)
	if _, err := svc.RegisterUpload(context.Background(), actor(0, 1), DocCommercialRegister, "/uploads/x.pdf", "x.pdf"); err == nil {
		t.Fatal("want Unauthorized when the actor has no organization")
	}
}

func TestRegisterUpload_RejectsUnknownType(t *testing.T) {
	svc := NewService(&stubRepo{}, nil, nil)
	if _, err := svc.RegisterUpload(context.Background(), actor(1, 1), DocumentType("fake"), "/uploads/x.pdf", "x.pdf"); err == nil {
		t.Fatal("want validation error for an unknown document type")
	}
}

func TestRegisterUpload_RejectsUnsupportedMime(t *testing.T) {
	svc := NewService(&stubRepo{}, nil, nil)
	if _, err := svc.RegisterUpload(context.Background(), actor(1, 1), DocCommercialRegister, "/uploads/x.exe", "x.exe"); err == nil {
		t.Fatal("want validation error for an unsupported extension")
	}
}

func TestRegisterUpload_RejectsEmptyURL(t *testing.T) {
	svc := NewService(&stubRepo{}, nil, nil)
	if _, err := svc.RegisterUpload(context.Background(), actor(1, 1), DocCommercialRegister, "  ", "x.pdf"); err == nil {
		t.Fatal("want validation error for an empty file URL")
	}
}

func TestRegisterUpload_CreatesPendingDocument(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil, nil)

	got, err := svc.RegisterUpload(context.Background(), actor(7, 42), DocTaxCard, "/uploads/documents/tax.pdf", "tax.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("want pending status, got %s", got.Status)
	}
	if repo.created == nil || repo.created.OrganizationID == nil || *repo.created.OrganizationID != 7 {
		t.Fatal("document must be owned by the actor's organization")
	}
	if repo.created.FileURL != "/uploads/documents/tax.pdf" {
		t.Fatalf("want stored file url, got %q", repo.created.FileURL)
	}
	if repo.created.DocumentType != DocTaxCard {
		t.Fatalf("want tax_card type, got %s", repo.created.DocumentType)
	}
}

func TestRegisterUpload_SupportsSyndicateCard(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil, nil)

	got, err := svc.RegisterUpload(context.Background(), actor(5, 12), DocSyndicateCard, "/uploads/documents/card.png", "card.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DocumentType != DocSyndicateCard {
		t.Fatalf("want syndicate_card, got %s", got.DocumentType)
	}
}

func TestIsComplianceDocType(t *testing.T) {
	complianceTypes := []DocumentType{
		DocCommercialRegister, DocTaxCard, DocPharmacistLicense,
		DocPharmacyLicense, DocNationalID, DocPassport,
		DocBankCertificate, DocAuthorizationLetter, DocSyndicateCard, DocOther,
	}
	for _, ct := range complianceTypes {
		if !IsComplianceDocType(ct) {
			t.Errorf("expected %s to be compliance doc type", ct)
		}
	}

	nonComplianceTypes := []DocumentType{
		DocImportFile, DocAvatar, DocOrgLogo, DocProductImage, DocReviewImage,
	}
	for _, nct := range nonComplianceTypes {
		if IsComplianceDocType(nct) {
			t.Errorf("expected %s NOT to be compliance doc type", nct)
		}
	}
}
