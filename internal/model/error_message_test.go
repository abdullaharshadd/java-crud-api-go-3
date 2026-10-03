package model

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Renamed from sp to avoid clashing with sp in user_test.go.
// Change every sp( call in this file to emSP(.
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

// Rest of the file (not shown in the request): rename these declarations,
// keeping their bodies the same:
//   func TestEqual(t *testing.T)  -> func TestErrorMessageEqual(t *testing.T)   (was line 100)
//   func TestString(t *testing.T) -> func TestErrorMessageString(t *testing.T)  (was line 144)
//   func TestJSON(t *testing.T)   -> func TestErrorMessageJSON(t *testing.T)    (was line 172)
