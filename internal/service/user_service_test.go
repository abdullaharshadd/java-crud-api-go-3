package service

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"migrated-app/internal/model"
	"migrated-app/internal/store"
)

func usSP(s string) *string { return &s }

// usFakeStore is an in-memory UserStore with Hibernate-like merge semantics and
// optional error injection.
type usFakeStore struct {
	users   map[int]model.User
	nextID  int
	calls   map[string]int
	saveErr error
	findErr error
	delErr  error
	// uniqueEmail enforces the unique constraint on email.
	uniqueEmail bool
	lastSaved   *model.User
}

var usErrConstraint = errors.New("constraint violation")

func newUSFakeStore(users ...model.User) *usFakeStore {
	f := &usFakeStore{users: map[int]model.User{}, nextID: 1, calls: map[string]int{}, uniqueEmail: true}
	for _, u := range users {
		f.users[u.ID] = u
		if u.ID >= f.nextID {
			f.nextID = u.ID + 1
		}
	}
	return f
}

func (f *usFakeStore) Save(_ context.Context, u *model.User) (*model.User, error) {
	f.calls["Save"]++
	cp := *u
	f.lastSaved = u
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	if f.uniqueEmail && cp.Email != nil {
		for id, e := range f.users {
			if id != cp.ID && e.Email != nil && *e.Email == *cp.Email {
				return nil, usErrConstraint
			}
		}
	}
	if _, ok := f.users[cp.ID]; cp.ID == 0 || !ok {
		cp.ID = f.nextID
		f.nextID++
	}
	f.users[cp.ID] = cp
	out := cp
	return &out, nil
}

func (f *usFakeStore) FindAll(_ context.Context) ([]model.User, error) {
	f.calls["FindAll"]++
	if f.findErr != nil {
		return nil, f.findErr
	}
	if len(f.users) == 0 {
		return nil, nil // exercise nil -> empty conversion
	}
	out := make([]model.User, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *usFakeStore) FindByID(_ context.Context, id int) (*model.User, bool, error) {
	f.calls["FindByID"]++
	if f.findErr != nil {
		return nil, false, f.findErr
	}
	u, ok := f.users[id]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func (f *usFakeStore) FindByName(_ context.Context, name string) (*model.User, bool, error) {
	f.calls["FindByName"]++
	if f.findErr != nil {
		return nil, false, f.findErr
	}
	var found []model.User
	for _, u := range f.users {
		if u.Name != nil && *u.Name == name {
			found = append(found, u)
		}
	}
	switch len(found) {
	case 0:
		return nil, false, nil
	case 1:
		return &found[0], true, nil
	}
	return nil, false, store.ErrNonUniqueResult
}

func (f *usFakeStore) DeleteByID(_ context.Context, id int) error {
	f.calls["DeleteByID"]++
	if f.delErr != nil {
		return f.delErr
	}
	if _, ok := f.users[id]; !ok {
		return store.ErrNoRowsDeleted
	}
	delete(f.users, id)
	return nil
}

func (f *usFakeStore) snapshot() map[int]model.User {
	m := make(map[int]model.User, len(f.users))
	for k, v := range f.users {
		m[k] = v
	}
	return m
}

func usAlice() model.User {
	return model.User{ID: 1, Name: usSP("alice"), Email: usSP("a@x.io"), Password: usSP("p"), Role: usSP("USER"), About: usSP("hi")}
}
func usBob() model.User {
	return model.User{ID: 2, Name: usSP("bob"), Email: usSP("b@x.io"), Role: usSP("ADMIN")}
}

// usGetByName calls GetUserNameByName regardless of whether it returns
// (user, error) or (user, found, error).
func usGetByName(t *testing.T, svc UserService, ctx context.Context, name string) (*model.User, error) {
	t.Helper()
	m := reflect.ValueOf(svc).MethodByName("GetUserNameByName")
	if !m.IsValid() {
		t.Fatal("GetUserNameByName not found")
	}
	res := m.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(name)})
	u, _ := res[0].Interface().(*model.User)
	var err error
	if e := res[len(res)-1].Interface(); e != nil {
		err = e.(error)
	}
	if len(res) == 3 && !res[1].Bool() && err == nil && u != nil {
		u = nil
	}
	return u, err
}

