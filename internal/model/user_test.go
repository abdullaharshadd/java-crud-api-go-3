package model

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func sp(s string) *string { return &s }

func TestNewUser(t *testing.T) {
	u := NewUser()
	if u == nil {
		t.Fatal("nil user")
	}
	if u.GetID() != 0 || u.GetName() != nil || u.GetEmail() != nil || u.GetPassword() != nil || u.GetRole() != nil || u.GetAbout() != nil {
		t.Fatalf("expected empty user, got %v", u)
	}
}

func TestNewUserWithAll(t *testing.T) {
	tests := []struct {
		name                              string
		id                                int
		n, email, password, role, about *string
	}{
		{"all set", 7, sp("bob"), sp("b@x.com"), sp("pw"), sp("ADMIN"), sp("hi")},
		{"nil values", 0, nil, nil, nil, nil, nil},
		{"blank values", 1, sp(""), sp(" "), sp(""), sp("\t"), sp("")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := NewUserWithAll(tc.id, tc.n, tc.email, tc.password, tc.role, tc.about)
			if u.GetID() != tc.id || u.GetName() != tc.n || u.GetEmail() != tc.email ||
				u.GetPassword() != tc.password || u.GetRole() != tc.role || u.GetAbout() != tc.about {
				t.Fatalf("fields mismatch: %v", u)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	tests := []struct {
		name  string
		build func() *User
		want  *User
	}{
		{"empty", func() *User { return NewUserBuilder().Build() }, NewUser()},
		{"partial", func() *User { return NewUserBuilder().Name("a").Email("e").Build() },
			&User{Name: sp("a"), Email: sp("e")}},
		{"full", func() *User {
			return NewUserBuilder().ID(3).Name("a").Email("e").Password("p").Role("r").About("ab").Build()
		}, NewUserWithAll(3, sp("a"), sp("e"), sp("p"), sp("r"), sp("ab"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build()
			if !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuilderNewInstance(t *testing.T) {
	b := NewUserBuilder().Name("x")
	u1 := b.Build()
	u2 := b.Build()
	if u1 == u2 {
		t.Fatal("expected distinct instances")
	}
	u1.SetID(99)
	if u2.GetID() != 0 {
		t.Fatal("instances share state")
	}
	b.Name("y")
	if *u1.GetName() != "x" {
		t.Fatal("builder mutation leaked into built user")
	}
}

func TestGettersSetters(t *testing.T) {
	type acc struct {
		set func(*User, *string)
		get func(*User) *string
	}
	fields := map[string]acc{
		"name":     {(*User).SetName, (*User).GetName},
		"email":    {(*User).SetEmail, (*User).GetEmail},
		"password": {(*User).SetPassword, (*User).GetPassword},
		"role":     {(*User).SetRole, (*User).GetRole},
		"about":    {(*User).SetAbout, (*User).GetAbout},
	}
	for fname, a := range fields {
		for _, v := range []*string{sp("val"), sp(""), sp("  "), nil} {
			t.Run(fname, func(t *testing.T) {
				u := NewUserWithAll(5, sp("n"), sp("e"), sp("p"), sp("r"), sp("a"))
				orig := *u
				a.set(u, v)
				if a.get(u) != v {
					t.Fatalf("getter did not return set value")
				}
				// only target field mutated
				for other, oa := range fields {
					if other == fname {
						continue
					}
					if oa.get(u) != oa.get(&orig) {
						t.Fatalf("field %s mutated", other)
					}
				}
				if u.GetID() != 5 {
					t.Fatal("id mutated")
				}
			})
		}
	}
	u := NewUser()
	for _, id := range []int{0, 1, -1, 1 << 30} {
		u.SetID(id)
		if u.GetID() != id {
			t.Fatalf("id %d", id)
		}
		if u.Name != nil {
			t.Fatal("name mutated")
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		n       *string
		wantErr bool
	}{
		{"nil", nil, true},
		{"empty", sp(""), true},
		{"spaces", sp("   "), true},
		{"tabs newlines", sp("\t\n\r "), true},
		{"valid", sp("bob"), false},
		{"padded", sp("  bob "), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := NewUserWithAll(0, tc.n, nil, nil, nil, nil)
			err := u.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrNameBlank) {
					t.Fatalf("want ErrNameBlank, got %v", err)
				}
				if err.Error() != "please Add the department Name" {
					t.Fatalf("message %q", err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected %v", err)
			}
		})
	}
	// only name is constrained
	u := NewUserBuilder().Name("x").Email("").About(strings.Repeat("a", 600)).Build()
	if err := u.Validate(); err != nil {
		t.Fatalf("other fields should not be validated: %v", err)
	}
}

func TestEqual(t *testing.T) {
	base := func() *User { return NewUserWithAll(1, sp("n"), sp("e"), sp("p"), sp("r"), sp("a")) }
	tests := []struct {
		name string
		a, b *User
		want bool
	}{
		{"identical", base(), base(), true},
		{"both empty", NewUser(), NewUser(), true},
		{"id differs", base(), func() *User { u := base(); u.SetID(2); return u }(), false},
		{"name differs", base(), func() *User { u := base(); u.SetName(sp("x")); return u }(), false},
		{"email differs", base(), func() *User { u := base(); u.SetEmail(sp("x")); return u }(), false},
		{"password differs", base(), func() *User { u := base(); u.SetPassword(sp("x")); return u }(), false},
		{"role differs", base(), func() *User { u := base(); u.SetRole(sp("x")); return u }(), false},
		{"about differs", base(), func() *User { u := base(); u.SetAbout(sp("x")); return u }(), false},
		{"nil vs set", base(), func() *User { u := base(); u.SetAbout(nil); return u }(), false},
		{"other nil", base(), nil, false},
		{"both nil", nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Fatalf("a.Equal(b)=%v", got)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Fatalf("symmetry broken")
			}
			if tc.a != nil && !tc.a.Equal(tc.a) {
				t.Fatal("not reflexive")
			}
		})
	}
	// consistent: equal users have equal string repr (hash proxy)
	if base().String() != base().String() {
		t.Fatal("inconsistent")
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		u    *User
		want string
	}{
		{"empty", NewUser(), "User(id=0, name=null, email=null, password=null, role=null, about=null)"},
		{"full", NewUserWithAll(1, sp("x"), sp("e@x"), sp("secret"), sp("ADMIN"), sp("hi")),
			"User(id=1, name=x, email=e@x, password=secret, role=ADMIN, about=hi)"},
		{"partial", NewUserBuilder().ID(1).Name("x").Build(),
			"User(id=1, name=x, email=null, password=null, role=null, about=null)"},
		{"nil", nil, "null"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	u := NewUserBuilder().ID(1).Name("x").Password("pw").Build()
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"name":"x","email":null,"password":"pw","role":null,"about":null}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	var back User
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(u) {
		t.Fatal("roundtrip mismatch")
	}
}

func TestSchemaStatements(t *testing.T) {
	if UserTable != "user" || UserColumnID != "user_id" || UserColumnName != "user_name" ||
		UserColumnEmail != "user_email" || UserColumnPassword != "user_password" ||
		UserColumnRole != "user_role" || UserColumnAbout != "user_about" {
		t.Fatal("column constants changed")
	}
	if len(UserSchemaStatements) != 3 {
		t.Fatalf("got %d statements", len(UserSchemaStatements))
	}
	ddl := UserSchemaStatements[2]
	for _, s := range []string{"CREATE TABLE IF NOT EXISTS `user`", "user_id INTEGER NOT NULL",
		"user_about VARCHAR(500)", "user_email VARCHAR(255)", "user_name", "user_password",
		"user_role", "PRIMARY KEY (user_id)", "UNIQUE (user_email)"} {
		if !strings.Contains(ddl, s) {
			t.Errorf("ddl missing %q", s)
		}
	}
	if !strings.Contains(UserSchemaStatements[0], "hibernate_sequence") {
		t.Error("missing hibernate_sequence")
	}
}

// ---- fake SQL driver ----

type fakeDriver struct {
	mu     sync.Mutex
	execs  []string
	failAt int // 1-based; 0 = never
	err    error
}

type fakeConn struct{ d *fakeDriver }

func (d *fakeDriver) Open(string) (driver.Conn, error) { return &fakeConn{d}, nil }
func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("no tx") }
func (c *fakeConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	c.d.execs = append(c.d.execs, q)
	if c.d.failAt == len(c.d.execs) {
		return nil, c.d.err
	}
	return driver.RowsAffected(0), nil
}

var drvSeq int
var drvMu sync.Mutex

func openFake(t *testing.T, d *fakeDriver) *sql.DB {
	drvMu.Lock()
	drvSeq++
	name := "fakeuser" + string(rune('a'+drvSeq%26)) + strings.Repeat("x", drvSeq)
	drvMu.Unlock()
	sql.Register(name, d)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestEnsureUserSchema(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		failAt    int
		wantExecs int
		wantErr   string
	}{
		{"success", 0, 3, ""},
		{"fail first", 1, 1, "statement 1"},
		{"fail third", 3, 3, "statement 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fakeDriver{failAt: tc.failAt, err: boom}
			db := openFake(t, d)
			err := EnsureUserSchema(context.Background(), db)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || !errors.Is(err, boom) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("unexpected err %v", err)
				}
			}
			if len(d.execs) != tc.wantExecs {
				t.Fatalf("execs %d", len(d.execs))
			}
			for i, q := range d.execs {
				if q != UserSchemaStatements[i] {
					t.Fatalf("stmt %d mismatch", i)
				}
			}
		})
	}
}