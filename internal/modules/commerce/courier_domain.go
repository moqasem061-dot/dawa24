package commerce

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The delivery representative's side of a shipment.
//
// A parcel that has left the warehouse belongs to one person until a pharmacy
// signs for it. These are the rules that make that ownership legible: who is
// carrying it, whether it is still work or already history, and how long it
// has been waiting — which is the whole of "what should I deliver first".

// IsAssignedTo reports whether this parcel is on the given courier's round.
func (s *OrderShipment) IsAssignedTo(userID int64) bool {
	return s != nil && s.CourierUserID != nil && *s.CourierUserID == userID
}

// IsClosed reports whether the parcel has reached a state no courier action
// can move it out of. The portal renders these as history rather than work.
func (s *OrderShipment) IsClosed() bool {
	if s == nil {
		return false
	}
	switch s.Status {
	case StatusDelivered, StatusCompleted, StatusCancelled, StatusReturned, StatusRefunded, StatusFailed:
		return true
	}
	return false
}

// WaitingHours is how long the parcel has sat on a courier's round, in whole
// hours. It is what turns an assignment date into a priority the courier can
// read at a glance; an unassigned parcel has waited no time at all.
func (s *OrderShipment) WaitingHours() int {
	if s == nil || s.CourierAssignedAt == nil {
		return 0
	}
	h := int(time.Since(*s.CourierAssignedAt).Hours())
	if h < 0 {
		return 0
	}
	return h
}

// CourierOpenStatuses are the shipment states a courier can still act on.
//
// A parcel is on a round from the moment it is confirmed until it is signed
// for. The list is written out rather than derived from IsClosed because it is
// also a SQL predicate, and the database cannot call a Go method.
func CourierOpenStatuses() []string {
	return []string{
		string(StatusPending), string(StatusProcessing), string(StatusConfirmed),
		string(StatusOnHold), string(StatusShipped), string(StatusInTransit),
		string(StatusOutForDelivery),
	}
}

// CourierWorkload summarises one delivery representative's round for the
// dispatcher: how much is still open, how much they have closed, and how long
// the oldest open parcel has been waiting.
type CourierWorkload struct {
	UserID           int64      `json:"user_id"`
	Name             string     `json:"name"`
	Phone            string     `json:"phone,omitempty"`
	OpenCount        int        `json:"open_count"`
	DeliveredCount   int        `json:"delivered_count"`
	OldestAssignedAt *time.Time `json:"oldest_assigned_at,omitempty"`
}

// CourierQueue names one view of the dispatch board.
//
// The portal shows the same parcels to two people who need opposite things
// from them. A مندوب wants "what am I carrying" and "what did I close". A
// dispatcher wants "what has nobody picked up" and "what is out with the
// team". One enum, four predicates, so the tab a person clicks and the SQL
// that answers it cannot drift apart.
type CourierQueue string

const (
	// CourierQueueMine is the caller's own open round, oldest assignment first.
	CourierQueueMine CourierQueue = "mine"
	// CourierQueueCompleted is what the caller has already closed.
	CourierQueueCompleted CourierQueue = "completed"
	// CourierQueueFailed is what the caller attempted but failed or returned.
	CourierQueueFailed CourierQueue = "failed"
	// CourierQueueUnassigned is every open parcel nobody is carrying.
	CourierQueueUnassigned CourierQueue = "unassigned"
	// CourierQueueAll is every open parcel in the company, assigned or not.
	CourierQueueAll CourierQueue = "all"
)

// ParseCourierQueue folds a query-string value onto a queue, defaulting to the
// caller's own round. An unknown value is not an error: it is a stale
// bookmark, and the right answer to that is the default tab.
func ParseCourierQueue(v string) CourierQueue {
	switch CourierQueue(v) {
	case CourierQueueCompleted:
		return CourierQueueCompleted
	case CourierQueueFailed:
		return CourierQueueFailed
	case CourierQueueUnassigned:
		return CourierQueueUnassigned
	case CourierQueueAll:
		return CourierQueueAll
	}
	return CourierQueueMine
}

// IsDispatch reports whether the queue shows other people's work, and so
// requires vendor.delivery.assign rather than only vendor.delivery.view.
func (q CourierQueue) IsDispatch() bool {
	return q == CourierQueueUnassigned || q == CourierQueueAll
}

// CourierQueueFilter addresses one page of the dispatch board.
type CourierQueueFilter struct {
	// VendorOrgID scopes every queue to one supplier. It is never optional.
	VendorOrgID int64
	// CourierUserID is whose round to read, for the personal queues. It is
	// ignored by the dispatch queues, which are company-wide by definition.
	CourierUserID int64
	Queue         CourierQueue
	// Search matches the shipment number, the waybill, the order number or the
	// receiving pharmacy's name.
	Search string
	Limit  int
	Offset int
}

// CourierQueueCounts is the badge on each tab.
type CourierQueueCounts struct {
	Mine       int `json:"mine"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Unassigned int `json:"unassigned"`
	All        int `json:"all"`
	// Overdue counts the caller's own open parcels assigned more than
	// overdueAfterHours ago — the ones a supervisor would ask about.
	Overdue int `json:"overdue"`
}

// CourierOverdueHours is when a parcel still on a round stops being in
// progress and starts being late. A day is the working assumption for
// same-city pharmaceutical delivery in this market; it is a display rule, not
// a business rule, and nothing is refused because of it.
const CourierOverdueHours = 24

// CollectionKind names what the representative has to collect at the door.
type CollectionKind string

const (
	// CollectNothing — the order was paid electronically in full.
	CollectNothing CollectionKind = "none"
	// CollectShippingOnly — the goods were paid from the pharmacy's wallet, so
	// only the delivery fee is due in cash.
	CollectShippingOnly CollectionKind = "shipping_only"
	// CollectFull — cash on delivery: the whole invoice is due.
	CollectFull CollectionKind = "full"
)

// CourierCollection is the cash owed at handover.
type CourierCollection struct {
	Kind   CollectionKind
	Amount money.Amount
}

// CourierCollection computes what the representative must take from the
// pharmacy.
//
// It is the single statement of a rule that decides both what the courier is
// told to collect and what the ledger records as collected. Those were two
// copies — one in the delivery template, one in the completion transaction —
// and a wallet order with free shipping showed the courier "collect nothing"
// while the ledger wrote the full invoice. One method, called by both.
func (s *OrderShipment) CourierCollection() CourierCollection {
	if s == nil {
		return CourierCollection{Kind: CollectNothing, Amount: money.Zero}
	}
	switch {
	case s.PaymentMethod == "wallet":
		// The goods are already paid. Anything still owed is the delivery fee,
		// and a supplier that delivers free owes nothing at all.
		if s.ShippingFee.IsPositive() {
			return CourierCollection{Kind: CollectShippingOnly, Amount: s.ShippingFee}
		}
		return CourierCollection{Kind: CollectNothing, Amount: money.Zero}
	case s.PaymentMethod == "cod" || s.PaymentStatus != PaymentPaid:
		return CourierCollection{Kind: CollectFull, Amount: s.TotalAmount}
	}
	return CourierCollection{Kind: CollectNothing, Amount: money.Zero}
}
