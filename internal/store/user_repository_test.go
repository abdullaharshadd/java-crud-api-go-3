package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"migrated-app/internal/model"
)

// ---------------------------------------------------------------------------
// Fake database/sql driver (scripted, ordered expectations)
// ---------------------------------------------------------------------------

type step struct {
	contains     string
	args         []driver.Value // nil => do not check
	rows         [][]driver.Value
	err          error
	rowsAffected int64
}

type fakeDB struct {
	mu        sync.Mutex
	t         *testing.T
	steps     []step
	idx       int
	commits   int
	rollbacks int
}

func (f *fakeDB) next(query string, args []driver.Value) (step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.steps) {
		f.t.Errorf("unexpected query: %s", query)
		return step{}, fmt.Errorf("unexpected query %q", query)
	}
	s := f.steps[f.idx]
	f.idx++
	if !strings.Contains(query, s.contains) {
		f.t.Errorf("query %d: got %q, want it to contain %q", f.idx, query, s.contains)
		return step{}, errors.New("query mismatch")
	}
	if s.args != nil && !reflect.DeepEqual(args, s.args) {
		f.t.Errorf("query %q: args got %#v, want %#v", query, args, s.args)
	}
	return s, s.err
}

func (f *fakeDB) Connect(context.Context) (driver.Conn, error) { return &fakeConn{f: f}, nil }
func (f *fakeDB) Driver() driver.Driver                        { return fakeDrv{} }

type fakeDrv struct{}

func (fakeDrv) Open(string) (driver.Conn, error) { return nil, errors.New("not supported") }

type fakeConn struct{ f *fakeDB }

func (c *fakeConn) Prepare(q string) (driver.Stmt, error) { return &fakeStmt{f: c.f, q: q}, nil }
func (c *fakeConn) Close() error                          { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)             { return &fakeTx{f: c.f}, nil }

type fakeTx struct{ f *fakeDB }

func (t *fakeTx) Commit() error {
	t.f.mu.Lock()
	t.f.commits++
	t.f.mu.Unlock()
	return nil
}
func (t *fakeTx) Rollback() error {
	t.f.mu.Lock()
	t.f.rollbacks++
	t.f.mu.Unlock()
	return nil
}

type fakeStmt struct {
	f *fakeDB
	q string
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	st, err := s.f.next(s.q, args)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(st.rowsAffected), nil
}
func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	st, err := s.f.next(s.q, args)
	if err != nil {
		return nil, err
	}
	n := 6
	if len(st.rows) > 0 {
		n = len(st.rows[0])
	}
	cols := make([]string, n)
	for i := range cols {
		cols[i] = fmt.Sprintf("c%d", i)
	}
	return &fakeRows{cols: cols, rows: st.rows}, nil
}

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	i    int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

