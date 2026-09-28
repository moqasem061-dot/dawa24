package attachments

import (
	"context"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/stretchr/testify/assert"
)

func TestRequirementsFor(t *testing.T) {
	t.Run("customer", func(t *testing.T) {
		reqs := RequirementsFor("customer")
		assert.Len(t, reqs, 5)
		for _, r := range reqs {
			if r.DocType == DocCommercialRegister || r.DocType == DocPharmacyLicense {
				assert.True(t, r.Required, "commercial register and pharmacy license must be mandatory")
			} else {
				assert.False(t, r.Required, "other document types should be optional")
			}
		}
	})

	t.Run("vendor", func(t *testing.T) {
		reqs := RequirementsFor("vendor")
		assert.Len(t, reqs, 4)
		for _, r := range reqs {
			assert.NotEqual(t, DocPharmacistLicense, r.DocType)
			if r.DocType == DocCommercialRegister || r.DocType == DocPharmacyLicense {
				assert.True(t, r.Required, "commercial register and pharmacy license must be mandatory")
			} else {
				assert.False(t, r.Required, "other document types should be optional")
			}
		}
	})

	t.Run("unknown treated as customer", func(t *testing.T) {
		assert.Len(t, RequirementsFor("wholesaler"), 5)
	})
}

func TestService_MissingRequiredDocuments(t *testing.T) {
	ctx := authctx.WithActor(context.Background(), authctx.Actor{
		OrganizationID: 77,
		UserID:         1,
	})

	now := time.Now().UTC()
	doc := func(typ DocumentType, status DocumentStatus, deleted bool) *Document {
		d := &Document{
			OrganizationID: ptrI64(77),
			DocumentType:   typ,
			Status:         status,
			CreatedAt:      now,
		}
		if deleted {
			d.DeletedAt = &now
		}
		return d
	}

	tests := []struct {
		name    string
		orgID   int64
		orgType string
		docs    []*Document
		want    []DocumentType
		wantErr bool
	}{
		{
			name:    "no org",
			orgID:   0,
			wantErr: true,
		},
		{
			name:    "both mandatory verified documents clear",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
			},
		},
		{
			name:    "no verified documents fails with both required",
			orgID:   77,
			orgType: "customer",
			docs:    []*Document{},
			want:    []DocumentType{DocCommercialRegister, DocPharmacyLicense},
		},
		{
			name:    "only commercial register verified leaves pharmacy license missing",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
			},
			want: []DocumentType{DocPharmacyLicense},
		},
		{
			name:    "only pharmacy license verified leaves commercial register missing",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocPharmacyLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "pending does not satisfy mandatory requirement",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
				doc(DocPharmacyLicense, StatusPending, false),
			},
			want: []DocumentType{DocPharmacyLicense},
		},
		{
			name:    "rejected does not satisfy mandatory requirement",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusRejected, false),
				doc(DocPharmacyLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "soft deleted does not satisfy",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, true),
				doc(DocPharmacyLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "tax card optional and does not block when mandatory are verified",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
				doc(DocTaxCard, StatusPending, false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{repo: &stubRepo{docs: tt.docs}}
			got, err := svc.MissingRequiredDocuments(ctx, tt.orgID, tt.orgType)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestService_MissingRequiredDocuments_error(t *testing.T) {
	ctx := authctx.WithActor(context.Background(), authctx.Actor{OrganizationID: 1, UserID: 1})
	svc := &Service{repo: &stubRepo{listErr: errDocList}}
	_, err := svc.MissingRequiredDocuments(ctx, 1, "customer")
	assert.Error(t, err)
}

var errDocList = apperr.Internal(assert.AnError)

func ptrI64(v int64) *int64 { return &v }
