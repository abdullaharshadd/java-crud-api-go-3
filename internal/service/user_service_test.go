package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"migrated-app/internal/model"
	"migrated-app/internal/store"
)

type fakeStore struct {
	saveCalls   int
	savedArg    *model.User
	saveRet     *model.User
	saveErr     error
	findAllRet  []model.User
	findAllErr  error
	findAllCall int
	byIDRet     *model.User
	byIDOK      bool
	byIDErr     error
	byIDArg     int
	byNameRet   *model.User
	byNameOK    bool
	byNameErr   error
	byNameArg   string
	delCalls    int
	delArg      int
	delErr      error
}

func (f *fakeStore) Save(ctx context.Context, u *model.User) (*model.User, error) {
	f.saveCalls++
	cp := *u
	f.savedArg = &cp
	return f.saveRet, f.saveErr
}
func (f *fakeStore) FindAll(ctx context.Context) ([]model.User, error) {
	f.findAllCall++
	return f.findAllRet, f.findAllErr
}
func (f *fakeStore) FindByID(ctx context.Context, id int) (*model.User, bool, error) {
	f.byIDArg = id
	return f.byIDRet, f.byIDOK, f.byIDErr
}
func (f *fakeStore) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	f.byNameArg = name
	return f.byNameRet, f.byNameOK, f.byNameErr
}
func (f *fakeStore) DeleteByID(ctx context.Context, id int) error {
	f.delCalls++
	f.delArg = id
	return f.delErr
}

func hemraj() *model.User {
	return model.NewUserBuilder().ID(3).Name("hemraj").Email("hemrajmalhi1234@gmail.com").
		About("Sr").Password("root").Role("java developer").Build()
}

