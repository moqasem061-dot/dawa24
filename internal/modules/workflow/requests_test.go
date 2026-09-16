package workflow

import "testing"

func TestRequestValidate(t *testing.T) {
	cases := []struct {
		name     string
		r        *Request
		wantErr  bool
		wantType RequestType
	}{
		{"valid with type", &Request{FromOrgID: 1, ToOrgID: 2, Type: RequestDocument}, false, RequestDocument},
		{"missing from org", &Request{ToOrgID: 2}, true, ""},
		{"missing to org", &Request{FromOrgID: 1}, true, ""},
		{"same org both sides", &Request{FromOrgID: 1, ToOrgID: 1}, true, ""},
		{"empty type defaults to document", &Request{FromOrgID: 1, ToOrgID: 2}, false, RequestDocument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.r.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.wantType != "" && tc.r.Type != tc.wantType {
				t.Fatalf("Type = %q, want %q", tc.r.Type, tc.wantType)
			}
		})
	}
}

func TestRequestStatusesAreClosedSet(t *testing.T) {
	// The statuses accepted by RespondRequest must be a closed set.
	if RequestAccepted != "accepted" || RequestDeclined != "declined" || RequestCancelled != "cancelled" {
		t.Fatalf("unexpected request status constants")
	}
}
