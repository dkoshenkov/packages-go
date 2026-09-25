package httpx

import "github.com/dkoshenkov/packages-go/consterr"

const (
	errVerifierIsNil         = consterr.Error("verifier must not be nil")
	errAuthorizationMissing  = consterr.Error("authorization header is missing")
	errAuthorizationInvalid  = consterr.Error("authorization header must be Bearer <token>")
	errHeaderNameEmpty       = consterr.Error("header name must not be empty")
	errHeaderValueMissing    = consterr.Error("required header value is missing")
	errMethodMissing         = consterr.Error("request method is not allowed")
	errBodyLimitNonPositive  = consterr.Error("body limit must be greater than zero")
	errAuthCookieNameEmpty   = consterr.Error("auth cookie name must not be empty")
	errAuthSourceInvalid     = consterr.Error("auth source is invalid")
	errCSRFOriginMissing     = consterr.Error("origin header is required")
	errCSRFOriginInvalid     = consterr.Error("origin is not trusted")
	errCSRFFetchSiteInvalid  = consterr.Error("fetch metadata indicates a cross-site request")
	errRequestIDGeneratorNil = consterr.Error("request ID generator must not be nil")
	errJSONBodyMustBeObject  = consterr.Error("request body must contain a JSON object")
	errJSONSingleValue       = consterr.Error("request body must contain a single JSON value")
)
