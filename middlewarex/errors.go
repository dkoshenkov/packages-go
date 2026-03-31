package middlewarex

import (
	"errors"
	"fmt"

	"github.com/dkoshenkov/packages-go/consterr"
)

const (
	errIdentityMissing    = consterr.Error("identity is missing")
	errRolesMissing       = consterr.Error("required role is missing")
	errScopesMissing      = consterr.Error("required scope is missing")
	errLoggerIsNil        = consterr.Error("logger must not be nil")
	errTimeoutNonPositive = consterr.Error("timeout must be greater than zero")
	errRecoveredPanic     = consterr.Error("panic recovered")
)

// ErrorKind groups errors for status mapping and policy decisions.
type ErrorKind string

const (
	ErrorKindUnauthorized   ErrorKind = "unauthorized"
	ErrorKindForbidden      ErrorKind = "forbidden"
	ErrorKindBadRequest     ErrorKind = "bad_request"
	ErrorKindMethodNotAllow ErrorKind = "method_not_allowed"
	ErrorKindTimeout        ErrorKind = "timeout"
	ErrorKindInternal       ErrorKind = "internal"
)

var (
	errUnauthorized   = &Error{kind: ErrorKindUnauthorized}
	errForbidden      = &Error{kind: ErrorKindForbidden}
	errBadRequest     = &Error{kind: ErrorKindBadRequest}
	errMethodNotAllow = &Error{kind: ErrorKindMethodNotAllow}
	errTimeout        = &Error{kind: ErrorKindTimeout}
	errInternal       = &Error{kind: ErrorKindInternal}
)

// Error classifies middleware failure.
type Error struct {
	kind ErrorKind
	err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.err == nil {
		return string(e.kind)
	}
	return fmt.Sprintf("%s: %v", e.kind, e.err)
}

// Unwrap returns wrapped error.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// Is compares errors by kind.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.kind == other.kind
}

// Kind returns error classification kind.
func (e *Error) Kind() ErrorKind {
	if e == nil {
		return ""
	}
	return e.kind
}

func classify(kind *Error, err error) error {
	if kind == nil {
		return err
	}
	if err == nil {
		return &Error{kind: kind.kind}
	}
	return &Error{kind: kind.kind, err: err}
}

// Unauthorized classifies err as unauthorized.
func Unauthorized(err error) error {
	return classify(errUnauthorized, err)
}

// Forbidden classifies err as forbidden.
func Forbidden(err error) error {
	return classify(errForbidden, err)
}

// BadRequest classifies err as bad request.
func BadRequest(err error) error {
	return classify(errBadRequest, err)
}

// MethodNotAllowed classifies err as method not allowed.
func MethodNotAllowed(err error) error {
	return classify(errMethodNotAllow, err)
}

// TimeoutError classifies err as timeout.
func TimeoutError(err error) error {
	return classify(errTimeout, err)
}

// Internal classifies err as internal.
func Internal(err error) error {
	return classify(errInternal, err)
}

// IsUnauthorized reports whether err is unauthorized.
func IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
}

// IsForbidden reports whether err is forbidden.
func IsForbidden(err error) bool {
	return errors.Is(err, errForbidden)
}

// IsBadRequest reports whether err is bad request.
func IsBadRequest(err error) bool {
	return errors.Is(err, errBadRequest)
}

// IsMethodNotAllowed reports whether err is method not allowed.
func IsMethodNotAllowed(err error) bool {
	return errors.Is(err, errMethodNotAllow)
}

// IsTimeout reports whether err is timeout.
func IsTimeout(err error) bool {
	return errors.Is(err, errTimeout)
}

// IsInternal reports whether err is internal.
func IsInternal(err error) bool {
	return errors.Is(err, errInternal)
}