func TestSaveUser(t *testing.T) {
	boom := errors.New("db down")
	tests := []struct {
		name      string
		in        *model.User
		ret       *model.User
		err       error
		wantErr   error
		wantCalls int
	}{
		{"valid user returns repo result", model.NewUserBuilder().Name("a").Build(), model.NewUserBuilder().ID(7).Name("a").Build(), nil, nil, 1},
		{"repo error propagates", model.NewUserBuilder().Name("a").Build(), nil, boom, boom, 1},
		{"nil user", nil, nil, nil, errNilUser, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{saveRet: tc.ret, saveErr: tc.err}
			got, err := NewUserService(fs).SaveUser(context.Background(), tc.in)
			if fs.saveCalls != tc.wantCalls {
				t.Fatalf("save calls = %d, want %d", fs.saveCalls, tc.wantCalls)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("expected nil user")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != tc.ret {
				t.Fatalf("expected repository result pointer to be returned")
			}
			if got == tc.in {
				t.Fatalf("returned input instead of repository result")
			}
			if !fs.savedArg.Equal(tc.in) {
				t.Fatalf("saved %v, want %v", fs.savedArg, tc.in)
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		ret     []model.User
		err     error
		wantLen int
		wantErr bool
	}{
		{"users exist", []model.User{*hemraj(), *model.NewUserBuilder().ID(4).Name("b").Build()}, nil, 2, false},
		{"nil from store becomes empty", nil, nil, 0, false},
		{"empty slice", []model.User{}, nil, 0, false},
		{"error", nil, boom, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{findAllRet: tc.ret, findAllErr: tc.err}
			got, err := NewUserService(fs).FetchUserList(context.Background())
			if tc.wantErr {
				if !errors.Is(err, boom) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("got nil slice")
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			for i := range got {
				if !got[i].Equal(&tc.ret[i]) {
					t.Fatalf("item %d mismatch", i)
				}
			}
			if fs.saveCalls != 0 || fs.delCalls != 0 {
				t.Fatal("read op modified data")
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		id       int
		ret      *model.User
		ok       bool
		err      error
		notFound bool
		wantErr  error
	}{
		{"found", 3, hemraj(), true, nil, false, nil},
		{"not found", 99, nil, false, nil, true, nil},
		{"store error", 1, nil, false, boom, false, boom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{byIDRet: tc.ret, byIDOK: tc.ok, byIDErr: tc.err}
			got, err := NewUserService(fs).FetchUserByID(context.Background(), tc.id)
			if fs.byIDArg != tc.id {
				t.Fatalf("id passed = %d", fs.byIDArg)
			}
			switch {
			case tc.notFound:
				if !errors.Is(err, ErrUserNotFound) {
					t.Fatalf("err = %v, want ErrUserNotFound", err)
				}
				if err.Error() != "User are not available" {
					t.Fatalf("msg = %q", err.Error())
				}
				var unf *UserNotFoundError
				if !errors.As(err, &unf) {
					t.Fatal("expected *UserNotFoundError")
				}
				if got != nil {
					t.Fatal("expected nil user")
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if errors.Is(err, ErrUserNotFound) {
					t.Fatal("store error should not be not-found")
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if got == nil || !got.Equal(tc.ret) {
					t.Fatalf("got %v", got)
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		err     error
		wantErr error
	}{
		{"exists", 3, nil, nil},
		{"missing propagates store error", 42, fmt.Errorf("delete user 42: %w", store.ErrNoRowsDeleted), store.ErrNoRowsDeleted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{delErr: tc.err}
			err := NewUserService(fs).DeleteUser(context.Background(), tc.id)
			if fs.delCalls != 1 || fs.delArg != tc.id {
				t.Fatalf("calls=%d arg=%d", fs.delCalls, fs.delArg)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v", err)
			}
			if errors.Is(err, ErrUserNotFound) {
				t.Fatal("should not be UserNotFound")
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		id        int
		in        *model.User
		err       error
		wantErr   error
		wantCalls int
	}{
		{"overwrites id", 5, model.NewUserBuilder().ID(1).Name("x").Build(), nil, nil, 1},
		{"zero id in user", 9, model.NewUserBuilder().Name("y").Build(), nil, nil, 1},
		{"persistence error", 5, model.NewUserBuilder().Name("x").Build(), boom, boom, 1},
		{"nil user", 5, nil, nil, errNilUser, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{saveErr: tc.err}
			err := NewUserService(fs).UpdateUser(context.Background(), tc.id, tc.in)
			if fs.saveCalls != tc.wantCalls {
				t.Fatalf("save calls = %d", fs.saveCalls)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if errors.Is(err, ErrUserNotFound) {
				t.Fatal("UserNotFound must never be returned")
			}
			if tc.in != nil {
				if tc.in.ID != tc.id {
					t.Fatalf("input id = %d, want %d", tc.in.ID, tc.id)
				}
				if fs.savedArg.ID != tc.id {
					t.Fatalf("saved id = %d", fs.savedArg.ID)
				}
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		query     string
		ret       *model.User
		ok        bool
		err       error
		wantFound bool
		wantErr   error
	}{
		{"found hemraj", "hemraj", hemraj(), true, nil, true, nil},
		{"not found", "nobody", nil, false, nil, false, nil},
		{"non unique", "dup", nil, false, store.ErrNonUniqueResult, false, store.ErrNonUniqueResult},
		{"store error", "x", nil, false, boom, false, boom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{byNameRet: tc.ret, byNameOK: tc.ok, byNameErr: tc.err}
			u, found, err := NewUserService(fs).GetUserNameByName(context.Background(), tc.query)
			if fs.byNameArg != tc.query {
				t.Fatalf("name passed = %q", fs.byNameArg)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if u != nil || found {
					t.Fatal("expected nil,false")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v", found)
			}
			if !found {
				if u != nil {
					t.Fatal("expected nil user")
				}
				return
			}
			if u == nil || u.Name == nil || *u.Name != tc.query {
				t.Fatalf("got %v", u)
			}
			if fs.saveCalls != 0 || fs.delCalls != 0 {
				t.Fatal("read op modified data")
			}
		})
	}
}