func newRepo(t *testing.T, steps ...step) (*UserRepository, *fakeDB) {
	t.Helper()
	f := &fakeDB{t: t, steps: steps}
	db := sql.OpenDB(f)
	t.Cleanup(func() {
		db.Close()
		if f.idx != len(f.steps) {
			t.Errorf("only %d of %d expected queries executed", f.idx, len(f.steps))
		}
	})
	return NewUserRepository(db), f
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sp(s string) *string { return &s }

func hemrajRow() []driver.Value {
	return []driver.Value{int64(3), "hemraj", "hemrajmalhi1234@gmail.com", "root", "java developer", "Sr"}
}

func hemraj() *model.User {
	return model.NewUserWithAll(3, sp("hemraj"), sp("hemrajmalhi1234@gmail.com"), sp("root"), sp("java developer"), sp("Sr"))
}

var errDB = errors.New("db down")

// ---------------------------------------------------------------------------
// FindByName
// ---------------------------------------------------------------------------

func TestFindByName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rows      [][]driver.Value
		qErr      error
		wantUser  *model.User
		wantFound bool
		wantErrIs error
		wantErr   bool
	}{
		{name: "found", input: "hemraj", rows: [][]driver.Value{hemrajRow()}, wantUser: hemraj(), wantFound: true},
		{name: "not found returns nil", input: "nobody", rows: nil},
		{name: "non unique", input: "hemraj", rows: [][]driver.Value{hemrajRow(),
			{int64(4), "hemraj", "x@y.z", nil, nil, nil}}, wantErrIs: ErrNonUniqueResult, wantErr: true},
		{name: "case differing name passed through verbatim, db decides", input: "HEMRAJ", rows: nil},
		{name: "query error", input: "hemraj", qErr: errDB, wantErrIs: errDB, wantErr: true},
		{name: "scan error", input: "hemraj", rows: [][]driver.Value{{"bad", "x"}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, f := newRepo(t, step{
				contains: "FROM `user` WHERE user_name = ? LIMIT 2",
				args:     []driver.Value{tc.input},
				rows:     tc.rows,
				err:      tc.qErr,
			})
			u, found, err := repo.FindByName(context.Background(), tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !u.Equal(tc.wantUser) {
				t.Fatalf("user = %v, want %v", u, tc.wantUser)
			}
			if u != nil && *u.Name != tc.input {
				t.Fatalf("name = %q, want %q", *u.Name, tc.input)
			}
			if f.commits != 0 {
				t.Fatalf("read-only lookup must not open/commit transactions")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func TestSave(t *testing.T) {
	newUser := model.NewUserBuilder().Name("hemraj").Email("hemrajmalhi1234@gmail.com").
		Password("root").Role("java developer").About("Sr").Build()
	existing := hemraj()
	missing := model.NewUserWithAll(99, sp("ghost"), nil, nil, nil, nil)

	insertArgs := func(id int64, u *model.User) []driver.Value {
		v := func(s *string) driver.Value {
			if s == nil {
				return nil
			}
			return *s
		}
		return []driver.Value{id, v(u.Name), v(u.Email), v(u.Password), v(u.Role), v(u.About)}
	}

	tests := []struct {
		name        string
		in          *model.User
		steps       []step
		wantID      int
		wantErr     bool
		wantErrText string
		wantCommits int
	}{
		{name: "nil entity", in: nil, wantErr: true},
		{
			name: "new user gets id from sequence",
			in:   newUser,
			steps: []step{
				{contains: "SELECT next_val FROM hibernate_sequence FOR UPDATE", rows: [][]driver.Value{{int64(5)}}},
				{contains: "UPDATE hibernate_sequence SET next_val = ?", args: []driver.Value{int64(6), int64(5)}, rowsAffected: 1},
				{contains: "INSERT INTO `user`", args: insertArgs(5, newUser), rowsAffected: 1},
			},
			wantID: 5, wantCommits: 1,
		},
		{
			name: "existing user updated",
			in:   existing,
			steps: []step{
				{contains: "SELECT user_id FROM `user` WHERE user_id = ? FOR UPDATE", args: []driver.Value{int64(3)}, rows: [][]driver.Value{{int64(3)}}},
				{contains: "UPDATE `user` SET", args: []driver.Value{"hemraj", "hemrajmalhi1234@gmail.com", "root", "java developer", "Sr", int64(3)}, rowsAffected: 1},
			},
			wantID: 3, wantCommits: 1,
		},
		{
			name: "detached id not in db gets new id",
			in:   missing,
			steps: []step{
				{contains: "FOR UPDATE", args: []driver.Value{int64(99)}},
				{contains: "SELECT next_val", rows: [][]driver.Value{{int64(7)}}},
				{contains: "UPDATE hibernate_sequence", rowsAffected: 1},
				{contains: "INSERT INTO `user`", args: insertArgs(7, missing), rowsAffected: 1},
			},
			wantID: 7, wantCommits: 1,
		},
		{
			name:    "lock error",
			in:      existing,
			steps:   []step{{contains: "FOR UPDATE", err: errDB}},
			wantErr: true, wantErrText: "lock existing row",
		},
		{
			name:    "empty sequence",
			in:      newUser,
			steps:   []step{{contains: "SELECT next_val"}},
			wantErr: true, wantErrText: "hibernate_sequence is empty",
		},
		{
			name:    "null sequence",
			in:      newUser,
			steps:   []step{{contains: "SELECT next_val", rows: [][]driver.Value{{nil}}}},
			wantErr: true, wantErrText: "NULL",
		},
		{
			name: "concurrent sequence change",
			in:   newUser,
			steps: []step{
				{contains: "SELECT next_val", rows: [][]driver.Value{{int64(5)}}},
				{contains: "UPDATE hibernate_sequence", rowsAffected: 0},
			},
			wantErr: true, wantErrText: "changed concurrently",
		},
		{
			name: "insert error",
			in:   newUser,
			steps: []step{
				{contains: "SELECT next_val", rows: [][]driver.Value{{int64(5)}}},
				{contains: "UPDATE hibernate_sequence", rowsAffected: 1},
				{contains: "INSERT INTO `user`", err: errDB},
			},
			wantErr: true, wantErrText: "insert",
		},
		{
			name: "update error",
			in:   existing,
			steps: []step{
				{contains: "FOR UPDATE", rows: [][]driver.Value{{int64(3)}}},
				{contains: "UPDATE `user` SET", err: errDB},
			},
			wantErr: true, wantErrText: "update",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, f := newRepo(t, tc.steps...)
			var before model.User
			if tc.in != nil {
				before = *tc.in
			}
			got, err := repo.Save(context.Background(), tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.wantErrText != "" && !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErrText)
				}
				if got != nil {
					t.Fatalf("got user %v on error", got)
				}
				if f.commits != 0 {
					t.Fatal("must not commit on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("id = %d, want %d", got.ID, tc.wantID)
			}
			want := before
			want.ID = tc.wantID
			if !got.Equal(&want) {
				t.Fatalf("saved = %v, want %v", got, &want)
			}
			if !tc.in.Equal(&before) {
				t.Fatal("input must not be modified")
			}
			if f.commits != tc.wantCommits {
				t.Fatalf("commits = %d, want %d", f.commits, tc.wantCommits)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FindByID / ExistsByID / Count
// ---------------------------------------------------------------------------

func TestFindByID(t *testing.T) {
	tests := []struct {
		name      string
		rows      [][]driver.Value
		err       error
		want      *model.User
		wantFound bool
		wantErr   bool
	}{
		{name: "existing", rows: [][]driver.Value{hemrajRow()}, want: hemraj(), wantFound: true},
		{name: "null columns", rows: [][]driver.Value{{int64(3), nil, nil, nil, nil, nil}}, want: model.NewUserWithAll(3, nil, nil, nil, nil, nil), wantFound: true},
		{name: "missing", rows: nil},
		{name: "error", err: errDB, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "WHERE user_id = ?", args: []driver.Value{int64(3)}, rows: tc.rows, err: tc.err})
			u, found, err := repo.FindByID(context.Background(), 3)
			if (err != nil) != tc.wantErr || found != tc.wantFound || !u.Equal(tc.want) {
				t.Fatalf("got (%v,%v,%v), want (%v,%v,err=%v)", u, found, err, tc.want, tc.wantFound, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, errDB) {
				t.Fatalf("err not wrapped: %v", err)
			}
		})
	}
}

func TestExistsByID(t *testing.T) {
	tests := []struct {
		name    string
		rows    [][]driver.Value
		err     error
		want    bool
		wantErr bool
	}{
		{name: "exists", rows: [][]driver.Value{{int64(1)}}, want: true},
		{name: "absent"},
		{name: "error", err: errDB, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "SELECT 1 FROM `user` WHERE user_id = ?", args: []driver.Value{int64(8)}, rows: tc.rows, err: tc.err})
			got, err := repo.ExistsByID(context.Background(), 8)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got (%v,%v)", got, err)
			}
		})
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		name    string
		rows    [][]driver.Value
		err     error
		want    int64
		wantErr bool
	}{
		{name: "count", rows: [][]driver.Value{{int64(42)}}, want: 42},
		{name: "error", err: errDB, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "SELECT COUNT(*) FROM `user`", rows: tc.rows, err: tc.err})
			got, err := repo.Count(context.Background())
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got (%v,%v)", got, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FindAll / FindAllByID / FindPage
// ---------------------------------------------------------------------------

func TestFindAll(t *testing.T) {
	tests := []struct {
		name    string
		rows    [][]driver.Value
		err     error
		wantLen int
		wantErr bool
	}{
		{name: "empty non-nil", wantLen: 0},
		{name: "rows", rows: [][]driver.Value{hemrajRow(), {int64(4), "a", nil, nil, nil, nil}}, wantLen: 2},
		{name: "error", err: errDB, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "SELECT " + userColumns + " FROM `user`", rows: tc.rows, err: tc.err})
			got, err := repo.FindAll(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErr {
				return
			}
			if got == nil || len(got) != tc.wantLen {
				t.Fatalf("got %#v, want len %d non-nil", got, tc.wantLen)
			}
		})
	}
}

func TestFindAllByID(t *testing.T) {
	tests := []struct {
		name    string
		ids     []int
		steps   []step
		wantLen int
		wantErr bool
	}{
		{name: "empty ids no query", ids: nil},
		{name: "two ids", ids: []int{3, 9}, steps: []step{{
			contains: "WHERE user_id IN (?,?)", args: []driver.Value{int64(3), int64(9)},
			rows: [][]driver.Value{hemrajRow()}}}, wantLen: 1},
		{name: "error", ids: []int{1}, steps: []step{{contains: "IN (?)", err: errDB}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, tc.steps...)
			got, err := repo.FindAllByID(context.Background(), tc.ids)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if !tc.wantErr && (got == nil || len(got) != tc.wantLen) {
				t.Fatalf("got %#v", got)
			}
		})
	}
}

func TestFindPage(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		size      int
		sortBy    string
		desc      bool
		steps     []step
		wantLen   int
		wantTotal int64
		wantErrIs error
		wantErr   bool
	}{
		{name: "negative page", page: -1, size: 10, wantErr: true, wantErrIs: ErrInvalidPage},
		{name: "zero size", page: 0, size: 0, wantErr: true, wantErrIs: ErrInvalidPage},
		{name: "unknown sort", page: 0, size: 10, sortBy: "bogus", wantErr: true, wantErrIs: ErrInvalidPage},
		{name: "unsorted", page: 2, size: 5, steps: []step{
			{contains: "FROM `user` LIMIT ? OFFSET ?", args: []driver.Value{int64(5), int64(10)}, rows: [][]driver.Value{hemrajRow()}},
			{contains: "COUNT(*)", rows: [][]driver.Value{{int64(11)}}},
		}, wantLen: 1, wantTotal: 11},
		{name: "sorted desc", page: 0, size: 3, sortBy: "name", desc: true, steps: []step{
			{contains: "ORDER BY user_name DESC LIMIT ? OFFSET ?", args: []driver.Value{int64(3), int64(0)}},
			{contains: "COUNT(*)", rows: [][]driver.Value{{int64(0)}}},
		}},
		{name: "sorted asc", page: 0, size: 3, sortBy: "email", steps: []step{
			{contains: "ORDER BY user_email ASC"},
			{contains: "COUNT(*)", rows: [][]driver.Value{{int64(0)}}},
		}},
		{name: "query error", page: 0, size: 3, steps: []step{{contains: "LIMIT", err: errDB}}, wantErr: true, wantErrIs: errDB},
		{name: "count error", page: 0, size: 3, steps: []step{{contains: "LIMIT"}, {contains: "COUNT(*)", err: errDB}}, wantErr: true, wantErrIs: errDB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, tc.steps...)
			got, total, err := repo.FindPage(context.Background(), tc.page, tc.size, tc.sortBy, tc.desc)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
			}
			if tc.wantErr {
				return
			}
			if got == nil || len(got) != tc.wantLen || total != tc.wantTotal {
				t.Fatalf("got %d users total %d", len(got), total)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteByID / DeleteAll
// ---------------------------------------------------------------------------

func TestDeleteByID(t *testing.T) {
	tests := []struct {
		name      string
		affected  int64
		err       error
		wantErrIs error
	}{
		{name: "existing", affected: 1},
		{name: "missing", affected: 0, wantErrIs: ErrNoRowsDeleted},
		{name: "error", err: errDB, wantErrIs: errDB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "DELETE FROM `user` WHERE user_id = ?", args: []driver.Value{int64(3)}, rowsAffected: tc.affected, err: tc.err})
			err := repo.DeleteByID(context.Background(), 3)
			if tc.wantErrIs == nil && err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
			}
		})
	}
}

func TestDeleteAll(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{name: "ok"},
		{name: "error", err: errDB, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newRepo(t, step{contains: "DELETE FROM `user`", err: tc.err, rowsAffected: 2})
			if err := repo.DeleteAll(context.Background()); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers under test
// ---------------------------------------------------------------------------

func TestNullableAndFromNull(t *testing.T) {
	tests := []struct {
		name string
		in   *string
	}{
		{name: "nil", in: nil},
		{name: "empty", in: sp("")},
		{name: "value", in: sp("x")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := fromNull(nullable(tc.in))
			if (out == nil) != (tc.in == nil) || (out != nil && *out != *tc.in) {
				t.Fatalf("round trip mismatch: %v vs %v", out, tc.in)
			}
		})
	}
}

func TestSortableColumnsWhitelist(t *testing.T) {
	want := map[string]string{
		"id": model.UserColumnID, "name": model.UserColumnName, "email": model.UserColumnEmail,
		"password": model.UserColumnPassword, "role": model.UserColumnRole, "about": model.UserColumnAbout,
	}
	if !reflect.DeepEqual(sortableUserColumns, want) {
		t.Fatalf("got %v", sortableUserColumns)
	}
}