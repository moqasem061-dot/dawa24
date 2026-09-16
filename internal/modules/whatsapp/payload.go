package whatsapp

import (
	"strings"
	"unicode/utf8"
)

// Cloud API request bodies.
//
// Limits (Cloud API reference): a text body is at most 4096 characters; an
// interactive message's body at most 1024, a reply button's title at most 20
// and its id at most 256; a template body parameter may not contain a newline,
// a tab or more than four consecutive spaces.

const (
	maxInteractiveBody = 1024
	maxTemplateParam   = 900
)

// Payload is a Cloud API /messages request body.
type Payload struct {
	MessagingProduct string       `json:"messaging_product"`
	RecipientType    string       `json:"recipient_type"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Text             *TextBody    `json:"text,omitempty"`
	Interactive      *Interactive `json:"interactive,omitempty"`
	Template         *Template    `json:"template,omitempty"`
}

// TextBody is a text message.
type TextBody struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

// Interactive is a message with reply buttons.
type Interactive struct {
	Type   string            `json:"type"`
	Body   InteractiveText   `json:"body"`
	Action InteractiveAction `json:"action"`
}

// InteractiveText is an interactive message's body.
type InteractiveText struct {
	Text string `json:"text"`
}

// InteractiveAction holds the buttons.
type InteractiveAction struct {
	Buttons []ReplyButton `json:"buttons"`
}

// ReplyButton is one reply button.
type ReplyButton struct {
	Type  string     `json:"type"`
	Reply ReplyTitle `json:"reply"`
}

// ReplyTitle is a reply button's id and title.
type ReplyTitle struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Template is an approved message template, the only thing WhatsApp delivers
// outside the 24-hour window.
type Template struct {
	Name       string              `json:"name"`
	Language   TemplateLanguage    `json:"language"`
	Components []TemplateComponent `json:"components"`
}

// TemplateLanguage is a template's language code.
type TemplateLanguage struct {
	Code string `json:"code"`
}

// TemplateComponent fills one template component.
type TemplateComponent struct {
	Type       string              `json:"type"`
	Parameters []TemplateParameter `json:"parameters"`
}

// TemplateParameter is one {{n}} value.
type TemplateParameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newPayload(to, kind string) *Payload {
	return &Payload{MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: kind}
}

func textPayload(to, body string) *Payload {
	p := newPayload(to, "text")
	p.Text = &TextBody{Body: body}
	return p
}

// buttonsPayload is a body with two reply buttons.
func buttonsPayload(to, body string, buttons ...ReplyTitle) *Payload {
	p := newPayload(to, "interactive")
	action := InteractiveAction{}
	for _, b := range buttons {
		action.Buttons = append(action.Buttons, ReplyButton{Type: "reply", Reply: b})
	}
	p.Interactive = &Interactive{Type: "button", Body: InteractiveText{Text: body}, Action: action}
	return p
}

// templatePayload fills a one-parameter body template with text.
func templatePayload(to, name, language, text string) *Payload {
	p := newPayload(to, "template")
	p.Template = &Template{
		Name:     name,
		Language: TemplateLanguage{Code: language},
		Components: []TemplateComponent{{
			Type:       "body",
			Parameters: []TemplateParameter{{Type: "text", Text: templateParam(text)}},
		}},
	}
	return p
}

// templateParam flattens text into what a template parameter accepts: one
// line, no formatting markers, no runs of spaces, within the length limit.
func templateParam(text string) string {
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(strings.NewReplacer("*", "", "~", "", "`", "").Replace(l)), " ")
	}
	var kept []string
	for _, l := range lines {
		if l != "" {
			kept = append(kept, l)
		}
	}
	return truncateRunes(strings.Join(kept, " — "), maxTemplateParam)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
