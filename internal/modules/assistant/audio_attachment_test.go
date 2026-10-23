package assistant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/stretchr/testify/require"
)

type audioReadingGateway struct {
	fakeAudioGateway
	audio       bool
	streamError bool
	requests    []gateway.ChatRequest
}

func (g *audioReadingGateway) Capabilities(_ context.Context, role gateway.Role) (gateway.ModelCapabilities, error) {
	return gateway.ModelCapabilities{Audio: g.audio && role == gateway.RolePrimary, AcceptedMIMETypes: []string{"audio/ogg"}}, nil
}

func (g *audioReadingGateway) Stream(_ context.Context, request gateway.ChatRequest) (<-chan gateway.StreamEvent, error) {
	g.requests = append(g.requests, request)
	events := make(chan gateway.StreamEvent, 2)
	events <- gateway.StreamEvent{Delta: "أريد ٥ علب من الدواء"}
	if g.streamError {
		events <- gateway.StreamEvent{Err: errors.New("connection lost")}
	}
	close(events)
	return events, nil
}

func TestAudioAttachmentUsesAudioCapableRoleWhenTranscriptionIsUnavailable(t *testing.T) {
	repo := newMemoryAttachmentRepo()
	gw := &audioReadingGateway{fakeAudioGateway: fakeAudioGateway{transcribeErr: gateway.ErrDisabled}, audio: true}
	svc := assistant.NewService(repo, gw, nil, nil)
	actor := authctx.Actor{UserID: 10, OrgID: 5}
	row, err := svc.IngestAttachment(context.Background(), actor, "voice.ogg", []byte("OggSvoice"))
	require.NoError(t, err)
	_, digests, parts := svc.ResolveAttachments(context.Background(), actor, []string{row.PublicID.String()})
	require.Empty(t, parts)
	require.Equal(t, "أريد ٥ علب من الدواء", digests[0].Text)
	require.Len(t, gw.requests, 1)
	request := gw.requests[0]
	require.Equal(t, gateway.RolePrimary, request.Role)
	require.Equal(t, actor.OrgID, request.OrgID)
	require.Equal(t, actor.UserID, request.UserID)
	require.Equal(t, gateway.PartAudio, request.Messages[1].Parts[0].Kind)
	require.Equal(t, "audio/ogg", request.Messages[1].Parts[0].MIMEType)
	require.Equal(t, digests[0].Text, repo.digests[row.ID])
}

func TestAudioAttachmentFailureKeepsWrittenInputFallbackWithoutCachingPartialTranscript(t *testing.T) {
	for _, audio := range []bool{false, true} {
		repo := newMemoryAttachmentRepo()
		gw := &audioReadingGateway{fakeAudioGateway: fakeAudioGateway{transcribeErr: gateway.ErrDisabled}, audio: audio, streamError: true}
		svc := assistant.NewService(repo, gw, nil, nil)
		actor := authctx.Actor{UserID: 10, OrgID: 5}
		row, err := svc.IngestAttachment(context.Background(), actor, "voice.ogg", []byte("OggSvoice"))
		require.NoError(t, err)
		_, digests, _ := svc.ResolveAttachments(context.Background(), actor, []string{row.PublicID.String()})
		require.Contains(t, digests[0].Text, "كتابة الأصناف كنص")
		require.Empty(t, repo.digests[row.ID])
		if !audio {
			require.Empty(t, gw.requests)
		}
	}
}
