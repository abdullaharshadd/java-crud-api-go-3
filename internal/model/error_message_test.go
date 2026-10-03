package model

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Renamed from sp to avoid clashing with sp in user_test.go.
func emSP(s string) *string       { return &s }
func hp(h HTTPStatus) *HTTPStatus { return &h }

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

func TestNewErrorMessageAndAccessors(t *testing.T) {
	e := NewErrorMessage(StatusNotFound, "User are not available")
	if e.GetStatus() == nil || *e.GetStatus() != StatusNotFound {
		t.Fatalf("status = %v", e.GetStatus())
	}
	if e.GetMessage() == nil || *e.GetMessage() != "User are not available" {
		t.Fatalf("message = %v", e.GetMessage())
	}
	e.SetStatus(hp(StatusBadRequest))
	e.SetMessage(emSP("bad"))
	if *e.GetStatus() != StatusBadRequest || *e.GetMessage() != "bad" {
		t.Fatalf("setters failed: %v", e)
	}
	e.SetStatus(nil)
	e.SetMessage(nil)
	if e.GetStatus() != nil || e.GetMessage() != nil {
		t.Fatalf("expected unset fields, got %v", e)
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
		{http.StatusTeapot, "I_AM_A_TEAPOT", true},
		{http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", true},
		{http.StatusRequestURITooLong, "URI_TOO_LONG", true},
		{http.StatusNonAuthoritativeInfo, "NON_AUTHORITATIVE_INFORMATION", true},
		{http.StatusOK, "OK", true},
		{999, "", false},
	}
	for _, tc := range tests {
		got, ok := HTTPStatusFromCode(tc.code)
		if got != tc.want || ok != tc.ok {
			t.Errorf("HTTPStatusFromCode(%d) = (%q,%v), want (%q,%v)", tc.code, got, ok, tc.want, tc.ok)
		}
		if ok && strings.ContainsAny(string(got), " -'") {
			t.Errorf("HTTPStatusFromCode(%d) = %q contains invalid chars", tc.code, got)
		}
	}
}

func TestErrorMessageEqual(t *testing.T) {
	a := NewErrorMessage(StatusNotFound, "x")
	b := NewErrorMessage(StatusNotFound, "x")
	if !a.Equal(b) {
		t.Error("expected equal")
	}
	if a.Equal(NewErrorMessage(StatusBadRequest, "x")) {
		t.Error("different status must not be equal")
	}
	if a.Equal(NewErrorMessage(StatusNotFound, "y")) {
		t.Error("different message must not be equal")
	}
	if !NewEmptyErrorMessage().Equal(NewEmptyErrorMessage()) {
		t.Error("empty messages must be equal")
	}
	if a.Equal(NewEmptyErrorMessage()) {
		t.Error("set vs unset must not be equal")
	}
	var n *ErrorMessage
	if !n.Equal(nil) || a.Equal(nil) {
		t.Error("nil handling wrong")
	}
}

func TestErrorMessageString(t *testing.T) {
	if got := NewErrorMessage(StatusNotFound, "User are not available").String(); got != "ErrorMessage(status=NOT_FOUND, message=User are not available)" {
		t.Errorf("String() = %q", got)
	}
	if got := NewEmptyErrorMessage().String(); got != "ErrorMessage(status=null, message=null)" {
		t.Errorf("String() = %q", got)
	}
	var n *ErrorMessage
	if n.String() != "null" {
		t.Errorf("nil String() = %q", n.String())
	}
}

func TestErrorMessageJSON(t *testing.T) {
	b, err := json.Marshal(NewErrorMessage(StatusNotFound, "User are not available"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"status":"NOT_FOUND","message":"User are not available"}` {
		t.Errorf("json = %s", b)
	}
	b, err = json.Marshal(NewEmptyErrorMessage())
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"status":null,"message":null}` {
		t.Errorf("json = %s", b)
	}
	var back ErrorMessage
	if err := json.Unmarshal([]byte(`{"status":"BAD_REQUEST","message":"m"}`), &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(&ErrorMessage{Status: hp(StatusBadRequest), Message: emSP("m")}) {
		t.Errorf("decoded %v", &back)
	}
}
