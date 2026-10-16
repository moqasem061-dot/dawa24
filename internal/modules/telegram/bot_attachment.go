package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ingestTelegramAttachment extracts, downloads and ingests documents, photos,
// voice notes, and audio files sent to the Telegram bot into the assistant's
// attachments store.
func (s *Service) ingestTelegramAttachment(ctx context.Context, link *Link, msg *Message) (string, error) {
	if s.assistant == nil {
		return "", errors.New("assistant service is not configured")
	}

	actor, refusal, err := s.core.Authorize(ctx, link.chat())
	if err != nil {
		return "", err
	}
	if refusal != "" {
		return "", errors.New(refusal)
	}

	var (
		content  []byte
		filename string
	)

	if msg.Document != nil {
		doc := msg.Document
		filename = doc.FileName
		if filename == "" {
			filename = "telegram_document"
		}
		content, err = s.DownloadFile(ctx, doc.FileID, doc.FileURL, doc.FileData)
		if err != nil {
			return "", err
		}
	} else if len(msg.Photo) > 0 {
		// Pick highest resolution photo
		p := msg.Photo[len(msg.Photo)-1]
		filename = fmt.Sprintf("telegram_photo_%d.jpg", time.Now().Unix())
		content, err = s.DownloadFile(ctx, p.FileID, p.FileURL, p.FileData)
		if err != nil {
			return "", err
		}
	} else if msg.Voice != nil {
		v := msg.Voice
		filename = fmt.Sprintf("telegram_voice_%d.oga", time.Now().Unix())
		content, err = s.DownloadFile(ctx, v.FileID, v.FileURL, v.FileData)
		if err != nil {
			return "", err
		}
	} else if msg.Audio != nil {
		a := msg.Audio
		filename = a.FileName
		if filename == "" {
			filename = fmt.Sprintf("telegram_audio_%d.mp3", time.Now().Unix())
		}
		content, err = s.DownloadFile(ctx, a.FileID, a.FileURL, a.FileData)
		if err != nil {
			return "", err
		}
	} else {
		return "", nil
	}

	if len(content) == 0 {
		return "", errors.New("الملف المرفق فارغ")
	}

	return s.assistant.IngestAttachment(ctx, actor, filename, content)
}
