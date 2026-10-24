# PaaS HTTP runtime profile

This document records the HTTP runtime behavior available to Flux-generated
PaaS services through `middlewarex/httpx`. Flux and the application compose
these existing mechanisms; this profile does not add a new library API.

## JSON request bodies

`JSONDecoder[T](DecodeOptions{})` rejects an empty body, decodes one JSON value,
and permits unknown object fields by default, matching `encoding/json`'s normal
field handling. It rejects a second JSON value and malformed trailing data;
trailing JSON whitespace is accepted. `DisallowUnknownFields` enables the
standard decoder's unknown-field rejection. `RequireObject` rejects `null`,
arrays, and scalar top-level values before decoding.

`AllowEmptyBody` accepts only a body with no bytes and returns the zero value of
`T`. Whitespace-only input is still invalid JSON. `DecodeJSON[T]` retains its
legacy behavior of allowing an actually empty body. Empty-body detection reads
the body stream; `Request.ContentLength` is metadata, not a guarantee about a
manually constructed or wrapped request's stream. For client requests, zero
can mean an unknown body length; for server requests, zero means no body.

Flux may normalize an absent body to `{}` before invoking the decoder. That
creates a JSON object and can invoke a DTO's custom `UnmarshalJSON`; it is not
equivalent to `AllowEmptyBody`, which returns the Go zero value without calling
`UnmarshalJSON`. Keep the generated normalization behavior when DTO semantics
depend on it. A custom `UnmarshalJSON` owns validation of its own JSON contents;
`DisallowUnknownFields` cannot inspect fields hidden behind that method.

## Body limits and errors

Place `LimitBody(maxBytes)` before middleware or handlers that consume the
request body. A read exceeding the limit is classified as payload too large and
maps to HTTP 413. The same configured status mapper and error encoder can be
passed to the generated handler runtime and to outer `Adapt` middleware so
errors raised before JSON decoding use the same response format.

`JSONErrorEncoder` emits a JSON `error` field using an explicitly configured
public message, a safe HTTP status phrase for client errors, or the generic
`internal server error` for 5xx responses. It does not include the underlying
error text. The default status mapper classifies bad requests as 400, oversized
bodies as 413, timeouts as 503, and other errors as 500. Applications must
choose public messages and configure the same encoder at each applicable
adapter boundary.

## Request IDs and logging

`RequestID` reads the configured request header or generates an ID, stores it
in request context, and returns it in the response header. The logging
middleware and runtime context logger include a request ID found in that
context. Applications must compose these middleware/runtime options and decide
their logging and redaction policy; the helpers do not automatically enable
HTTP request logging in every handler.

## Integration responsibilities

The library supplies body decoding/limits, error classification and encoding,
request ID propagation, and logging hooks. The application remains responsible
for middleware composition, request-size values, error messages, logger setup,
redaction, and transport/server configuration. Flux generates handlers from
the API contract; it does not select monitoring, deploy a server, or establish
the PaaS daemon's logging policy.
