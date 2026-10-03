package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

type errorMessageJSON struct {
	Status  *string `json:"status"`
	Message *string `json:"message"`
}

func TestWriteError_UserNotFound(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"sentinel", service.ErrUserNotFound, "User are not available"},
		{"custom message", service.NewUserNotFoundError("User not found with id 5"), "User not found with id 5"},
		{"empty message uses default", service.NewUserNotFoundError(""), "User are not available"},
		{"wrapped with cause", service.WrapUserNotFound("", errors.New("db miss")), "db miss"},
		{"message and cause", service.WrapUserNotFound("missing 7", errors.New("x")), "missing 7"},
		{"fmt wrapped", fmt.Errorf("ctx: %w", service.NewUserNotFoundError("inner")), "inner"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/get_user_data/5", nil)
			WriteError(rec, req, tc.err)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			var body errorMessageJSON
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
			}
			if body.Status == nil || *body.Status != string(model.StatusNotFound) {
				t.Errorf("status field = %v", body.Status)
			}
			if body.Message == nil || *body.Message != tc.wantMsg {
				t.Errorf("message = %v, want %q", body.Message, tc.wantMsg)
			}
		})
	}
}

func TestWriteErrorMapping_OtherErrorKinds(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  int
		wantEmpty bool
	}{
		{"bad request", ErrBadRequest, http.StatusBadRequest, true},
		{"wrapped bad request", fmt.Errorf("%w: decode", ErrBadRequest), http.StatusBadRequest, true},
		{"unsupported media", ErrUnsupportedMediaType, http.StatusUnsupportedMediaType, true},
		{"generic", errors.New("boom"), http.StatusInternalServerError, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x/y", nil)
			WriteError(rec, req, tc.err)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantEmpty {
				if rec.Body.Len() != 0 {
					t.Errorf("expected empty body, got %q", rec.Body.String())
				}
				return
			}
			var b springErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if b.Status != 500 || b.Error != "Internal Server Error" || b.Path != "/x/y" || b.Timestamp == "" {
				t.Errorf("unexpected body %+v", b)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /missing", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, service.NewUserNotFoundError("nope"))
	})
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})
	h := Middleware(mux)

	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{"ok", "GET", "/users", 200, func(t *testing.T, rec *httptest.ResponseRecorder) {
			if rec.Body.String() != "ok" {
				t.Errorf("body %q", rec.Body.String())
			}
		}},
		{"trailing slash", "GET", "/users/", 200, nil},
		{"unmapped 404", "GET", "/nope", 404, func(t *testing.T, rec *httptest.ResponseRecorder) {
			var b springErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatalf("decode: %v body=%q", err, rec.Body.String())
			}
			if b.Status != 404 || b.Error != "Not Found" || b.Path != "/nope" {
				t.Errorf("body %+v", b)
			}
		}},
		{"method not allowed", "POST", "/users", 405, func(t *testing.T, rec *httptest.ResponseRecorder) {
			if rec.Body.Len() != 0 {
				t.Errorf("expected empty body, got %q", rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Allow"), "GET") {
				t.Errorf("Allow = %q", rec.Header().Get("Allow"))
			}
		}},
		{"user not found passes through", "GET", "/missing", 404, func(t *testing.T, rec *httptest.ResponseRecorder) {
			var b errorMessageJSON
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if b.Message == nil || *b.Message != "nope" || b.Status == nil || *b.Status != "NOT_FOUND" {
				t.Errorf("body %s", rec.Body.String())
			}
		}},
		{"panic", "GET", "/panic", 500, func(t *testing.T, rec *httptest.ResponseRecorder) {
			var b springErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if b.Status != 500 || b.Path != "/panic" {
				t.Errorf("body %+v", b)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d; body=%q", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}