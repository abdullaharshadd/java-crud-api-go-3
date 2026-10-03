package model

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func sp(s string) *string           { return &s }
func hp(h HTTPStatus) *HTTPStatus   { return &h }

func TestNewEmptyErrorMessage(t *testing.T) {
	a := NewEmptyErrorMessage()
	if a == nil {
		t.Fatal("nil instance")
	}
	if a.GetStatus() != nil || a.GetMessage() != nil {
		t.Fatalf("expected unset fields, got %v", a)
	}
	b := NewEmptyErrorMessage()
	if a == b {
		t.Fatal("expected distinct instances")
	}
}

func TestNewErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		status  HTTPStatus
		message string
	}{
		{"not found", StatusNotFound, "Contact not found"},
		{"empty", "", ""},
		{"bad request", StatusBadRequest, "  raw  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewErrorMessage(tc.status, tc.message)
			if e.GetStatus() == nil || *e.GetStatus() != tc.status {
				t.Errorf("status = %v, want %v", e.GetStatus(), tc.status)
			}
			if e.GetMessage() == nil || *e.GetMessage() != tc.message {
				t.Errorf("message = %v, want %q", e.GetMessage(), tc.message)
			}
		})
	}
}

func TestNullFieldsViaLiteral(t *testing.T) {
	e := &ErrorMessage{Status: nil, Message: nil}
	if e.GetStatus() != nil || e.GetMessage() != nil {
		t.Fatal("expected nil fields")
	}
}

func TestStatusAccessors(t *testing.T) {
	tests := []struct {
		name string
		in   *HTTPStatus
	}{
		{"set value", hp(StatusInternalServerError)},
		{"set nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewErrorMessage(StatusNotFound, "x")
			e.SetStatus(tc.in)
			got := e.GetStatus()
			if !statusPtrEqual(got, tc.in) {
				t.Errorf("got %v want %v", got, tc.in)
			}
			if e.GetMessage() == nil || *e.GetMessage() != "x" {
				t.Error("message changed unexpectedly")
			}
		})
	}
}

func TestMessageAccessors(t *testing.T) {
	tests := []struct {
		name string
		in   *string
	}{
		{"set value", sp("Invalid input")},
		{"set empty", sp("")},
		{"set nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewErrorMessage(StatusBadRequest, "orig")
			e.SetMessage(tc.in)
			if !strPtrEqual(e.GetMessage(), tc.in) {
				t.Errorf("got %v want %v", e.GetMessage(), tc.in)
			}
		})
	}
}

func TestEqual(t *testing.T) {
	var nilEM *ErrorMessage
	tests := []struct {
		name string
		a, b *ErrorMessage
		want bool
	}{
		{"same values", NewErrorMessage(StatusNotFound, "m"), NewErrorMessage(StatusNotFound, "m"), true},
		{"diff status", NewErrorMessage(StatusNotFound, "m"), NewErrorMessage(StatusBadRequest, "m"), false},
		{"diff message", NewErrorMessage(StatusNotFound, "m"), NewErrorMessage(StatusNotFound, "n"), false},
		{"both empty", NewEmptyErrorMessage(), NewEmptyErrorMessage(), true},
		{"one nil status", &ErrorMessage{Message: sp("m")}, NewErrorMessage(StatusNotFound, "m"), false},
		{"one nil message", &ErrorMessage{Status: hp(StatusNotFound)}, NewErrorMessage(StatusNotFound, "m"), false},
		{"other nil", NewEmptyErrorMessage(), nil, false},
		{"receiver nil", nilEM, NewEmptyErrorMessage(), false},
		{"both nil", nilEM, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("a.Equal(b) = %v want %v", got, tc.want)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("symmetry: b.Equal(a) = %v want %v", got, tc.want)
			}
		})
	}
}

func TestEqualReflexiveTransitive(t *testing.T) {
	a := NewErrorMessage(StatusNotFound, "m")
	b := NewErrorMessage(StatusNotFound, "m")
	c := NewErrorMessage(StatusNotFound, "m")
	if !a.Equal(a) {
		t.Error("not reflexive")
	}
	if a.Equal(b) && b.Equal(c) && !a.Equal(c) {
		t.Error("not transitive")
	}
	if a.String() != b.String() {
		t.Error("equal objects should render equally")
	}
}

func TestString(t *testing.T) {
	var nilEM *ErrorMessage
	tests := []struct {
		name string
		e    *ErrorMessage
		want string
	}{
		{"full", NewErrorMessage(StatusBadRequest, "oops"), "ErrorMessage(status=BAD_REQUEST, message=oops)"},
		{"empty", NewEmptyErrorMessage(), "ErrorMessage(status=null, message=null)"},
		{"not found", NewErrorMessage(StatusNotFound, "User are not available"), "ErrorMessage(status=NOT_FOUND, message=User are not available)"},
		{"nil receiver", nilEM, "null"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.e.String()
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
	s := NewErrorMessage(StatusBadRequest, "oops").String()
	for _, part := range []string{"ErrorMessage", "BAD_REQUEST", "oops"} {
		if !strings.Contains(s, part) {
			t.Errorf("missing %q in %q", part, s)
		}
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name string
		e    *ErrorMessage
		want string
	}{
		{"full", NewErrorMessage(StatusNotFound, "User are not available"), `{"status":"NOT_FOUND","message":"User are not available"}`},
		{"empty", NewEmptyErrorMessage(), `{"status":null,"message":null}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.e)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Errorf("got %s want %s", b, tc.want)
			}
			var back ErrorMessage
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			if !back.Equal(tc.e) {
				t.Errorf("round trip mismatch: %v vs %v", &back, tc.e)
			}
		})
	}
}

func TestHTTPStatusFromCode(t *testing.T) {
	tests := []struct {
		code int
		want HTTPStatus
		ok   bool
	}{
		{http.StatusNotFound, StatusNotFound, true},
		{http.StatusBadRequest, StatusBadRequest, true},
		{http.StatusInternalServerError, StatusInternalServerError, true},
		{http.StatusOK, "OK", true},
		{http.StatusTeapot, "I_AM_A_TEAPOT", true},
		{http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", true},
		{http.StatusRequestURITooLong, "URI_TOO_LONG", true},
		{http.StatusRequestedRangeNotSatisfiable, "REQUESTED_RANGE_NOT_SATISFIABLE", true},
		{http.StatusHTTPVersionNotSupported, "HTTP_VERSION_NOT_SUPPORTED", true},
		{http.StatusNonAuthoritativeInfo, "NON_AUTHORITATIVE_INFORMATION", true},
		{http.StatusMultiStatus, "MULTI_STATUS", true},
		{999, "", false},
		{0, "", false},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			got, ok := HTTPStatusFromCode(tc.code)
			if got != tc.want || ok != tc.ok {
				t.Errorf("code %d: got (%q,%v) want (%q,%v)", tc.code, got, ok, tc.want, tc.ok)
			}
		})
	}
}