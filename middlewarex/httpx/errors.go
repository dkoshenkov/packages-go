package httpx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errVerifierIsNil         = consterr.Error("verifier must not be nil")
	errAuthorizationMissing  = consterr.Error("authorization header is missing")
	errAuthorizationInvalid  = consterr.Error("authorization header must be Bearer <token>")
	errHeaderNameEmpty       = consterr.Error("header name must not be empty")
	errHeaderValueMissing    = consterr.Error("required header value is missing")
	errMethodMissing         = consterr.Error("request method is not allowed")
	errRequestIDGeneratorNil = consterr.Error("request ID generator must not be nil")
)
