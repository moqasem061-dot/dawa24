package telegram

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
)

func TestTelegram_DocumentWithCaption(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	activeLink(repo, userA, tgA, int64p(orgOne))

	asst := &fakeAssistant{
		gate:   "pharmacy.assistant.use",
		answer: chatbridge.Answer{Markdown: "تم فحص الملف والأصناف المطلوبة متوفرة."},
	}
	grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}
	svc := newTestService(repo, grants, asst)

	fileData := base64.StdEncoding.EncodeToString([]byte("اسم الصنف,الكمية\nكونجستال,5\n"))
	update := Update{
		UpdateID: 101,
		Message: &Message{
			MessageID: 1,
			From:      &User{ID: tgA},
			Chat:      Chat{ID: tgA, Type: "private"},
			Caption:   "احسب تكلفة هذه النواقص",
			Document: &Document{
				FileID:   "doc-file-id-1",
				FileName: "needs.csv",
				FileData: fileData,
			},
		},
	}

	reply, err := svc.HandleUpdate(ctx, update)
	require.NoError(t, err)
	require.NotEmpty(t, reply.Messages)
	require.Contains(t, reply.Messages[0].Text, "تم فحص الملف والأصناف المطلوبة متوفرة.")

	require.Equal(t, "احسب تكلفة هذه النواقص", asst.lastQ)
	require.Equal(t, []string{"att-needs.csv"}, asst.lastRefs)
}

func TestTelegram_PhotoWithoutCaption(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	activeLink(repo, userB, tgB, int64p(orgOne))

	asst := &fakeAssistant{
		gate:   "pharmacy.assistant.use",
		answer: chatbridge.Answer{Markdown: "هذه روشتة تحتوي على أوجمنتين."},
	}
	grants := fakeGrants{{userB, orgOne}: memberGrant(userB, orgOne, "customer", "approved", "pharmacy.assistant.use")}
	svc := newTestService(repo, grants, asst)

	photoData := base64.StdEncoding.EncodeToString([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIFfakeimage"))
	update := Update{
		UpdateID: 102,
		Message: &Message{
			MessageID: 2,
			From:      &User{ID: tgB},
			Chat:      Chat{ID: tgB, Type: "private"},
			Photo: []PhotoSize{
				{FileID: "thumb", Width: 100, Height: 100, FileSize: 50},
				{FileID: "large", Width: 800, Height: 800, FileSize: 500, FileData: photoData},
			},
		},
	}

	reply, err := svc.HandleUpdate(ctx, update)
	require.NoError(t, err)
	require.NotEmpty(t, reply.Messages)
	require.Contains(t, reply.Messages[0].Text, "هذه روشتة تحتوي على أوجمنتين.")

	require.Len(t, asst.lastRefs, 1)
	require.Contains(t, asst.lastRefs[0], "att-telegram_photo_")
}
