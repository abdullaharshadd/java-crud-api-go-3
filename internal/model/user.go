// Package model contains the domain entities of the smart-contact service.
package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Table and column names. Hibernate's default physical naming strategy
// (Spring Boot's SpringPhysicalNamingStrategy) lower-cases the names declared
// in @Table/@Column, so the real MySQL identifiers are lowercase.
const (
	UserTable          = "user"
	UserColumnID       = "user_id"
	UserColumnName     = "user_name"
	UserColumnEmail    = "user_email"
	UserColumnPassword = "user_password"
	UserColumnRole     = "user_role"
	UserColumnAbout    = "user_about"
)

// MsgNameBlank is the validation message of the @NotBlank constraint on name.
// The wording ("department") is a deliberate copy of the source.
const MsgNameBlank = "please Add the department Name"

// ErrNameBlank is returned by Validate when the name is nil, empty or only whitespace.
var ErrNameBlank = errors.New(MsgNameBlank)

// User is the persisted user entity (table `user`).
//
// String fields are pointers so that SQL NULL / JSON null round-trip exactly
// like Java's null references. The password is intentionally serialised,
// matching the source behaviour.
type User struct {
	ID       int     `json:"id"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
	About    *string `json:"about"`
}

// NewUser returns an empty User: ID is 0 and every string field is nil.
func NewUser() *User {
	return &User{}
}

// NewUserWithAll returns a User with every field set, in declaration order.
func NewUserWithAll(id int, name, email, password, role, about *string) *User {
	return &User{
		ID:       id,
		Name:     name,
		Email:    email,
		Password: password,
		Role:     role,
		About:    about,
	}
}

// GetID returns the user id.
func (u *User) GetID() int { return u.ID }

// SetID sets the user id.
func (u *User) SetID(id int) { u.ID = id }

// GetName returns the user name (nil when unset).
func (u *User) GetName() *string { return u.Name }

// SetName sets the user name.
func (u *User) SetName(v *string) { u.Name = v }

// GetEmail returns the email (nil when unset).
func (u *User) GetEmail() *string { return u.Email }

// SetEmail sets the email.
func (u *User) SetEmail(v *string) { u.Email = v }

// GetPassword returns the password (nil when unset).
func (u *User) GetPassword() *string { return u.Password }

// SetPassword sets the password.
func (u *User) SetPassword(v *string) { u.Password = v }

// GetRole returns the role (nil when unset).
func (u *User) GetRole() *string { return u.Role }

// SetRole sets the role.
func (u *User) SetRole(v *string) { u.Role = v }

// GetAbout returns the about text (nil when unset).
func (u *User) GetAbout() *string { return u.About }

// SetAbout sets the about text.
func (u *User) SetAbout(v *string) { u.About = v }

// Validate replaces the @NotBlank bean-validation constraint on name: the
// name must be non-nil and contain at least one non-whitespace character.
func (u *User) Validate() error {
	if u.Name == nil || strings.TrimFunc(*u.Name, unicode.IsSpace) == "" {
		return ErrNameBlank
	}
	return nil
}

// Equal reports value equality over all six fields (Lombok @EqualsAndHashCode).
func (u *User) Equal(o *User) bool {
	if u == nil || o == nil {
		return u == o
	}
	return u.ID == o.ID &&
		strPtrEqual(u.Name, o.Name) &&
		strPtrEqual(u.Email, o.Email) &&
		strPtrEqual(u.Password, o.Password) &&
		strPtrEqual(u.Role, o.Role) &&
		strPtrEqual(u.About, o.About)
}

// String renders every field in Lombok's toString format, e.g.
// User(id=1, name=x, email=null, password=null, role=null, about=null).
func (u *User) String() string {
	if u == nil {
		return "null"
	}
	return fmt.Sprintf("User(id=%d, name=%s, email=%s, password=%s, role=%s, about=%s)",
		u.ID, strPtr(u.Name), strPtr(u.Email), strPtr(u.Password), strPtr(u.Role), strPtr(u.About))
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func strPtr(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

// UserBuilder is a fluent builder for User (Lombok @Builder).
type UserBuilder struct {
	u User
}

// NewUserBuilder starts a new User builder.
func NewUserBuilder() *UserBuilder { return &UserBuilder{} }

// ID sets the id.
func (b *UserBuilder) ID(id int) *UserBuilder { b.u.ID = id; return b }

// Name sets the name.
func (b *UserBuilder) Name(v string) *UserBuilder { b.u.Name = &v; return b }

// Email sets the email.
func (b *UserBuilder) Email(v string) *UserBuilder { b.u.Email = &v; return b }

// Password sets the password.
func (b *UserBuilder) Password(v string) *UserBuilder { b.u.Password = &v; return b }

// Role sets the role.
func (b *UserBuilder) Role(v string) *UserBuilder { b.u.Role = &v; return b }

// About sets the about text.
func (b *UserBuilder) About(v string) *UserBuilder { b.u.About = &v; return b }

// Build returns a new User holding the builder's values.
func (b *UserBuilder) Build() *User {
	u := b.u
	return &u
}

// UserSchemaStatements is the DDL equivalent of Hibernate's
// ddl-auto=update for the User entity. Every statement is a no-op on an
// existing database.
//
// MIGRATION_NOTE: @GeneratedValue(strategy = AUTO) on Hibernate 5 + MySQL uses
// the hibernate_sequence table rather than AUTO_INCREMENT; this is preserved
// so ids stay compatible with existing data. Repositories must allocate ids
// from hibernate_sequence (SELECT next_val ... FOR UPDATE; UPDATE next_val+1).
// MIGRATION_NOTE: Hibernate names the unique key UK_<hash>; here it is
// uk_user_email. On an existing DB the original constraint remains.
var UserSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS hibernate_sequence (
	next_val BIGINT
) ENGINE=InnoDB`,
	`INSERT INTO hibernate_sequence (next_val)
SELECT 1 FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM hibernate_sequence)`,
	"CREATE TABLE IF NOT EXISTS `user` (\n" +
		"\tuser_id INTEGER NOT NULL,\n" +
		"\tuser_about VARCHAR(500),\n" +
		"\tuser_email VARCHAR(255),\n" +
		"\tuser_name VARCHAR(255),\n" +
		"\tuser_password VARCHAR(255),\n" +
		"\tuser_role VARCHAR(255),\n" +
		"\tPRIMARY KEY (user_id),\n" +
		"\tCONSTRAINT uk_user_email UNIQUE (user_email)\n" +
		") ENGINE=InnoDB",
}

// EnsureUserSchema creates the hibernate_sequence and `user` tables if they do
// not exist yet. It should be called once at startup.
func EnsureUserSchema(ctx context.Context, db *sql.DB) error {
	for i, stmt := range UserSchemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure user schema (statement %d): %w", i+1, err)
		}
	}
	return nil
}
