package chat

import "testing"

func TestConversationValidate(t *testing.T) {
	cases := []struct {
		name    string
		c       *Conversation
		wantErr bool
	}{
		{"valid two distinct orgs", &Conversation{OrganizationID: 1, CounterpartyOrgID: 2}, false},
		{"missing organization", &Conversation{OrganizationID: 0, CounterpartyOrgID: 2}, true},
		{"missing counterparty", &Conversation{OrganizationID: 1, CounterpartyOrgID: 0}, true},
		{"same org both sides", &Conversation{OrganizationID: 1, CounterpartyOrgID: 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestMessageValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       *Message
		wantErr bool
	}{
		{"valid text", &Message{SenderUserID: 1, Body: "hello"}, false},
		{"valid attachment only", &Message{SenderUserID: 1, Attachments: []map[string]any{{"key": "cv.pdf"}}}, false},
		{"missing sender", &Message{Body: "hello"}, true},
		{"empty body and no attachment", &Message{SenderUserID: 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