func TestUserServiceSaveUser(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("db down")
	tests := []struct {
		name      string
		seed      []model.User
		input     *model.User
		saveErr   error
		wantErr   error
		wantID    int
		wantCount int
	}{
		{name: "new user gets generated id", seed: []model.User{usAlice()},
			input: &model.User{Name: usSP("carol"), Email: usSP("c@x.io")}, wantID: 2, wantCount: 2},
		{name: "existing id overwrites", seed: []model.User{usAlice()},
			input: &model.User{ID: 1, Name: usSP("alice2"), Email: usSP("a2@x.io")}, wantID: 1, wantCount: 1},
		{name: "duplicate unique email", seed: []model.User{usAlice()},
			input: &model.User{Name: usSP("dup"), Email: usSP("a@x.io")}, wantErr: usErrConstraint, wantCount: 1},
		{name: "store error wrapped", seed: []model.User{usAlice()},
			input: &model.User{Name: usSP("x")}, saveErr: dbErr, wantErr: dbErr, wantCount: 1},
		{name: "nil user", seed: []model.User{usAlice()}, input: nil, wantErr: errNilUser, wantCount: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(tc.seed...)
			fs.saveErr = tc.saveErr
			svc := NewUserService(fs)
			got, err := svc.SaveUser(ctx, tc.input)
			if len(fs.users) != tc.wantCount {
				t.Fatalf("count = %d, want %d", len(fs.users), tc.wantCount)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("expected nil user, got %v", got)
				}
				if tc.input == nil && fs.calls["Save"] != 0 {
					t.Fatal("store should not be called for nil user")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("id = %d, want %d", got.ID, tc.wantID)
			}
			exp := *tc.input
			exp.ID = got.ID
			if !got.Equal(&exp) {
				t.Fatalf("got %v, want %v", got, &exp)
			}
			fetched, err := svc.FetchUserByID(ctx, got.ID)
			if err != nil || !fetched.Equal(got) {
				t.Fatalf("fetch after save = %v, %v", fetched, err)
			}
		})
	}
}

func TestUserServiceFetchUserList(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("boom")
	tests := []struct {
		name    string
		seed    []model.User
		findErr error
		want    []model.User
	}{
		{name: "users exist", seed: []model.User{usAlice(), usBob()}, want: []model.User{usAlice(), usBob()}},
		{name: "empty store returns empty non-nil", want: []model.User{}},
		{name: "store error", findErr: dbErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(tc.seed...)
			fs.findErr = tc.findErr
			before := fs.snapshot()
			got, err := NewUserService(fs).FetchUserList(ctx)
			if tc.findErr != nil {
				if !errors.Is(err, tc.findErr) || got != nil {
					t.Fatalf("got %v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil {
				t.Fatal("list must not be nil")
			}
			if len(got) != len(fs.users) || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(before, fs.snapshot()) {
				t.Fatal("read modified store")
			}
		})
	}
}

func TestUserServiceFetchUserByID(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("boom")
	tests := []struct {
		name         string
		id           int
		findErr      error
		wantNotFound bool
		wantErr      error
	}{
		{name: "existing", id: 2},
		{name: "missing", id: 99, wantNotFound: true},
		{name: "store error", id: 1, findErr: dbErr, wantErr: dbErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(usAlice(), usBob())
			fs.findErr = tc.findErr
			before := fs.snapshot()
			got, err := NewUserService(fs).FetchUserByID(ctx, tc.id)
			switch {
			case tc.wantNotFound:
				if !errors.Is(err, ErrUserNotFound) || got != nil {
					t.Fatalf("got %v, %v", got, err)
				}
				if err.Error() != "User are not available" {
					t.Fatalf("msg = %q", err.Error())
				}
				var unf *UserNotFoundError
				if !errors.As(err, &unf) {
					t.Fatal("expected *UserNotFoundError")
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) || errors.Is(err, ErrUserNotFound) || got != nil {
					t.Fatalf("got %v, %v", got, err)
				}
			default:
				if err != nil || got == nil || got.ID != tc.id {
					t.Fatalf("got %v, %v", got, err)
				}
				b := usBob()
				if !got.Equal(&b) {
					t.Fatalf("got %v", got)
				}
			}
			if !reflect.DeepEqual(before, fs.snapshot()) {
				t.Fatal("read modified store")
			}
		})
	}
}

