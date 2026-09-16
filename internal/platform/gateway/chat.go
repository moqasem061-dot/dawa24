package gateway

import (
	"errors"
	"strings"
)

// PartKind categorises multimodal content parts.
type PartKind string

const (
	PartText  PartKind = "text"
	PartImage PartKind = "image"
	PartAudio PartKind = "audio"
	PartVideo PartKind = "video"
	PartFile  PartKind = "file"
)

// ContentPart is one piece of multimodal content in the Gateway's OpenAI dialect.
type ContentPart struct {
	Kind     PartKind
	Text     string
	MIMEType string
	DataURL  string
	Filename string
}

// ChatMessage is one turn in a conversation.
//
// Role is the OpenAI vocabulary: system, user, assistant, tool. An assistant
// message that asked for tools carries ToolCalls; the tool results that answer
// it are separate messages with Role "tool" and the matching ToolCallID.
type ChatMessage struct {
	Role       string
	Text       string
	Parts      []ContentPart
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolSpec is one function the model may call.
//
// The Gateway forwards an OpenAI-shaped body verbatim to the upstream provider
// and translates tool_calls in both directions, so declaring tools here is the
// whole of what the Store has to do. What the Store must NOT do is trust the
// arguments that come back: see modules/assistant/tools, where every call is
// re-authorized against the live actor before it reaches a query.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall is one function invocation the model asked for.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON text, exactly as the model wrote it
}

// Usage reports token counts from a completion turn.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatRequest carries the parameters for a streaming completion.
type ChatRequest struct {
	Role        Role
	Messages    []ChatMessage
	MaxTokens   int
	Temperature float64
	// Tools are the functions the model may call this turn. They are read-only
	// data lookups; nothing the Store exposes to a model mutates state.
	Tools      []ToolSpec
	OrgID      int64
	UserID     int64
	VirtualKey string // Tenant virtual key
	// Feature names the screen that asked, for the usage ledger. See
	// Request.Feature.
	Feature string
}

// StreamEvent represents one decoded SSE chunk or lifecycle event.
type StreamEvent struct {
	Delta     string
	Reasoning string
	Done      bool
	Err       error
	Usage     *Usage
	// ToolCalls is non-empty on exactly one event per turn: the one that
	// reports the model finished by asking for tools. Fragments arrive spread
	// across many chunks and are reassembled before this is emitted.
	ToolCalls []ToolCall
	// FinishReason is the upstream's own word for why generation stopped.
	FinishReason string
}

// ErrToolsNotSupported is retained for callers that still branch on it. Tools
// are supported now; nothing in this package returns this any more.
var ErrToolsNotSupported = errors.New("gateway: tools not supported in this phase")

// base64Payload returns the bytes of a data URI without its "data:…;base64,"
// prefix. A value that is not a data URI is returned unchanged, so a caller
// that already holds bare base64 is not corrupted.
func base64Payload(dataURL string) string {
	if !strings.HasPrefix(dataURL, "data:") {
		return dataURL
	}
	if idx := strings.Index(dataURL, ","); idx != -1 {
		return dataURL[idx+1:]
	}
	return dataURL
}

// audioFormatFor names the container for an input_audio part.
//
// The MIME type is authoritative; the data URI is only consulted when the
// caller did not record one, which happens for parts assembled from a stored
// file whose row predates the MIME column being populated.
func audioFormatFor(mime, dataURL string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if m == "" && strings.HasPrefix(dataURL, "data:") {
		if idx := strings.IndexAny(dataURL[5:], ";,"); idx != -1 {
			m = strings.ToLower(dataURL[5 : 5+idx])
		}
	}
	if idx := strings.Index(m, ";"); idx != -1 {
		m = strings.TrimSpace(m[:idx])
	}
	switch m {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/webm":
		return "webm"
	case "audio/ogg":
		return "ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return "m4a"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav"
	}
	// Unknown containers keep the historical default rather than sending an
	// empty format, which the upstream rejects outright.
	return "wav"
}

func buildWireMessages(messages []ChatMessage) []wireChatMessage {
	wireMsgs := make([]wireChatMessage, 0, len(messages))
	for _, m := range messages {
		wm := wireChatMessage{Role: m.Role, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: wireToolCallFunc{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		if len(m.Parts) == 0 {
			wm.Content = m.Text
		} else {
			parts := make([]wireContentPart, 0, len(m.Parts))
			for _, p := range m.Parts {
				switch p.Kind {
				case PartText:
					parts = append(parts, wireContentPart{
						Type: "text",
						Text: p.Text,
					})
				case PartImage:
					parts = append(parts, wireContentPart{
						Type:     "image_url",
						ImageURL: &wireURLPart{URL: p.DataURL},
					})
				case PartVideo:
					parts = append(parts, wireContentPart{
						Type:     "video_url",
						VideoURL: &wireURLPart{URL: p.DataURL},
					})
				case PartAudio:
					// input_audio is the one part of this dialect that does NOT
					// take a data URI. The Gateway forwards {data, format}
					// straight through as OpenAI defines it: data is bare
					// base64 and format names the container. Sending the whole
					// "data:audio/webm;base64,…" string in data meant the
					// upstream decoded the prefix as audio and rejected every
					// voice note, and the hardcoded "wav" fallback mislabelled
					// webm and m4a recordings on top of that.
					parts = append(parts, wireContentPart{
						Type: "input_audio",
						InputAudio: &wireAudioPart{
							Data:   base64Payload(p.DataURL),
							Format: audioFormatFor(p.MIMEType, p.DataURL),
						},
					})
				case PartFile:
					fileMap := map[string]any{
						"url": p.DataURL,
					}
					if p.Filename != "" {
						fileMap["name"] = p.Filename
					}
					if p.MIMEType != "" {
						fileMap["mime_type"] = p.MIMEType
					}
					parts = append(parts, wireContentPart{
						Type: "file",
						File: fileMap,
					})
				default:
					parts = append(parts, wireContentPart{
						Type: "text",
						Text: p.Text,
					})
				}
			}
			wm.Content = parts
		}
		wireMsgs = append(wireMsgs, wm)
	}
	return wireMsgs
}

type wireChatRequest struct {
	Model       string            `json:"model"`
	Messages    []wireChatMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	Stream      bool              `json:"stream"`
	Tools       []wireTool        `json:"tools,omitempty"`
	ToolChoice  string            `json:"tool_choice,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireToolFunc `json:"function"`
}

type wireToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolCallFunc `json:"function"`
}

type wireToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireChatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// buildWireTools converts the Store's tool specs into the OpenAI shape the
// Gateway forwards unchanged.
func buildWireTools(tools []ToolSpec) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, wireTool{
			Type: "function",
			Function: wireToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

type wireContentPart struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	ImageURL   *wireURLPart   `json:"image_url,omitempty"`
	VideoURL   *wireURLPart   `json:"video_url,omitempty"`
	InputAudio *wireAudioPart `json:"input_audio,omitempty"`
	File       map[string]any `json:"file,omitempty"`
}

type wireURLPart struct {
	URL string `json:"url"`
}

type wireAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}
