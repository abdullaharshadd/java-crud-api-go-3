package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

func TestNewUser(t *testing.T) {
	u := NewUser()
	if u.GetID() != 0 || u.GetName() != nil || u.GetEmail() != nil || u.GetPassword() != nil || u.GetRole() != nil || u.GetAbout() != nil {
		t.Fatalf("expected zero user, got %v", u)
	}
}

func TestUserSettersAndBuilder(t *testing.T) {
	u := NewUser()
	u.SetID(3)
	u.SetName(sp("hemraj"))
	u.SetEmail(sp("e"))
	u.SetPassword(sp("p"))
	u.SetRole(sp("r"))
	u.SetAbout(sp("a"))
	b := NewUserBuilder().ID(3).Name("hemraj").Email("e").Password("p").Role("r").About("a").Build()
	all := NewUserWithAll(3, sp("hemraj"), sp("e"), sp("p"), sp("r"), sp("a"))
	if !u.Equal(b) || !b.Equal(all) {
		t.Fatalf("mismatch: %v / %v / %v", u, b, all)
	}
	if u.Equal(NewUser()) {
		t.Fatal("must differ from empty user")
	}
}

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		ok   bool
	}{
		{"nil", nil, false},
		{"empty", sp(""), false},
		{"blank", sp(" \t\n"), false},
		{"valid", sp("x"), true},
	}
	for _, tc := range tests {
		err := (&User{Name: tc.in}).Validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
		if err != nil && (!errors.Is(err, ErrNameBlank) || err.Error() != MsgNameBlank) {
			t.Errorf("%s: wrong error %v", tc.name, err)
		}
	}
}

func TestUserString(t *testing.T) {
	u := NewUserBuilder().ID(1).Name("x").Build()
	if got := u.String(); got != "User(id=1, name=x, email=null, password=null, role=null, about=null)" {
		t.Errorf("String() = %q", got)
	}
	var n *User
	if n.String() != "null" {
		t.Error("nil String")
	}
}

func TestUserJSON(t *testing.T) {
	b, err := json.Marshal(NewUserBuilder().ID(1).Name("x").Password("p").Build())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"name":"x","email":null,"password":"p","role":null,"about":null}`
	if string(b) != want {
		t.Errorf("json = %s", b)
	}
}

func TestUserSchemaStatements(t *testing.T) {
	if len(UserSchemaStatements) != 3 {
		t.Fatalf("got %d statements", len(UserSchemaStatements))
	}
	if !strings.Contains(UserSchemaStatements[2], "CREATE TABLE IF NOT EXISTS `user`") ||
		!strings.Contains(UserSchemaStatements[2], "uk_user_email UNIQUE (user_email)") {
		t.Errorf("unexpected DDL: %s", UserSchemaStatements[2])
	}
}
