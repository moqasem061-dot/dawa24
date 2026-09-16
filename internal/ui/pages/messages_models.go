package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ChatListItem is one conversation in the messages sidebar, with the other
// party's name resolved for display.
type ChatListItem struct {
	Conversation *chat.Conversation
	OtherName    i18n.Text
}

// MessagesData is the /messages two-pane view model.
type MessagesData struct {
	Items        []ChatListItem
	Active       *chat.Conversation
	ActiveOther  i18n.Text
	Thread       []*chat.Message
	CurrentOrgID int64
}
