package assistant_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
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

type fakeAudioGateway struct {
	gateway.Client
	transcribeText string
	transcribeErr  error
	called         bool
}

func (f *fakeAudioGateway) Capabilities(context.Context, gateway.Role) (gateway.ModelCapabilities, error) {
	return gateway.ModelCapabilities{Audio: false}, nil
}

func (f *fakeAudioGateway) Transcribe(_ context.Context, _ gateway.TranscribeRequest) (string, error) {
	f.called = true
	return f.transcribeText, f.transcribeErr
}

func TestService_ResolveAudioAttachment_TranscribesSpeech(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryAttachmentRepo()
	gw := &fakeAudioGateway{transcribeText: "أريد 10 علب بنادول اكسترا"}
	svc := assistant.NewService(repo, gw, nil, nil)
	actor := authctx.Actor{UserID: 10, OrgID: 5}

	voiceData := []byte("OggSfakeaudiocontent")
	row, err := svc.IngestAttachment(ctx, actor, "telegram_voice.ogg", voiceData)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, "audio/ogg", row.MIMEType)

	ref := row.PublicID.String()
	atts, digests, parts := svc.ResolveAttachments(ctx, actor, []string{ref})
	require.Len(t, atts, 1)
	require.Len(t, digests, 1)
	require.Empty(t, parts) // Audio must be transcribed into digest, not sent as raw part
	require.True(t, gw.called)
	require.Equal(t, "telegram_voice.ogg", digests[0].Filename)
	require.Equal(t, "أريد 10 علب بنادول اكسترا", digests[0].Text)
}

func TestService_ResolveAudioAttachment_StaleDigestRefreshes(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryAttachmentRepo()
	gw := &fakeAudioGateway{transcribeText: "أريد 5 علب أوجمنتين"}
	svc := assistant.NewService(repo, gw, nil, nil)
	actor := authctx.Actor{UserID: 10, OrgID: 5}

	voiceData := []byte("OggSfakeaudiocontent")
	row, err := svc.IngestAttachment(ctx, actor, "telegram_voice.ogg", voiceData)
	require.NoError(t, err)

	// Simulate stale hallucinated apology previously cached in DB
	row.Digest = "التسجيل ده مفيهوش محتوى صوتي مُفرّغ — اللي وصلني نصه مجرد طلب لتفريغ التسجيل نفسه..."
	repo.digests[row.ID] = row.Digest

	ref := row.PublicID.String()
	_, digests, _ := svc.ResolveAttachments(ctx, actor, []string{ref})
	require.Len(t, digests, 1)
	require.True(t, gw.called)
	require.Equal(t, "أريد 5 علب أوجمنتين", digests[0].Text)
}
