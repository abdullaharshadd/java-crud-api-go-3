package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

type fakeService struct {
	mu sync.Mutex

	saveCalls   []*model.User
	saveErr     error
	listResult  []model.User
	listErr     error
	listCalls   int
	byIDResult  *model.User
	byIDErr     error
	byIDCalls   []int
	deleteErr   error
	deleteCalls []int
	updateErr   error
	updateIDs   []int
	updateUsers []*model.User
	byName      *model.User
	byNameFound bool
	byNameErr   error
	byNameCalls []string
}

func (f *fakeService) SaveUser(_ context.Context, u *model.User) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCalls = append(f.saveCalls, u)
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return u, nil
}

func (f *fakeService) FetchUserList(_ context.Context) ([]model.User, error) {
	f.listCalls++
	return f.listResult, f.listErr
}

func (f *fakeService) FetchUserByID(_ context.Context, id int) (*model.User, error) {
	f.byIDCalls = append(f.byIDCalls, id)
	return f.byIDResult, f.byIDErr
}

func (f *fakeService) DeleteUser(_ context.Context, id int) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return f.deleteErr
}

func (f *fakeService) UpdateUser(_ context.Context, id int, u *model.User) error {
	f.updateIDs = append(f.updateIDs, id)
	f.updateUsers = append(f.updateUsers, u)
	if f.updateErr != nil {
		return f.updateErr
	}
	u.ID = id
	return nil
}

func (f *fakeService) GetUserNameByName(_ context.Context, name string) (*model.User, bool, error) {
	f.byNameCalls = append(f.byNameCalls, name)
	return f.byName, f.byNameFound, f.byNameErr
}

func newServer(t *testing.T, svc *fakeService) (http.Handler, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	h := NewHandler(svc, zerolog.New(buf))
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, buf
}

