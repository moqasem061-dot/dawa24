package attachments

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/stretchr/testify/assert"
)

func TestRequirementsFor(t *testing.T) {
	t.Run("customer", func(t *testing.T) {
		reqs := RequirementsFor("customer")
		assert.Len(t, reqs, 5)
		for _, r := range reqs {
			assert.False(t, r.Required, "all document types should be individually optional")
		}
	})

	t.Run("vendor", func(t *testing.T) {
		reqs := RequirementsFor("vendor")
		assert.Len(t, reqs, 4)
		for _, r := range reqs {
			assert.NotEqual(t, DocPharmacistLicense, r.DocType)
			assert.False(t, r.Required, "all document types should be individually optional")
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
			name:    "one verified document clears",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
			},
		},
		{
			name:    "no verified documents fails",
			orgID:   77,
			orgType: "customer",
			docs:    []*Document{},
			want:    []DocumentType{DocCommercialRegister},
		},
		{
			name:    "pending does not satisfy when no verified docs exist",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusPending, false),
				doc(DocTaxCard, StatusPending, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "rejected does not satisfy when no verified docs exist",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusRejected, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "soft deleted does not satisfy",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, true),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "at least one verified among mixed docs clears",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusPending, false),
				doc(DocTaxCard, StatusVerified, false),
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
