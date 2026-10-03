package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

// Plain-text confirmation bodies returned by the source controller.
const (
	// MsgUserSaved is returned by POST /save_user_data on success.
	MsgUserSaved = "User data saved successfully!"
	// MsgUserDeleted is returned by DELETE /delete_user_data/{id} on success.
	MsgUserDeleted = "user data deleted Successfully"
)

// Handler exposes the user CRUD endpoints. It replaces the Spring
// UserController.
type Handler struct {
	svc service.UserService
	log zerolog.Logger
}

// NewHandler returns a Handler that delegates to svc and logs with log.
func NewHandler(svc service.UserService, log zerolog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register registers every user route on mux (Go 1.22 method+pattern
// routing). Wrap the mux with Middleware to get Spring's trailing-slash
// matching and empty-body 404/405 behaviour.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /save_user_data", h.SaveUser)
	mux.HandleFunc("GET /get_user_data", h.FetchUserList)
	mux.HandleFunc("GET /get_user_data/{id}", h.FetchUserByID)
	mux.HandleFunc("DELETE /delete_user_data/{id}", h.DeleteUser)
	mux.HandleFunc("PUT /update_user_data/{id}", h.UpdateUser)
	mux.HandleFunc("GET /get_user_name/name/{name}", h.GetUserNameByName)
}

// SaveUser handles POST /save_user_data. The body must be valid JSON and
// pass model.User.Validate (the @Valid constraints); otherwise 400 with an
// empty body.
func (h *Handler) SaveUser(w http.ResponseWriter, r *http.Request) {
	h.log.Info().Msg("inside the saveUser of UserController ")
	user, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := user.Validate(); err != nil {
		WriteError(w, r, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return
	}
	if _, err := h.svc.SaveUser(r.Context(), user); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, MsgUserSaved)
}

// FetchUserList handles GET /get_user_data and returns all users as a JSON
// array ("[]" when empty).
func (h *Handler) FetchUserList(w http.ResponseWriter, r *http.Request) {
	h.log.Info().Msg("inside the fetchUserList of UserController ")
	users, err := h.svc.FetchUserList(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if users == nil {
		users = []model.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

// FetchUserByID handles GET /get_user_data/{id}. A missing user yields the
// 404 ErrorMessage body via WriteError.
func (h *Handler) FetchUserByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	user, err := h.svc.FetchUserByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// DeleteUser handles DELETE /delete_user_data/{id}.
//
// MIGRATION_NOTE: deleting a nonexistent id surfaces the store error as a
// 500, matching Spring Data 2.x's EmptyResultDataAccessException (quirk kept).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := h.svc.DeleteUser(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, MsgUserDeleted)
}

// UpdateUser handles PUT /update_user_data/{id}. No validation is applied
// (the source has no @Valid). The response echoes the request body with the
// path id applied, not the persisted entity.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	user, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := h.svc.UpdateUser(r.Context(), id, user); err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// GetUserNameByName handles GET /get_user_name/name/{name}. When no user
// matches, the source returned null, which Spring renders as 200 with an
// empty body and no Content-Type.
func (h *Handler) GetUserNameByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	user, found, err := h.svc.GetUserNameByName(r.Context(), name)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if !found || user == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// pathID parses the {id} path variable as a 32-bit signed int, as Spring's
// int @PathVariable conversion did. Failure maps to ErrBadRequest (400).
func pathID(r *http.Request) (int, error) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid id %q: %v", ErrBadRequest, raw, err)
	}
	return int(id), nil
}

// decodeUser leniently decodes the JSON request body (unknown fields are
// ignored, as Jackson was configured by Spring Boot). A missing, malformed
// or null body maps to ErrBadRequest.
func decodeUser(r *http.Request) (*model.User, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("%w: missing request body", ErrBadRequest)
	}
	var user *model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("%w: decode user: %v", ErrBadRequest, err)
	}
	if user == nil {
		return nil, fmt.Errorf("%w: null request body", ErrBadRequest)
	}
	return user, nil
}

// writeText writes a text/plain body, as Spring's StringHttpMessageConverter did.
func writeText(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}