func do(t *testing.T, h http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func strp(s string) *string { return &s }

func TestSaveUser(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		nilBody    bool
		svcErr     error
		wantStatus int
		wantBody   string
		wantCalls  int
	}{
		{name: "valid", body: `{"name":"alice","email":"a@x.com","extra":1}`, wantStatus: 200, wantBody: MsgUserSaved, wantCalls: 1},
		{name: "blank name", body: `{"name":"   "}`, wantStatus: 400, wantCalls: 0},
		{name: "missing name", body: `{"email":"a@x.com"}`, wantStatus: 400, wantCalls: 0},
		{name: "malformed json", body: `{"name":`, wantStatus: 400, wantCalls: 0},
		{name: "empty body", body: ``, wantStatus: 400, wantCalls: 0},
		{name: "null body", body: `null`, wantStatus: 400, wantCalls: 0},
		{name: "service error", body: `{"name":"bob"}`, svcErr: errors.New("db down"), wantStatus: 500, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{saveErr: tc.svcErr}
			h, logs := newServer(t, svc)
			rec := do(t, h, http.MethodPost, "/save_user_data", strings.NewReader(tc.body))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if len(svc.saveCalls) != tc.wantCalls {
				t.Fatalf("save calls = %d, want %d", len(svc.saveCalls), tc.wantCalls)
			}
			if !strings.Contains(logs.String(), "inside the saveUser of UserController ") {
				t.Errorf("missing log, got %q", logs.String())
			}
			switch tc.wantStatus {
			case 200:
				if rec.Body.String() != tc.wantBody {
					t.Errorf("body = %q", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
					t.Errorf("content-type = %q", ct)
				}
				if got := svc.saveCalls[0].Name; got == nil || *got != "alice" {
					t.Errorf("saved name = %v", got)
				}
			case 400:
				if rec.Body.Len() != 0 {
					t.Errorf("expected empty body, got %q", rec.Body.String())
				}
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	tests := []struct {
		name       string
		result     []model.User
		err        error
		wantStatus int
		wantLen    int
	}{
		{name: "users exist", result: []model.User{*model.NewUserBuilder().ID(1).Name("a").Build(), *model.NewUserBuilder().ID(2).Name("b").Build()}, wantStatus: 200, wantLen: 2},
		{name: "nil result", result: nil, wantStatus: 200, wantLen: 0},
		{name: "empty result", result: []model.User{}, wantStatus: 200, wantLen: 0},
		{name: "error", err: errors.New("boom"), wantStatus: 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{listResult: tc.result, listErr: tc.err}
			h, logs := newServer(t, svc)
			rec := do(t, h, http.MethodGet, "/get_user_data", nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d", rec.Code)
			}
			if !strings.Contains(logs.String(), "inside the fetchUserList of UserController ") {
				t.Errorf("missing log")
			}
			if tc.wantStatus != 200 {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			body := strings.TrimSpace(rec.Body.String())
			if tc.wantLen == 0 && body != "[]" {
				t.Errorf("body = %q, want []", body)
			}
			var got []model.User
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("len = %d", len(got))
			}
			for i := range got {
				if !got[i].Equal(&tc.result[i]) {
					t.Errorf("user %d mismatch", i)
				}
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	user := model.NewUserBuilder().ID(5).Name("eve").Email("e@x.com").Build()
	tests := []struct {
		name       string
		path       string
		result     *model.User
		err        error
		wantStatus int
		wantCalled bool
		wantMsg    string
	}{
		{name: "found", path: "/get_user_data/5", result: user, wantStatus: 200, wantCalled: true},
		{name: "not found sentinel", path: "/get_user_data/9", err: service.ErrUserNotFound, wantStatus: 404, wantCalled: true, wantMsg: "User are not available"},
		{name: "not found custom", path: "/get_user_data/9", err: service.NewUserNotFoundError("User are not available"), wantStatus: 404, wantCalled: true, wantMsg: "User are not available"},
		{name: "non-integer", path: "/get_user_data/abc", wantStatus: 400},
		{name: "overflow int32", path: "/get_user_data/3000000000", wantStatus: 400},
		{name: "internal error", path: "/get_user_data/1", err: errors.New("db"), wantStatus: 500, wantCalled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{byIDResult: tc.result, byIDErr: tc.err}
			h, _ := newServer(t, svc)
			rec := do(t, h, http.MethodGet, tc.path, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if (len(svc.byIDCalls) > 0) != tc.wantCalled {
				t.Fatalf("called = %v", svc.byIDCalls)
			}
			switch tc.wantStatus {
			case 200:
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if !got.Equal(user) {
					t.Errorf("got %v", got.String())
				}
				if svc.byIDCalls[0] != 5 {
					t.Errorf("id = %d", svc.byIDCalls[0])
				}
			case 404:
				var m map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if m["message"] != tc.wantMsg {
					t.Errorf("message = %v", m["message"])
				}
				if m["status"] != "NOT_FOUND" {
					t.Errorf("status field = %v", m["status"])
				}
			case 400:
				if rec.Body.Len() != 0 {
					t.Errorf("body = %q", rec.Body.String())
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		err        error
		wantStatus int
		wantID     int
		wantCalled bool
	}{
		{name: "existing", path: "/delete_user_data/3", wantStatus: 200, wantID: 3, wantCalled: true},
		{name: "negative id", path: "/delete_user_data/-1", wantStatus: 200, wantID: -1, wantCalled: true},
		{name: "nonexistent", path: "/delete_user_data/77", err: errors.New("no rows deleted"), wantStatus: 500, wantID: 77, wantCalled: true},
		{name: "non-integer", path: "/delete_user_data/x1", wantStatus: 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{deleteErr: tc.err}
			h, _ := newServer(t, svc)
			rec := do(t, h, http.MethodDelete, tc.path, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d", rec.Code)
			}
			if (len(svc.deleteCalls) > 0) != tc.wantCalled {
				t.Fatalf("calls = %v", svc.deleteCalls)
			}
			if tc.wantCalled && svc.deleteCalls[0] != tc.wantID {
				t.Errorf("id = %d", svc.deleteCalls[0])
			}
			if tc.wantStatus == 200 {
				if rec.Body.String() != MsgUserDeleted {
					t.Errorf("body = %q", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
					t.Errorf("ct = %q", ct)
				}
			}
			if tc.wantStatus == 500 {
				var m map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if m["path"] != tc.path {
					t.Errorf("path = %v", m["path"])
				}
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		err        error
		wantStatus int
		wantCalled bool
		wantID     int
	}{
		{name: "valid", path: "/update_user_data/4", body: `{"id":99,"name":"zed","about":"hi"}`, wantStatus: 200, wantCalled: true, wantID: 4},
		{name: "blank name not validated", path: "/update_user_data/4", body: `{"name":""}`, wantStatus: 200, wantCalled: true, wantID: 4},
		{name: "malformed", path: "/update_user_data/4", body: `{bad`, wantStatus: 400},
		{name: "null body", path: "/update_user_data/4", body: `null`, wantStatus: 400},
		{name: "non-integer id", path: "/update_user_data/zz", body: `{"name":"a"}`, wantStatus: 400},
		{name: "service error", path: "/update_user_data/4", body: `{"name":"a"}`, err: errors.New("db"), wantStatus: 500, wantCalled: true, wantID: 4},
		{name: "service not found", path: "/update_user_data/4", body: `{"name":"a"}`, err: service.ErrUserNotFound, wantStatus: 404, wantCalled: true, wantID: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{updateErr: tc.err}
			h, _ := newServer(t, svc)
			rec := do(t, h, http.MethodPut, tc.path, strings.NewReader(tc.body))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d", rec.Code)
			}
			if (len(svc.updateIDs) > 0) != tc.wantCalled {
				t.Fatalf("calls = %v", svc.updateIDs)
			}
			if tc.wantCalled && svc.updateIDs[0] != tc.wantID {
				t.Errorf("id = %d", svc.updateIDs[0])
			}
			if tc.wantStatus == 200 {
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				var sent model.User
				_ = json.Unmarshal([]byte(tc.body), &sent)
				sent.ID = tc.wantID
				if !got.Equal(&sent) {
					t.Errorf("echo mismatch: %s vs %s", got.String(), sent.String())
				}
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	user := &model.User{ID: 2, Name: strp("john")}
	tests := []struct {
		name       string
		path       string
		result     *model.User
		found      bool
		err        error
		wantStatus int
		wantEmpty  bool
		wantName   string
	}{
		{name: "match", path: "/get_user_name/name/john", result: user, found: true, wantStatus: 200, wantName: "john"},
		{name: "no match", path: "/get_user_name/name/nobody", wantStatus: 200, wantEmpty: true, wantName: "nobody"},
		{name: "found but nil user", path: "/get_user_name/name/x", found: true, wantStatus: 200, wantEmpty: true, wantName: "x"},
		{name: "error", path: "/get_user_name/name/john", err: errors.New("db"), wantStatus: 500, wantName: "john"},
		{name: "escaped name", path: "/get_user_name/name/john%20doe", wantStatus: 200, wantEmpty: true, wantName: "john doe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{byName: tc.result, byNameFound: tc.found, byNameErr: tc.err}
			h, _ := newServer(t, svc)
			rec := do(t, h, http.MethodGet, tc.path, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d", rec.Code)
			}
			if len(svc.byNameCalls) != 1 || svc.byNameCalls[0] != tc.wantName {
				t.Fatalf("calls = %v", svc.byNameCalls)
			}
			if tc.wantEmpty {
				if rec.Body.Len() != 0 {
					t.Errorf("body = %q", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); ct != "" {
					t.Errorf("ct = %q", ct)
				}
				return
			}
			if tc.wantStatus == 200 {
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if !got.Equal(user) {
					t.Errorf("got %s", got.String())
				}
			}
		})
	}
}

func TestPathID(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"0", 0, false},
		{"42", 42, false},
		{"-7", -7, false},
		{"2147483647", 2147483647, false},
		{"2147483648", 0, true},
		{"", 0, true},
		{"1.5", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("id", tc.raw)
			got, err := pathID(req)
			if tc.wantErr {
				if !errors.Is(err, ErrBadRequest) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v", got, err)
			}
		})
	}
}

func TestUnregisteredMethod(t *testing.T) {
	svc := &fakeService{}
	h, _ := newServer(t, svc)
	rec := do(t, h, http.MethodGet, "/save_user_data", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(svc.saveCalls) != 0 {
		t.Fatal("service called")
	}
}