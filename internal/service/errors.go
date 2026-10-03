// Package service contains the business logic of the smart-contact
// application together with the domain errors it reports.
package service

import "errors"

// userNotFoundMessage is the exact message the source used when a user lookup
// failed. Its grammar is kept as-is because it ends up in the 404 response body.
const userNotFoundMessage = "User are not available"

// ErrUserNotFound means the requested user does not exist.
// Check for it with errors.Is. HTTP handlers turn it into a 404 response.
var ErrUserNotFound = errors.New(userNotFoundMessage)

// UserNotFoundError is a "user not found" error that can carry its own
// message and/or an underlying cause. errors.Is(err, ErrUserNotFound)
// reports true for any *UserNotFoundError, so callers only need the sentinel.
//
// MIGRATION_NOTE: The Java UserNotFoundException had five constructors.
// They map to the following:
//   - ()                    -> ErrUserNotFound (or NewUserNotFoundError(""))
//   - (message)             -> NewUserNotFoundError(message)
//   - (message, cause)      -> WrapUserNotFound(message, cause)
//   - (cause)               -> WrapUserNotFound("", cause)
//   - protected (message, cause, enableSuppression, writableStackTrace):
//     Go errors have no suppression list or stack trace, so the two flags have
//     nothing to control. Use WrapUserNotFound(message, cause).
type UserNotFoundError struct {
	msg   string
	cause error
}

// NewUserNotFoundError returns a user-not-found error with the given detail
// message. If message is empty, the default message is used.
func NewUserNotFoundError(message string) *UserNotFoundError {
	return &UserNotFoundError{msg: message}
}

// WrapUserNotFound returns a user-not-found error with an optional detail
// message that wraps cause. If message is empty and cause is not nil, the
// cause's text becomes the message. Java's Throwable(cause) constructor
// behaves the same way.
func WrapUserNotFound(message string, cause error) *UserNotFoundError {
	return &UserNotFoundError{msg: message, cause: cause}
}

// Error implements the error interface.
func (e *UserNotFoundError) Error() string {
	switch {
	case e.msg != "":
		return e.msg
	case e.cause != nil:
		return e.cause.Error()
	default:
		return userNotFoundMessage
	}
}

// Unwrap returns the underlying cause, if any.
func (e *UserNotFoundError) Unwrap() error { return e.cause }

// Is makes errors.Is(err, ErrUserNotFound) match any *UserNotFoundError.
func (e *UserNotFoundError) Is(target error) bool { return target == ErrUserNotFound }
