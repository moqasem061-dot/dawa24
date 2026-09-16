// Package handles issues and verifies the opaque identifiers the assistant
// gives a model in place of database ids.
//
// The problem it solves is structural rather than incidental. A model can be
// argued into anything: a document it reads, a product name a supplier chose,
// or a user simply asking nicely can all produce a tool call carrying an id
// that belongs to somebody else. Validating each such id inside each tool works
// exactly until the day somebody adds a tool and forgets.
//
// So the assistant never shows a model a raw id. Every referenceable row is
// rendered as a handle: a signed token carrying the row's kind and id together
// with the organisation and user it was issued to, and an expiry. Verification
// checks the signature, the kind, the expiry, AND that the binding matches the
// caller live at the moment of use.
//
// The consequences are the point:
//
//   - An invented handle fails at the signature. There is nothing to guess.
//   - A handle lifted from another tenant's session fails at the binding, even
//     though its signature is perfectly valid.
//   - Enumeration is meaningless: consecutive ids do not produce consecutive
//     handles, and a handle for a row the caller may not see cannot be minted
//     by anyone but the server that already checked they may see it.
//
// Handles are deliberately short-lived. They exist to let one conversation
// refer back to a row it was just shown, not to be a durable identifier.
package handles

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind names what a handle points at. It is part of the signed payload, so a
// handle for an order cannot be replayed where a branch is expected.
type Kind string

const (
	KindOrder      Kind = "ord"
	KindShipment   Kind = "shp"
	KindProduct    Kind = "prd"
	KindBranch     Kind = "brn"
	KindOffer      Kind = "ofr"
	KindInvoice    Kind = "inv"
	KindVariant    Kind = "var"
	KindWarehouse  Kind = "whs"
	KindTransfer   Kind = "xfr"
	KindImportRun  Kind = "imp"
	KindSmartRun   Kind = "run"
	KindRequest    Kind = "req"
	KindUser       Kind = "usr"
	KindPayment    Kind = "pay"
	KindReview     Kind = "rev"
	KindAttachment Kind = "att"
	KindOrgUnit    Kind = "org"
	KindTurn       Kind = "trn"
	KindConv       Kind = "cnv"
)

// TTL is how long a handle stays usable. Half an hour is long enough for a
// conversation to refer back to something it was shown and short enough that a
// handle copied out of a log is worthless by the time anybody reads it.
const TTL = 30 * time.Minute

// Errors verification can return. Callers map all of them to one refusal
// message: telling a model which check failed teaches it what to try next.
var (
	ErrMalformed = errors.New("assistant: malformed handle")
	ErrSignature = errors.New("assistant: handle signature mismatch")
	ErrExpired   = errors.New("assistant: handle expired")
	ErrKind      = errors.New("assistant: handle kind mismatch")
	ErrBinding   = errors.New("assistant: handle belongs to another caller")
)

// Binding is the caller a handle was issued to.
type Binding struct {
	OrgID  int64
	UserID int64
}

// Signer mints and verifies handles.
type Signer struct {
	key []byte
	now func() time.Time
}

// NewSigner derives a signing key from the application secret.
//
// An empty secret yields a random per-process key rather than an unkeyed MAC.
// That degrades gracefully in development — handles simply stop working across
// a restart, which nothing depends on — and it fails closed rather than
// producing forgeable tokens on a misconfigured deployment.
func NewSigner(appSecret string) *Signer {
	var key []byte
	if strings.TrimSpace(appSecret) == "" {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			// A machine that cannot produce randomness cannot be trusted to
			// hold secrets. Panic here is better than silently issuing
			// predictable handles.
			panic(fmt.Sprintf("assistant: cannot seed handle signer: %v", err))
		}
	} else {
		sum := sha256.Sum256([]byte("dawa24.capsule.handle.v1|" + appSecret))
		key = sum[:]
	}
	return &Signer{key: key, now: time.Now}
}

// Issue mints a handle for one row, bound to one caller.
func (s *Signer) Issue(kind Kind, id int64, b Binding) string {
	return s.issueAt(kind, id, b, s.now().Add(TTL))
}

