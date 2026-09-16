package handles_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

// A handle is the only identifier the assistant ever gives a model, so these
// tests are the ones that matter: they say that nothing the model produces can
// be turned into a row it was not already shown.

const secret = "test-secret-value-at-least-32-characters-long"

func TestRoundTrip(t *testing.T) {
	s := handles.NewSigner(secret)
	owner := handles.Binding{OrgID: 7, UserID: 42}

	token := s.Issue(handles.KindOrder, 1234, owner)
	got, err := s.Resolve(token, handles.KindOrder, owner)
	if err != nil {
		t.Fatalf("resolve own handle: %v", err)
	}
	if got != 1234 {
		t.Fatalf("id = %d, want 1234", got)
	}
}

// The row id must not be readable from the token by inspection. It is not
// encrypted — the binding is what protects it — but a handle that spelled out
// "order 1234" would invite exactly the enumeration this design removes.
func TestHandleDoesNotLeakIDInPlainText(t *testing.T) {
	s := handles.NewSigner(secret)
	token := s.Issue(handles.KindOrder, 987654, handles.Binding{OrgID: 1, UserID: 1})
	if strings.Contains(token, "987654") {
		t.Fatalf("handle contains the raw id: %s", token)
	}
}

// Consecutive ids must not produce guessable neighbours.
func TestSequentialIDsAreNotGuessable(t *testing.T) {
	s := handles.NewSigner(secret)
	b := handles.Binding{OrgID: 3, UserID: 9}
	a := s.Issue(handles.KindOrder, 1000, b)
	c := s.Issue(handles.KindOrder, 1001, b)

	// The payloads differ by one byte, but the MACs must be unrelated.
	macA := a[strings.LastIndex(a, ".")+1:]
	macC := c[strings.LastIndex(c, ".")+1:]
	if macA == macC {
		t.Fatal("neighbouring ids produced the same signature")
	}
	common := 0
	for i := 0; i < len(macA) && i < len(macC); i++ {
		if macA[i] == macC[i] {
			common++
		}
	}
	if common > len(macA)/2 {
		t.Fatalf("signatures for neighbouring ids are too similar (%d/%d chars)", common, len(macA))
	}
}

// The heart of it: a perfectly valid handle issued to another tenant must fail
// for this one. Signature checks alone would pass it.
func TestForeignBindingIsRefused(t *testing.T) {
	s := handles.NewSigner(secret)
	issued := s.Issue(handles.KindOrder, 55, handles.Binding{OrgID: 1, UserID: 10})

	cases := []struct {
		name string
		b    handles.Binding
	}{
		{"other organization", handles.Binding{OrgID: 2, UserID: 10}},
		{"other user, same org", handles.Binding{OrgID: 1, UserID: 11}},
		{"no tenant", handles.Binding{OrgID: 0, UserID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Resolve(issued, handles.KindOrder, tc.b); err != handles.ErrBinding {
				t.Fatalf("err = %v, want ErrBinding", err)
			}
		})
	}
}

// A handle for one kind of row must not be usable where another is expected,
// so an order reference cannot be replayed as a branch reference.
func TestKindIsEnforced(t *testing.T) {
	s := handles.NewSigner(secret)
	b := handles.Binding{OrgID: 1, UserID: 1}
	token := s.Issue(handles.KindOrder, 5, b)

	if _, err := s.Resolve(token, handles.KindBranch, b); err == nil {
		t.Fatal("an order handle resolved as a branch handle")
	}
}

func TestTamperedTokenIsRefused(t *testing.T) {
	s := handles.NewSigner(secret)
	b := handles.Binding{OrgID: 1, UserID: 1}
	token := s.Issue(handles.KindOrder, 5, b)

	dot := strings.LastIndex(token, ".")
	payload, mac := token[:dot], token[dot+1:]

	cases := map[string]string{
		"flipped payload byte":   flip(payload) + "." + mac,
		"flipped signature byte": payload + "." + flip(mac),
		"no signature":           payload,
		"empty":                  "",
		"not a handle":           "1234",
		"plausible-looking":      "hord_AAAA.BBBB",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Resolve(bad, handles.KindOrder, b); err == nil {
				t.Fatalf("accepted a %s handle", name)
			}
		})
	}
}

// A different deployment secret must not validate another's handles.
func TestSignersDoNotInterchange(t *testing.T) {
	a := handles.NewSigner(secret)
	other := handles.NewSigner("a-completely-different-secret-value-here")
	b := handles.Binding{OrgID: 1, UserID: 1}

	token := a.Issue(handles.KindOrder, 5, b)
	if _, err := other.Resolve(token, handles.KindOrder, b); err != handles.ErrSignature {
		t.Fatalf("err = %v, want ErrSignature", err)
	}
}

// An empty application secret must still produce unforgeable handles, because
// development environments have one and a predictable MAC there would become a
// production habit.
func TestEmptySecretStillSigns(t *testing.T) {
	a := handles.NewSigner("")
	b := handles.NewSigner("")
	binding := handles.Binding{OrgID: 1, UserID: 1}

	token := a.Issue(handles.KindOrder, 5, binding)
	if _, err := a.Resolve(token, handles.KindOrder, binding); err != nil {
		t.Fatalf("signer cannot read its own handle: %v", err)
	}
	if _, err := b.Resolve(token, handles.KindOrder, binding); err == nil {
		t.Fatal("two unkeyed signers agreed; the key is not random")
	}
}

func flip(s string) string {
	if s == "" {
		return "x"
	}
	b := []byte(s)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}
