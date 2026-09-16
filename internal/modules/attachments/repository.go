package attachments

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for platform_admin.documents.
type Repository interface {
	Create(ctx context.Context, doc *Document) (*Document, error)
	GetByID(ctx context.Context, id int64) (*Document, error)
	GetByPublicID(ctx context.Context, publicID uuid.UUID) (*Document, error)
	ListByOrganization(ctx context.Context, orgID int64) ([]*Document, error)
	ListByUser(ctx context.Context, userID int64) ([]*Document, error)
	ListAll(ctx context.Context, filter DocumentFilter) ([]*Document, int, error)
	UpdateStatus(ctx context.Context, id int64, status DocumentStatus, notes string, reviewedBy *int64) error
	UpdateTypeAndStatus(ctx context.Context, id int64, docType DocumentType, status DocumentStatus, notes string, reviewedBy *int64) error
	SoftDelete(ctx context.Context, id int64) error
	HardDelete(ctx context.Context, id int64) error

	// Document Requests
	CreateDocumentRequest(ctx context.Context, req *DocumentRequest) (*DocumentRequest, error)
	ListRequestsByOrg(ctx context.Context, orgID int64) ([]*DocumentRequest, error)
	ListAllRequests(ctx context.Context) ([]*DocumentRequest, error)
	UpdateRequestStatus(ctx context.Context, id int64, status DocumentRequestStatus, submittedDocID *int64) error
	FulfillRequestByDoc(ctx context.Context, orgID int64, docType DocumentType, docID int64) error
}