func (s *Signer) issueAt(kind Kind, id int64, b Binding, exp time.Time) string {
	payload := encodePayload(kind, id, b, exp.Unix())
	mac := s.sign(payload)
	return "h" + string(kind) + "_" +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac)
}

// Resolve verifies a handle and returns the row id it points at.
//
// want is the kind the caller expects and b is the live caller, read from the
// request context — never from anything the model produced.
func (s *Signer) Resolve(token string, want Kind, b Binding) (int64, error) {
	prefix := "h" + string(want) + "_"
	if !strings.HasPrefix(token, prefix) {
		// Either it is not a handle at all, or it is a handle of another kind.
		// Both are refusals; distinguishing them only helps a prober.
		if !strings.HasPrefix(token, "h") || !strings.Contains(token, "_") {
			return 0, ErrMalformed
		}
		return 0, ErrKind
	}
	body := strings.TrimPrefix(token, prefix)
	dot := strings.LastIndex(body, ".")
	if dot <= 0 || dot == len(body)-1 {
		return 0, ErrMalformed
	}
	payload, err := base64.RawURLEncoding.DecodeString(body[:dot])
	if err != nil {
		return 0, ErrMalformed
	}
	mac, err := base64.RawURLEncoding.DecodeString(body[dot+1:])
	if err != nil {
		return 0, ErrMalformed
	}
	// Constant time, and before anything is read out of the payload: an
	// unverified payload is attacker-controlled bytes.
	if subtle.ConstantTimeCompare(mac, s.sign(payload)) != 1 {
		return 0, ErrSignature
	}

	kind, id, bound, exp, err := decodePayload(payload)
	if err != nil {
		return 0, err
	}
	if kind != want {
		return 0, ErrKind
	}
	if s.now().Unix() > exp {
		return 0, ErrExpired
	}
	if bound.OrgID != b.OrgID || bound.UserID != b.UserID {
		return 0, ErrBinding
	}
	return id, nil
}

// KindOf reads the kind a handle claims to be, without verifying it. It only
// chooses which verification to run: Resolve must still be called with the
// kind returned here, and it checks the signature, expiry and binding.
func KindOf(token string) (Kind, bool) {
	token = strings.TrimSpace(token)
	if len(token) < 6 || token[0] != 'h' || token[4] != '_' {
		return "", false
	}
	kind := Kind(token[1:4])
	for _, r := range kind {
		if r < 'a' || r > 'z' {
			return "", false
		}
	}
	return kind, true
}

func (s *Signer) sign(payload []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(payload)
	return m.Sum(nil)
}

// Payload layout, fixed width so there is nothing to parse and no ambiguity
// between fields: 3 bytes kind, 8 id, 8 org, 8 user, 8 expiry.
const payloadLen = 3 + 8 + 8 + 8 + 8

func encodePayload(kind Kind, id int64, b Binding, exp int64) []byte {
	out := make([]byte, payloadLen)
	copy(out[0:3], padKind(kind))
	binary.BigEndian.PutUint64(out[3:11], uint64(id))
	binary.BigEndian.PutUint64(out[11:19], uint64(b.OrgID))
	binary.BigEndian.PutUint64(out[19:27], uint64(b.UserID))
	binary.BigEndian.PutUint64(out[27:35], uint64(exp))
	return out
}

func decodePayload(p []byte) (Kind, int64, Binding, int64, error) {
	if len(p) != payloadLen {
		return "", 0, Binding{}, 0, ErrMalformed
	}
	kind := Kind(strings.TrimRight(string(p[0:3]), "\x00"))
	id := int64(binary.BigEndian.Uint64(p[3:11]))
	org := int64(binary.BigEndian.Uint64(p[11:19]))
	user := int64(binary.BigEndian.Uint64(p[19:27]))
	exp := int64(binary.BigEndian.Uint64(p[27:35]))
	return kind, id, Binding{OrgID: org, UserID: user}, exp, nil
}

func padKind(k Kind) []byte {
	out := make([]byte, 3)
	copy(out, k)
	return out
}
