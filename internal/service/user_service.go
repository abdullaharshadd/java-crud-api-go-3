package service

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/model"
)

// errNilUser is returned when a nil *model.User is passed to a write method.
var errNilUser = errors.New("user must not be nil")

// UserStore is the persistence contract the user service depends on.
// *store.UserRepository satisfies it. It replaces the Spring Data UserDao.
type UserStore interface {
	// Save inserts or merges u and returns the persisted entity.
	Save(ctx context.Context, u *model.User) (*model.User, error)
	// FindAll returns every stored user.
	FindAll(ctx context.Context) ([]model.User, error)
	// FindByID returns the user with the given id. It reports false when no
	// user has that id.
	FindByID(ctx context.Context, id int) (*model.User, bool, error)
	// FindByName returns the single user with the given name. It reports
	// false when no user has that name.
	FindByName(ctx context.Context, name string) (*model.User, bool, error)
	// DeleteByID removes the user with the given id.
	DeleteByID(ctx context.Context, id int) error
}

// UserService is the business contract for managing users. It replaces the
// Java UserService interface and its UserServiceImp implementation.
type UserService interface {
	// SaveUser persists user, either inserting it or merging it into an
	// existing row, and returns the saved entity.
	SaveUser(ctx context.Context, user *model.User) (*model.User, error)
	// FetchUserList returns all stored users. The result is never nil.
	FetchUserList(ctx context.Context) ([]model.User, error)
	// FetchUserByID returns the user with the given id. If no such user
	// exists, the error satisfies errors.Is(err, ErrUserNotFound).
	FetchUserByID(ctx context.Context, id int) (*model.User, error)
	// DeleteUser removes the user with the given id.
	DeleteUser(ctx context.Context, id int) error
	// UpdateUser stores user's values under the given id.
	UpdateUser(ctx context.Context, id int, user *model.User) error
	// GetUserNameByName returns the user whose name equals name. It returns
	// (nil, nil) when no user matches, just as the source returned null.
	GetUserNameByName(ctx context.Context, name string) (*model.User, error)
}

// userService is the default UserService and delegates to a UserStore.
type userService struct {
	store UserStore
}

// NewUserService returns a UserService backed by store.
func NewUserService(store UserStore) UserService {
	return &userService{store: store}
}

// SaveUser implements UserService.
func (s *userService) SaveUser(ctx context.Context, user *model.User) (*model.User, error) {
	if user == nil {
		return nil, fmt.Errorf("save user: %w", errNilUser)
	}
	saved, err := s.store.Save(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("save user: %w", err)
	}
	return saved, nil
}

// FetchUserList implements UserService.
func (s *userService) FetchUserList(ctx context.Context) ([]model.User, error) {
	users, err := s.store.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch user list: %w", err)
	}
	if users == nil {
		users = []model.User{}
	}
	return users, nil
}

// FetchUserByID implements UserService.
func (s *userService) FetchUserByID(ctx context.Context, id int) (*model.User, error) {
	user, ok, err := s.store.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetch user %d: %w", id, err)
	}
	if !ok {
		// Same message as the source: "User are not available".
		return nil, NewUserNotFoundError(userNotFoundMessage)
	}
	return user, nil
}

// DeleteUser implements UserService.
//
// MIGRATION_NOTE: deleting a missing id returns the store's error
// (store.ErrNoRowsDeleted), the equivalent of Spring Data's
// EmptyResultDataAccessException. The source surfaced that as a 500, and the
// handler should do the same.
func (s *userService) DeleteUser(ctx context.Context, id int) error {
	if err := s.store.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("delete user %d: %w", id, err)
	}
	return nil
}

// UpdateUser implements UserService.
//
// It sets the id on a copy of user and saves it with merge semantics. As in
// the source, if no row has that id the store inserts a new row with a
// freshly allocated id instead of failing.
//
// MIGRATION_NOTE: the source mutated the caller's object (user.setId(id)).
// Here a copy is changed, so the caller's value stays untouched.
func (s *userService) UpdateUser(ctx context.Context, id int, user *model.User) error {
	if user == nil {
		return fmt.Errorf("update user %d: %w", id, errNilUser)
	}
	updated := *user
	updated.ID = id
	if _, err := s.store.Save(ctx, &updated); err != nil {
		return fmt.Errorf("update user %d: %w", id, err)
	}
	return nil
}

// GetUserNameByName implements UserService.
func (s *userService) GetUserNameByName(ctx context.Context, name string) (*model.User, error) {
	user, ok, err := s.store.FindByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get user by name: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return user, nil
}
