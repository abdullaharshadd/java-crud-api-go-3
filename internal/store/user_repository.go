// Package store contains the database/sql repositories that replace the
// source's Spring Data JPA repositories.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"migrated-app/internal/model"
)

// ErrNoRowsDeleted is returned by DeleteByID when no user has the given id.
// It mirrors Spring Data's EmptyResultDataAccessException, which the source
// surfaced as a 500 response.
var ErrNoRowsDeleted = errors.New("store: no user entity with the given id exists")

// ErrNonUniqueResult is returned by FindByName when more than one user has
// the requested name. It mirrors Spring Data's
// IncorrectResultSizeDataAccessException (surfaced as a 500 by the source).
var ErrNonUniqueResult = errors.New("store: query did not return a unique result")

// ErrInvalidPage is returned by FindPage for a negative page or non-positive size.
var ErrInvalidPage = errors.New("store: invalid page request")

// userColumns is the fixed select list, in the order scanUser expects.
const userColumns = "user_id, user_name, user_email, user_password, user_role, user_about"

// sortableUserColumns whitelists the property names accepted by FindPage
// (JPA property name -> SQL column).
var sortableUserColumns = map[string]string{
	"id":       model.UserColumnID,
	"name":     model.UserColumnName,
	"email":    model.UserColumnEmail,
	"password": model.UserColumnPassword,
	"role":     model.UserColumnRole,
	"about":    model.UserColumnAbout,
}

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// UserRepository persists model.User values in MySQL. It replaces the Spring
// Data JPA interface UserDao (JpaRepository<User, Integer>).
//
// MIGRATION_NOTE: JPA returned managed entities with dirty checking; here
// every returned *model.User is a detached value — changes must be written
// back explicitly with Save.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository returns a UserRepository backed by db.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Save inserts u when it is new or merges it into the existing row, following
// Hibernate's merge semantics used by SimpleJpaRepository.save:
//   - ID == 0: a new id is allocated from hibernate_sequence and the row is inserted.
//   - ID != 0 and the row exists: every column is updated.
//   - ID != 0 and the row does not exist: like Hibernate's merge of a detached
//     entity, a NEW id is allocated and the row is inserted (the given id is ignored).
//
// The whole operation runs in one transaction. The persisted user is returned;
// u itself is not modified.
func (r *UserRepository) Save(ctx context.Context, u *model.User) (*model.User, error) {
	if u == nil {
		return nil, errors.New("save user: entity must not be nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("save user: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	saved := *u

	exists := false
	if saved.ID != 0 {
		var id int
		err := tx.QueryRowContext(ctx,
			"SELECT user_id FROM `user` WHERE user_id = ? FOR UPDATE", saved.ID).Scan(&id)
		switch {
		case err == nil:
			exists = true
		case errors.Is(err, sql.ErrNoRows):
			exists = false
		default:
			return nil, fmt.Errorf("save user %d: lock existing row: %w", saved.ID, err)
		}
	}

	if exists {
		_, err := tx.ExecContext(ctx,
			"UPDATE `user` SET user_name = ?, user_email = ?, user_password = ?, user_role = ?, user_about = ? WHERE user_id = ?",
			nullable(saved.Name), nullable(saved.Email), nullable(saved.Password),
			nullable(saved.Role), nullable(saved.About), saved.ID)
		if err != nil {
			return nil, fmt.Errorf("save user %d: update: %w", saved.ID, err)
		}
	} else {
		id, err := nextID(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("save user: %w", err)
		}
		saved.ID = id
		_, err = tx.ExecContext(ctx,
			"INSERT INTO `user` (user_id, user_name, user_email, user_password, user_role, user_about) VALUES (?, ?, ?, ?, ?, ?)",
			saved.ID, nullable(saved.Name), nullable(saved.Email), nullable(saved.Password),
			nullable(saved.Role), nullable(saved.About))
		if err != nil {
			return nil, fmt.Errorf("save user: insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("save user: commit: %w", err)
	}
	return &saved, nil
}

// nextID allocates the next id from hibernate_sequence (allocation size 1),
// exactly as Hibernate 5's SequenceStyleGenerator does on MySQL.
func nextID(ctx context.Context, tx *sql.Tx) (int, error) {
	var next sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT next_val FROM hibernate_sequence FOR UPDATE").Scan(&next)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("allocate id: hibernate_sequence is empty")
		}
		return 0, fmt.Errorf("allocate id: read hibernate_sequence: %w", err)
	}
	if !next.Valid {
		return 0, errors.New("allocate id: hibernate_sequence.next_val is NULL")
	}
	res, err := tx.ExecContext(ctx,
		"UPDATE hibernate_sequence SET next_val = ? WHERE next_val = ?", next.Int64+1, next.Int64)
	if err != nil {
		return 0, fmt.Errorf("allocate id: advance hibernate_sequence: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("allocate id: rows affected: %w", err)
	}
	if n == 0 {
		return 0, errors.New("allocate id: hibernate_sequence changed concurrently")
	}
	return int(next.Int64), nil
}

// FindAll returns every user. The result is never nil (an empty table yields
// an empty slice, which serialises as [] like the source's List).
func (r *UserRepository) FindAll(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+userColumns+" FROM `user`")
	if err != nil {
		return nil, fmt.Errorf("find all users: %w", err)
	}
	users, err := scanUsers(rows)
	if err != nil {
		return nil, fmt.Errorf("find all users: %w", err)
	}
	return users, nil
}

// FindPage returns one page of users ordered by the given property
// (JpaRepository's findAll(Pageable)) together with the total row count.
// sortBy must be one of id, name, email, password, role, about, or empty for
// unsorted; page is zero-based.
func (r *UserRepository) FindPage(ctx context.Context, page, size int, sortBy string, desc bool) ([]model.User, int64, error) {
	if page < 0 || size <= 0 {
		return nil, 0, fmt.Errorf("find user page %d size %d: %w", page, size, ErrInvalidPage)
	}
	query := "SELECT " + userColumns + " FROM `user`"
	if sortBy != "" {
		col, ok := sortableUserColumns[sortBy]
		if !ok {
			return nil, 0, fmt.Errorf("find user page: unknown sort property %q: %w", sortBy, ErrInvalidPage)
		}
		dir := "ASC"
		if desc {
			dir = "DESC"
		}
		query += " ORDER BY " + col + " " + dir
	}
	query += " LIMIT ? OFFSET ?"

	rows, err := r.db.QueryContext(ctx, query, size, int64(page)*int64(size))
	if err != nil {
		return nil, 0, fmt.Errorf("find user page: %w", err)
	}
	users, err := scanUsers(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("find user page: %w", err)
	}
	total, err := r.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("find user page: %w", err)
	}
	return users, total, nil
}

// FindByID looks up a user by id. It returns (nil, false, nil) when no row
// matches (the source's Optional.empty()).
func (r *UserRepository) FindByID(ctx context.Context, id int) (*model.User, bool, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM `user` WHERE user_id = ?", id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("find user by id %d: %w", id, err)
	}
	return u, true, nil
}

