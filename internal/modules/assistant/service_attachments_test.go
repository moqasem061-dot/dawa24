package assistant_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type memoryAttachmentRepo struct {
	assistant.Repository
	rows    map[string]*assistant.AttachmentRow
	blobs   map[int64][]byte
	digests map[int64]string
	nextID  int64
}

func newMemoryAttachmentRepo() *memoryAttachmentRepo {
	return &memoryAttachmentRepo{
		rows:    make(map[string]*assistant.AttachmentRow),
		blobs:   make(map[int64][]byte),
		digests: make(map[int64]string),
		nextID:  1,
	}
}

func (m *memoryAttachmentRepo) CreateAttachment(_ context.Context, a *assistant.AttachmentRow) error {
	a.ID = m.nextID
	m.nextID++
	if a.PublicID == uuid.Nil {
		a.PublicID = uuid.New()
	}
	m.rows[a.PublicID.String()] = a
	return nil
}

func (m *memoryAttachmentRepo) GetAttachment(_ context.Context, publicID string, orgID, userID int64) (*assistant.AttachmentRow, error) {
	row, ok := m.rows[publicID]
	if !ok || row.OrganizationID != orgID || row.UserID != userID {
		return nil, nil
	}
	return row, nil
}

func (m *memoryAttachmentRepo) SaveAttachmentContent(_ context.Context, id int64, content []byte) error {
	m.blobs[id] = content
	return nil
}

func (m *memoryAttachmentRepo) LoadAttachmentContent(_ context.Context, id int64) ([]byte, error) {
	return m.blobs[id], nil
}

func (m *memoryAttachmentRepo) SetAttachmentDigest(_ context.Context, id int64, digest string) error {
	m.digests[id] = digest
	return nil
}

func TestService_IngestAndResolveSpreadsheet(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryAttachmentRepo()
	svc := assistant.NewService(repo, nil, nil, nil)
	actor := authctx.Actor{UserID: 10, OrgID: 5}

	csvData := []byte("الصنف,الكمية\nكونجستال,5\nبانادول,10\n")
	row, err := svc.IngestAttachment(ctx, actor, "orders.csv", csvData)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, "orders.csv", row.Filename)
	require.Equal(t, "text/csv", row.MIMEType)

	ref := row.PublicID.String()
	atts, digests, parts := svc.ResolveAttachments(ctx, actor, []string{ref})
	require.Len(t, atts, 1)
	require.Len(t, digests, 1)
	require.Empty(t, parts) // CSV is not sent as direct vision part

	require.Equal(t, "orders.csv", digests[0].Filename)
	require.Contains(t, digests[0].Text, "| الصنف | الكمية |")
	require.Contains(t, digests[0].Text, "| كونجستال | 5 |")
	require.Contains(t, digests[0].Text, "| بانادول | 10 |")
}

func TestService_IngestAttachment_Validation(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryAttachmentRepo()
	svc := assistant.NewService(repo, nil, nil, nil)
	actor := authctx.Actor{UserID: 10, OrgID: 5}

	// Empty content
	_, err := svc.IngestAttachment(ctx, actor, "empty.txt", nil)
	require.Error(t, err)

	// Forbidden extension
	_, err = svc.IngestAttachment(ctx, actor, "virus.exe", []byte("MZ....."))
	require.Error(t, err)
}
