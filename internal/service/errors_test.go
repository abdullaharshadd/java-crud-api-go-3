package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"migrated-app/internal/model"
)

// fakeStore is an in-memory mock of UserStore used to exercise
// FetchUserByID's not-found path.
type fakeStore struct {
	byID    map[int]*model.User
	findErr error
}

func (f *fakeStore) Save(ctx context.Context, u *model.User) (*model.User, error) { return u, nil }
func (f *fakeStore) FindAll(ctx context.Context) ([]model.User, error)            { return nil, nil }
func (f *fakeStore) FindByID(ctx context.Context, id int) (*model.User, bool, error) {
	if f.findErr != nil {
		return nil, false, f.findErr
	}
	u, ok := f.byID[id]
	return u, ok, nil
}
func (f *fakeStore) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	return nil, false, nil
}
func (f *fakeStore) DeleteByID(ctx context.Context, id int) error { return nil }

func TestUserNotFoundError_Constructors(t *testing.T) {
	cause := errors.New("db: row missing")

	tests := []struct {
		name      string
		err       *UserNotFoundError
		wantMsg   string
		wantCause error
	}{
		{"no args (empty message)", NewUserNotFoundError(""), userNotFoundMessage, nil},
		{"message only", NewUserNotFoundError("User not found"), "User not found", nil},
		{"message and cause", WrapUserNotFound("missing", cause), "missing", cause},
		{"message and nil cause", WrapUserNotFound("missing", nil), "missing", nil},
		{"cause only derives message", WrapUserNotFound("", cause), cause.Error(), cause},
		{"nil cause and empty message", WrapUserNotFound("", nil), userNotFoundMessage, nil},
		// Protected 4-arg constructor maps to WrapUserNotFound; flags have no Go analogue.
		{"protected ctor equivalent", WrapUserNotFound("msg", cause), "msg", cause},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("constructor returned nil")
			}
			if got := tc.err.Error(); got != tc.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tc.wantMsg)
			}
			if got := tc.err.Unwrap(); got != tc.wantCause {
				t.Errorf("Unwrap() = %v, want %v", got, tc.wantCause)
			}
			if got := errors.Unwrap(tc.err); got != tc.wantCause {
				t.Errorf("errors.Unwrap() = %v, want %v", got, tc.wantCause)
			}
			if !errors.Is(tc.err, ErrUserNotFound) {
				t.Error("errors.Is(err, ErrUserNotFound) = false, want true")
			}
			if tc.wantCause != nil && !errors.Is(tc.err, tc.wantCause) {
				t.Error("errors.Is(err, cause) = false, want true")
			}
			var asErr *UserNotFoundError
			if !errors.As(tc.err, &asErr) || asErr != tc.err {
				t.Error("errors.As failed to extract *UserNotFoundError")
			}
			// Assignable to the error interface (Exception/Throwable analogue).
			var _ error = tc.err
		})
	}
}

func TestErrUserNotFoundSentinel(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"sentinel itself", ErrUserNotFound, true},
		{"wrapped sentinel", fmt.Errorf("ctx: %w", ErrUserNotFound), true},
		{"wrapped typed error", fmt.Errorf("ctx: %w", NewUserNotFoundError("x")), true},
		{"unrelated error", errors.New("User are not available"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, ErrUserNotFound); got != tc.want {
				t.Errorf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
	if ErrUserNotFound.Error() != "User are not available" {
		t.Errorf("sentinel message = %q", ErrUserNotFound.Error())
	}
}

func TestFetchUserByID_NotFoundUsesUserNotFoundError(t *testing.T) {
	storeErr := errors.New("connection refused")
	tests := []struct {
		name         string
		store        *fakeStore
		id           int
		wantNotFound bool
		wantErr      bool
		wantMsg      string
	}{
		{"found", &fakeStore{byID: map[int]*model.User{1: {ID: 1}}}, 1, false, false, ""},
		{"missing", &fakeStore{byID: map[int]*model.User{}}, 2, true, true, userNotFoundMessage},
		{"store error", &fakeStore{findErr: storeErr}, 3, false, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewUserService(tc.store)
			u, err := svc.FetchUserByID(context.Background(), tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got := errors.Is(err, ErrUserNotFound); got != tc.wantNotFound {
				t.Errorf("errors.Is(err, ErrUserNotFound) = %v, want %v", got, tc.wantNotFound)
			}
			if tc.wantMsg != "" && err.Error() != tc.wantMsg {
				t.Errorf("message = %q, want %q", err.Error(), tc.wantMsg)
			}
			if !tc.wantErr && (u == nil || u.ID != tc.id) {
				t.Errorf("unexpected user %+v", u)
			}
			if tc.name == "store error" && !errors.Is(err, storeErr) {
				t.Error("store error not wrapped")
			}
		})
	}
}