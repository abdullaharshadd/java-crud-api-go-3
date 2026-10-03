package model

import (
	"net/http"
	"strings"
)

// HTTPStatus is the name of an HTTP status as Spring's HttpStatus enum
// serialises it (e.g. "NOT_FOUND"). Jackson writes enum names, not codes, so
// the JSON payload carries this string rather than the numeric status.
type HTTPStatus string

// Commonly used HTTPStatus values.
const (
	StatusBadRequest          HTTPStatus = "BAD_REQUEST"
	StatusNotFound            HTTPStatus = "NOT_FOUND"
	StatusInternalServerError HTTPStatus = "INTERNAL_SERVER_ERROR"
)

// HTTPStatusFromCode converts a numeric HTTP status code into the Spring
// HttpStatus enum name. It reports false when the code is unknown.
func HTTPStatusFromCode(code int) (HTTPStatus, bool) {
	// Spring names that do not follow the "upper-case StatusText" rule.
	switch code {
	case http.StatusTeapot:
		return "I_AM_A_TEAPOT", true
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE", true
	case http.StatusRequestURITooLong:
		return "URI_TOO_LONG", true
	case http.StatusRequestedRangeNotSatisfiable:
		return "REQUESTED_RANGE_NOT_SATISFIABLE", true
	case http.StatusHTTPVersionNotSupported:
		return "HTTP_VERSION_NOT_SUPPORTED", true
	case http.StatusNonAuthoritativeInfo:
		return "NON_AUTHORITATIVE_INFORMATION", true
	}
	text := http.StatusText(code)
	if text == "" {
		return "", false
	}
	r := strings.NewReplacer(" ", "_", "-", "_", "'", "")
	return HTTPStatus(strings.ToUpper(r.Replace(text))), true
}

// ErrorMessage is the JSON error payload returned by the exception handler,
// e.g. {"status":"NOT_FOUND","message":"User are not available"}.
//
// Fields are pointers so that an unset value serialises as JSON null, exactly
// like the Java null references of the Lombok source.
type ErrorMessage struct {
	Status  *HTTPStatus `json:"status"`
	Message *string     `json:"message"`
}

// NewEmptyErrorMessage returns an ErrorMessage with status and message unset
// (Lombok @NoArgsConstructor).
func NewEmptyErrorMessage() *ErrorMessage {
	return &ErrorMessage{}
}

// NewErrorMessage returns an ErrorMessage with the given status and message,
// in that order (Lombok @AllArgsConstructor).
func NewErrorMessage(status HTTPStatus, message string) *ErrorMessage {
	return &ErrorMessage{Status: &status, Message: &message}
}

// GetStatus returns the status (nil when unset).
func (e *ErrorMessage) GetStatus() *HTTPStatus { return e.Status }

// SetStatus sets the status; pass nil to unset it.
func (e *ErrorMessage) SetStatus(status *HTTPStatus) { e.Status = status }

// GetMessage returns the message (nil when unset).
func (e *ErrorMessage) GetMessage() *string { return e.Message }

// SetMessage sets the message; pass nil to unset it.
func (e *ErrorMessage) SetMessage(message *string) { e.Message = message }

// Equal reports value equality over status and message (Lombok @Data).
func (e *ErrorMessage) Equal(o *ErrorMessage) bool {
	if e == nil || o == nil {
		return e == o
	}
	return statusPtrEqual(e.Status, o.Status) && strPtrEqual(e.Message, o.Message)
}

// String renders the value in Lombok's toString format, e.g.
// ErrorMessage(status=NOT_FOUND, message=User are not available).
func (e *ErrorMessage) String() string {
	if e == nil {
		return "null"
	}
	status := "null"
	if e.Status != nil {
		status = string(*e.Status)
	}
	return "ErrorMessage(status=" + status + ", message=" + strPtr(e.Message) + ")"
}

func statusPtrEqual(a, b *HTTPStatus) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