func TestUserServiceDeleteUser(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("boom")
	tests := []struct {
		name    string
		id      int
		delErr  error
		wantErr error
	}{
		{name: "existing", id: 1},
		{name: "missing", id: 42, wantErr: store.ErrNoRowsDeleted},
		{name: "store error", id: 1, delErr: dbErr, wantErr: dbErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(usAlice(), usBob())
			fs.delErr = tc.delErr
			before := fs.snapshot()
			svc := NewUserService(fs)
			err := svc.DeleteUser(ctx, tc.id)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if errors.Is(err, ErrUserNotFound) {
					t.Fatal("must not be ErrUserNotFound")
				}
				if !reflect.DeepEqual(before, fs.snapshot()) {
					t.Fatal("data changed")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if _, err := svc.FetchUserByID(ctx, tc.id); !errors.Is(err, ErrUserNotFound) {
				t.Fatalf("after delete err = %v", err)
			}
			if u, err := svc.FetchUserByID(ctx, 2); err != nil || u.ID != 2 {
				t.Fatal("other user affected")
			}
		})
	}
}

func TestUserServiceUpdateUser(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("boom")
	tests := []struct {
		name      string
		id        int
		input     *model.User
		saveErr   error
		wantErr   error
		wantCount int
	}{
		{name: "existing id updated", id: 1,
			input: &model.User{ID: 77, Name: usSP("alice-new"), Email: usSP("new@x.io")}, wantCount: 2},
		{name: "missing id creates new", id: 50,
			input: &model.User{Name: usSP("zed")}, wantCount: 3},
		{name: "nil user", id: 1, input: nil, wantErr: errNilUser, wantCount: 2},
		{name: "store error", id: 1, input: &model.User{Name: usSP("x")}, saveErr: dbErr, wantErr: dbErr, wantCount: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(usAlice(), usBob())
			fs.saveErr = tc.saveErr
			var origCopy model.User
			if tc.input != nil {
				origCopy = *tc.input
			}
			err := NewUserService(fs).UpdateUser(ctx, tc.id, tc.input)
			if len(fs.users) != tc.wantCount {
				t.Fatalf("count = %d, want %d", len(fs.users), tc.wantCount)
			}
			if tc.input != nil && !reflect.DeepEqual(*tc.input, origCopy) {
				t.Fatal("caller's user mutated")
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if fs.lastSaved == nil || fs.lastSaved.ID != tc.id {
				t.Fatalf("store received id %v, want %d", fs.lastSaved, tc.id)
			}
			if tc.id == 1 {
				got := fs.users[1]
				exp := origCopy
				exp.ID = 1
				if !got.Equal(&exp) {
					t.Fatalf("got %v want %v", &got, &exp)
				}
				b, want := fs.users[2], usBob()
				if !b.Equal(&want) {
					t.Fatal("other user modified")
				}
			}
		})
	}
}

func TestUserServiceGetUserNameByName(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("boom")
	tests := []struct {
		name    string
		query   string
		seed    []model.User
		findErr error
		wantNil bool
		wantErr error
	}{
		{name: "match", query: "bob", seed: []model.User{usAlice(), usBob()}},
		{name: "no match returns nil,nil", query: "nobody", seed: []model.User{usAlice()}, wantNil: true},
		{name: "non unique", query: "alice",
			seed:    []model.User{usAlice(), {ID: 3, Name: usSP("alice")}},
			wantErr: store.ErrNonUniqueResult},
		{name: "store error", query: "bob", seed: []model.User{usBob()}, findErr: dbErr, wantErr: dbErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newUSFakeStore(tc.seed...)
			fs.findErr = tc.findErr
			before := fs.snapshot()
			got, err := usGetByName(t, NewUserService(fs), ctx, tc.query)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("got %v, %v", got, err)
				}
			case tc.wantNil:
				if err != nil || got != nil {
					t.Fatalf("got %v, %v", got, err)
				}
			default:
				if err != nil || got == nil || got.Name == nil || *got.Name != tc.query {
					t.Fatalf("got %v, %v", got, err)
				}
			}
			if !reflect.DeepEqual(before, fs.snapshot()) {
				t.Fatal("read modified store")
			}
		})
	}
}