// FindByName looks up the single user whose name exactly equals name (the
// derived query findByName). It returns (nil, false, nil) when no user
// matches (the source returned null) and ErrNonUniqueResult when more than
// one does.
//
// MIGRATION_NOTE: equality follows the column's MySQL collation (usually
// case-insensitive), exactly like the JPA-generated WHERE clause did.
func (r *UserRepository) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM `user` WHERE user_name = ? LIMIT 2", name)
	if err != nil {
		return nil, false, fmt.Errorf("find user by name: %w", err)
	}
	users, err := scanUsers(rows)
	if err != nil {
		return nil, false, fmt.Errorf("find user by name: %w", err)
	}
	switch len(users) {
	case 0:
		return nil, false, nil
	case 1:
		return &users[0], true, nil
	default:
		return nil, false, fmt.Errorf("find user by name %q: %w", name, ErrNonUniqueResult)
	}
}

// ExistsByID reports whether a user with the given id exists.
func (r *UserRepository) ExistsByID(ctx context.Context, id int) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, "SELECT 1 FROM `user` WHERE user_id = ? LIMIT 1", id).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("exists user by id %d: %w", id, err)
	}
	return true, nil
}

// Count returns the number of users.
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `user`").Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// DeleteByID removes the user with the given id. It returns ErrNoRowsDeleted
// when no such user exists (Spring Data's EmptyResultDataAccessException).
func (r *UserRepository) DeleteByID(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM `user` WHERE user_id = ?", id)
	if err != nil {
		return fmt.Errorf("delete user %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user %d: rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete user %d: %w", id, ErrNoRowsDeleted)
	}
	return nil
}

// DeleteAll removes every user.
func (r *UserRepository) DeleteAll(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM `user`"); err != nil {
		return fmt.Errorf("delete all users: %w", err)
	}
	return nil
}

// FindAllByID returns the users whose ids are in ids (findAllById). Missing
// ids are silently skipped. The result is never nil.
func (r *UserRepository) FindAllByID(ctx context.Context, ids []int) ([]model.User, error) {
	if len(ids) == 0 {
		return []model.User{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM `user` WHERE user_id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, fmt.Errorf("find users by ids: %w", err)
	}
	users, err := scanUsers(rows)
	if err != nil {
		return nil, fmt.Errorf("find users by ids: %w", err)
	}
	return users, nil
}

// scanUsers drains and closes rows. It always returns a non-nil slice on success.
func scanUsers(rows *sql.Rows) ([]model.User, error) {
	defer rows.Close()
	users := []model.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return users, nil
}

// scanUser reads one row produced by a "SELECT userColumns" query.
func scanUser(s rowScanner) (*model.User, error) {
	var (
		u                                  model.User
		name, email, password, role, about sql.NullString
	)
	if err := s.Scan(&u.ID, &name, &email, &password, &role, &about); err != nil {
		return nil, err
	}
	u.Name = fromNull(name)
	u.Email = fromNull(email)
	u.Password = fromNull(password)
	u.Role = fromNull(role)
	u.About = fromNull(about)
	return &u, nil
}

func fromNull(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func nullable(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
