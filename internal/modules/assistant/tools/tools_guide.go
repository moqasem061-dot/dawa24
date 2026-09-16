package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// platform_guide answers "where is…" and "how does…" from the platform's own
// declarations rather than from what the model imagines the product looks
// like: the screens come from the RBAC sidebar registry filtered by the
// caller's grants, the assistant's abilities from the action flow the caller
// is actually offered, and the order lifecycle from the commerce status machine
// the checkout and fulfilment code enforce.

func guideTools(r *Registry) []Tool {
	return []Tool{{
		Name: "platform_guide",
		Description: "How Dawa24 works for this user: screens they can open (with links), what the assistant can do for them, " +
			"and the order status lifecycle. Use for 'where do I find…', 'how do I…', 'what does this status mean'.",
		Params: objectSchema(map[string]any{
			"topic": enumProp("What to explain.", "screens", "assistant", "order_statuses"),
		}, "topic"),
		Scopes: allScopes, Permissions: anyGate, Timeout: 5 * time.Second,
		Handler: r.platformGuide,
	}}
}

func (r *Registry) platformGuide(_ context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Topic string `json:"topic"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	switch args.Topic {
	case "screens":
		return guideScreens(actor), nil
	case "assistant":
		return r.guideAssistant(actor), nil
	case "order_statuses":
		return guideStatuses(), nil
	}
	return Result{}, badArgs("topic must be screens, assistant or order_statuses")
}

func guideScreens(actor authctx.Actor) Result {
	held := rbac.NewSet(actor.Permissions)
	var sections []map[string]any
	n := 0
	for _, sec := range rbac.VisibleNav(actor.DashboardScope(), held) {
		items := make([]map[string]string, 0, len(sec.Items))
		for _, it := range sec.Items {
			items = append(items, map[string]string{"screen": it.NameAr, "url": it.Href})
			n++
		}
		sections = append(sections, map[string]any{"section": sec.NameAr, "screens": items})
	}
	return Result{Data: map[string]any{
		"sections": sections,
		"note":     "Only screens this user's role can open are listed. Give the URL as a link.",
	}, Rows: n}
}

func (r *Registry) guideAssistant(actor authctx.Actor) Result {
	var acts []map[string]string
	for _, d := range r.flow.Available(actor) {
		acts = append(acts, map[string]string{"action": d.Name, "label": d.Label, "risk": string(d.Risk)})
	}
	data := map[string]any{
		"reads":           "Any dataset from describe_data, within this user's permissions; exports up to 50,000 rows as a spreadsheet.",
		"actions":         acts,
		"confirmation":    "Actions are prepared, never performed by the assistant. The user confirms each on a card showing exactly what happens; it expires after 10 minutes and is re-checked against current data and permissions when confirmed.",
		"telegram":        "The same assistant works on Telegram after linking the account from Settings → تيليجرام; confirmation cards arrive with buttons and exports as files.",
		"retention":       "Conversations are deleted after six months; exported files after seven days.",
		"never_available": "Passwords, payment methods, wallet withdrawals, role and permission changes, and deleting accounts or organisations stay on the dashboard only.",
	}
	if len(acts) == 0 {
		data["actions_note"] = "This user's role has not been given actions through the assistant; the organisation owner can enable it in roles and permissions."
	}
	if _, ok := assistant.Allowed(actor); !ok {
		data = map[string]any{}
	}
	return Result{Data: data, Rows: len(acts) + 1}
}

// orderStatusGuide mirrors commerce.IsValidStatusTransition. The commerce
// module cannot be imported here; TestOrderStatusGuideMatchesCommerce in
// cmd/server holds the two together.
var orderStatusGuide = []map[string]any{
	{"status": "pending", "meaning": "placed, waiting for the supplier", "next": []string{"processing", "confirmed", "shipped", "delivered", "cancelled", "on_hold"}},
	{"status": "processing", "meaning": "being prepared", "next": []string{"confirmed", "shipped", "delivered", "on_hold", "cancelled", "failed"}},
	{"status": "confirmed", "meaning": "accepted by the supplier", "next": []string{"shipped", "in_transit", "out_for_delivery", "delivered", "on_hold", "cancelled", "failed"}},
	{"status": "on_hold", "meaning": "paused", "next": []string{"processing", "confirmed", "shipped", "cancelled", "failed"}},
	{"status": "shipped", "meaning": "left the supplier", "next": []string{"in_transit", "out_for_delivery", "delivered", "completed", "returned", "failed"}},
	{"status": "in_transit", "meaning": "on the way", "next": []string{"out_for_delivery", "delivered", "completed", "returned", "failed"}},
	{"status": "out_for_delivery", "meaning": "with the courier", "next": []string{"delivered", "completed", "returned", "failed"}},
	{"status": "delivered", "meaning": "received by the buyer", "next": []string{"shipped", "in_transit", "out_for_delivery", "completed", "returned", "refunded"}},
	{"status": "completed", "meaning": "closed", "next": []string{"shipped", "in_transit", "out_for_delivery", "refunded"}},
	{"status": "cancelled", "meaning": "cancelled; final", "next": []string{}},
	{"status": "failed", "meaning": "delivery failed; final", "next": []string{}},
	{"status": "returned", "meaning": "returned; final", "next": []string{}},
	{"status": "refunded", "meaning": "refunded; final", "next": []string{}},
}

func guideStatuses() Result {
	return Result{Data: map[string]any{
		"statuses":     orderStatusGuide,
		"buyer_cancel": "A buyer can cancel while the order is pending, processing, confirmed or on_hold and no shipment has left.",
	}, Rows: len(orderStatusGuide)}
}

// OrderStatusGuide exposes the lifecycle copy as status → next statuses, for
// the test that holds it to commerce.
func OrderStatusGuide() map[string][]string {
	out := make(map[string][]string, len(orderStatusGuide))
	for _, s := range orderStatusGuide {
		out[s["status"].(string)] = s["next"].([]string)
	}
	return out
}
