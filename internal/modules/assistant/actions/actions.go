// Package actions is how the assistant changes things: by proposing, never by
// doing.
//
// A language model can be talked into anything — by the user, by a product
// description a supplier wrote, by a PDF. So the assistant's authority to act
// is split in two, and the model only ever holds the first half:
//
//  1. propose — the model names an action and its arguments. The server checks
//     the caller, resolves every reference as a signed handle, runs the same
//     authorization and validation the dashboard runs, and renders a preview
//     of exactly what will happen. It stores that as a pending action bound to
//     the user, the organisation and the conversation, for ten minutes, once.
//  2. confirm — a person presses a button (in the web drawer or on Telegram).
//     That is a new request with its own authentication. The server re-resolves
//     the caller's live grant, re-runs authorization against current state,
//     re-renders the preview and refuses if it no longer matches what the
//     person saw, then executes through the dashboard's own code path.
//
// Nothing in this package knows how to add to a cart or accept an order. The
// platform implements Executor with the code its screens already run; this
// package defines the contract and the argument discipline.
package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Risk decides how much the confirmation card shows and how it is worded.
type Risk string

const (
	// RiskLow changes something easily undone: a cart line, a favourite.
	RiskLow Risk = "low"
	// RiskHigh commits money, stock or a counterparty: placing or cancelling
	// an order, a price, an approval.
	RiskHigh Risk = "high"
)

// ParamType is the shape of one argument.
type ParamType string

const (
	ParamRef    ParamType = "ref"
	ParamInt    ParamType = "int"
	ParamNumber ParamType = "number"
	ParamString ParamType = "string"
	ParamEnum   ParamType = "enum"
	ParamBool   ParamType = "bool"
)

// Param declares one argument.
type Param struct {
	Name        string
	Description string
	Type        ParamType
	// RefKind is the handle kind a ref argument must carry.
	RefKind  handles.Kind
	Required bool
	// Min and Max bound int and number arguments.
	Min, Max float64
	// MaxLen bounds string arguments; zero means 500.
	MaxLen int
	// Values lists the allowed enum values.
	Values []string
}

// Definition is one action as the assistant offers it.
type Definition struct {
	Name string
	// Label is the Arabic name shown on the confirmation card.
	Label string
	// Description tells the model when to use it and where its refs come from.
	Description string
	Risk        Risk
	Params      []Param
}

// Detail is one labelled line of a preview.
type Detail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Preview is what the person confirms. Everything in it is rendered by the
// server from current data; nothing is text the model wrote.
type Preview struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Details  []Detail `json:"details,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Outcome is the result of an executed action.
type Outcome struct {
	Message string `json:"message"`
	// URL is a site-relative page showing the result, when there is one.
	URL string `json:"url,omitempty"`
}

// Executor is implemented by the platform with the code its dashboard runs.
//
// Both methods receive the live actor and arguments whose references are
// already verified ids. Both must perform the full authorization the equivalent
// dashboard request performs — audience, organisation approval, permission,
// ownership and state — because Execute runs long after Prepare, for an actor
// whose grant may have changed in between.
type Executor interface {
	// Definitions lists every action this caller's dashboard offers, before
	// permission filtering.
	Definitions(actor authctx.Actor) []Definition
	// Permitted reports whether the caller may use an action right now.
	Permitted(actor authctx.Actor, name string) bool
	// Prepare validates and authorizes an action and renders its preview.
	// It changes nothing.
	Prepare(ctx context.Context, actor authctx.Actor, name string, args Args) (Preview, error)
	// Execute performs it.
	Execute(ctx context.Context, actor authctx.Actor, name string, args Args) (Outcome, error)
}

// Refusal is an expected "no" with a message safe to show the user and the
// model: out of stock, not your order, already shipped.
type Refusal struct{ Message string }

func (r *Refusal) Error() string { return r.Message }

// Refuse builds a Refusal.
func Refuse(format string, a ...any) error { return &Refusal{Message: fmt.Sprintf(format, a...)} }

// AsRefusal extracts a Refusal.
func AsRefusal(err error) (*Refusal, bool) {
	var r *Refusal
	ok := errors.As(err, &r)
	return r, ok
}
