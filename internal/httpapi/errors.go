// Package httpapi contains the HTTP layer of the smart-contact service:
// handlers, middleware and the central error-to-response mapping that
// replaces Spring's @ControllerAdvice / ResponseEntityExceptionHandler.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

// Sentinel errors that handlers return for the standard framework failures
// Spring's ResponseEntityExceptionHandler handled implicitly. Spring answers
// these with the status code and an empty body.
var (
	// ErrBadRequest covers validation failures, unreadable JSON and
	// path variables that cannot be converted (Spring: 400, empty body).
	ErrBadRequest = errors.New("httpapi: bad request")
	// ErrUnsupportedMediaType covers a request body with a Content-Type the
	// endpoint cannot consume (Spring: 415, empty body).
	ErrUnsupportedMediaType = errors.New("httpapi: unsupported media type")
)

// springTimestampLayout matches Jackson's default rendering of the
// timestamp in Spring Boot's /error body, e.g. 2024-01-02T03:04:05.678+00:00.
const springTimestampLayout = "2006-01-02T15:04:05.000-07:00"

// springErrorBody is the JSON body Spring Boot's BasicErrorController
// produces for unhandled errors and unmapped paths.
type springErrorBody struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Path      string `json:"path"`
}

// WriteError maps err to an HTTP response. It is the Go equivalent of the
// source's global exception handler:
//
//   - service.ErrUserNotFound (any *service.UserNotFoundError) -> 404 with
//     {"status":"NOT_FOUND","message":<error message>}
//   - ErrBadRequest           -> 400, empty body
//   - ErrUnsupportedMediaType -> 415, empty body
//   - anything else           -> 500 with Spring's /error JSON; the error is logged.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, service.ErrUserNotFound):
		// The exception's own message is used, as exception.getMessage() was.
		msg := err.Error()
		var unf *service.UserNotFoundError
		if errors.As(err, &unf) {
			msg = unf.Error()
		}
		writeJSON(w, http.StatusNotFound, model.NewErrorMessage(model.StatusNotFound, msg))
	case errors.Is(err, ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
	case errors.Is(err, ErrUnsupportedMediaType):
		w.WriteHeader(http.StatusUnsupportedMediaType)
	default:
		log.Error().Err(err).Str("method", r.Method).Str("path", r.URL.Path).Msg("unhandled request error")
		writeSpringError(w, r, http.StatusInternalServerError)
	}
}

// writeSpringError writes Spring Boot's default /error JSON body.
func writeSpringError(w http.ResponseWriter, r *http.Request, code int) {
	writeJSON(w, code, springErrorBody{
		Timestamp: time.Now().Format(springTimestampLayout),
		Status:    code,
		Error:     http.StatusText(code),
		Path:      r.URL.Path,
	})
}

// writeJSON serialises v with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("encode JSON response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(body); err != nil {
		log.Debug().Err(err).Msg("write JSON response")
	}
}

// Middleware wraps next with Spring-compatible cross-cutting behaviour:
// trailing-slash tolerance, panic recovery (500 /error JSON), Spring's 404
// /error JSON for unmapped paths and an empty-body 405 that keeps the Allow
// header. Wire it around the router in cmd/server: srv.Handler = httpapi.Middleware(mux).
func Middleware(next http.Handler) http.Handler {
	return trimTrailingSlash(recoverer(springStatusRewriter(next)))
}

// trimTrailingSlash makes "/users/" match "/users", as Spring MVC 5's
// trailing-slash matching did.
func trimTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; len(p) > 1 && strings.HasSuffix(p, "/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimRight(p, "/")
			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}
			r2.URL.RawPath = ""
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}

// recoverer turns a handler panic into the 500 /error JSON.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				WriteError(w, r, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// springStatusRewriter replaces the plain-text 404/405 bodies produced by
// http.ServeMux with Spring's responses: 404 -> /error JSON, 405 -> empty
// body (Allow header preserved). JSON responses written by handlers (e.g.
// the user-not-found 404) are passed through untouched.
func springStatusRewriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&statusRewriter{ResponseWriter: w, r: r}, r)
	})
}

type statusRewriter struct {
	http.ResponseWriter
	r           *http.Request
	wroteHeader bool
	swallow     bool
}

func (s *statusRewriter) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.wroteHeader = true
	plain := strings.HasPrefix(s.Header().Get("Content-Type"), "text/plain")
	switch {
	case plain && code == http.StatusNotFound:
		s.swallow = true
		s.Header().Del("Content-Type")
		s.Header().Del("X-Content-Type-Options")
		writeSpringError(s.ResponseWriter, s.r, http.StatusNotFound)
	case plain && code == http.StatusMethodNotAllowed:
		s.swallow = true
		s.Header().Del("Content-Type")
		s.Header().Del("X-Content-Type-Options")
		s.ResponseWriter.WriteHeader(http.StatusMethodNotAllowed)
	default:
		s.ResponseWriter.WriteHeader(code)
	}
}

func (s *statusRewriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	if s.swallow {
		return len(b), nil
	}
	return s.ResponseWriter.Write(b)
}
