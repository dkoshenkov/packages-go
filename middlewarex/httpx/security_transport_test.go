package httpx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

type decodeRequest struct {
	Value int `json:"value"`
}

type customJSONValue struct{ Called bool }

func (v *customJSONValue) UnmarshalJSON(data []byte) error {
	v.Called = string(data) == `{"value":1}`
	return nil
}

func TestJSONDecoderOptions(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		options DecodeOptions
		wantErr bool
	}{
		{name: "empty allowed", body: "", options: DecodeOptions{AllowEmptyBody: true}},
		{name: "whitespace is not empty", body: " \n\t", options: DecodeOptions{AllowEmptyBody: true}, wantErr: true},
		{name: "object", body: `{"value":1}`, options: DecodeOptions{RequireObject: true}},
		{name: "null", body: `null`, options: DecodeOptions{RequireObject: true}, wantErr: true},
		{name: "array", body: `[]`, options: DecodeOptions{RequireObject: true}, wantErr: true},
		{name: "scalar", body: `1`, options: DecodeOptions{RequireObject: true}, wantErr: true},
		{name: "unknown field allowed", body: `{"other":1}`, options: DecodeOptions{RequireObject: true}},
		{name: "unknown field rejected", body: `{"other":1}`, options: DecodeOptions{RequireObject: true, DisallowUnknownFields: true}, wantErr: true},
		{name: "second value rejected", body: `{"value":1} {"value":2}`, options: DecodeOptions{RequireObject: true}, wantErr: true},
		{name: "trailing whitespace", body: "{\"value\":1} \n", options: DecodeOptions{RequireObject: true}},
		{name: "malformed", body: `{"value":`, options: DecodeOptions{RequireObject: true}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			_, err := JSONDecoder[decodeRequest](tt.options)(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}

	legacy, err := DecodeJSON[decodeRequest](httptest.NewRequest(http.MethodPost, "/", nil))
	if err != nil || legacy != (decodeRequest{}) {
		t.Fatalf("legacy empty body behavior changed: value=%+v error=%v", legacy, err)
	}

	custom, err := JSONDecoder[customJSONValue](DecodeOptions{RequireObject: true})(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":1}`)))
	if err != nil {
		t.Fatal(err)
	}
	if !custom.Called {
		t.Fatal("custom UnmarshalJSON was not called")
	}
}

func TestLimitBodyReturns413OverRealHTTP(t *testing.T) {
	var calls atomic.Int32
	mapper := StatusMapperFunc(DefaultStatusMapper)
	encoder := JSONErrorEncoder(mapper, map[int]string{http.StatusRequestEntityTooLarge: "payload too large"})
	inner := newLimitedJSONHandler(&calls, mapper, encoder)
	handler := Adapt(Chain(FromHTTP(inner), LimitBody(11)), WithStatusMapper(mapper), WithErrorEncoder(encoder))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sandbox does not allow a loopback HTTP server: %v", err)
	}
	server := &httptest.Server{Config: &http.Server{Handler: handler}, Listener: listener}
	server.Start()
	defer server.Close()

	for _, tt := range []struct {
		name          string
		body          string
		unknownLength bool
		wantStatus    int
	}{
		{name: "exact limit", body: `{"value":1}`, wantStatus: http.StatusOK},
		{name: "one over", body: `{"value":1} `, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "unknown length over", body: `{"value":1} `, unknownLength: true, wantStatus: http.StatusRequestEntityTooLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if tt.unknownLength {
				request.Body = io.NopCloser(strings.NewReader(tt.body))
				request.ContentLength = -1
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.wantStatus {
				payload, _ := io.ReadAll(response.Body)
				t.Fatalf("status=%d body=%s", response.StatusCode, payload)
			}
			if tt.wantStatus == http.StatusRequestEntityTooLarge {
				payload, _ := io.ReadAll(response.Body)
				if !strings.Contains(string(payload), `"error":"payload too large"`) {
					t.Fatalf("error payload=%s", payload)
				}
			}
		})
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("service calls=%d, want exactly one successful request", got)
	}
}

func TestLimitBodyClassifiesOversizeWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	mapper := StatusMapperFunc(DefaultStatusMapper)
	encoder := JSONErrorEncoder(mapper, nil)
	inner := newLimitedJSONHandler(&calls, mapper, encoder)
	handler := Adapt(Chain(FromHTTP(inner), LimitBody(11)), WithStatusMapper(mapper), WithErrorEncoder(encoder))

	for _, tt := range []struct {
		body       string
		wantStatus int
	}{
		{body: `{"value":1}`, wantStatus: http.StatusOK},
		{body: `{"value":1} `, wantStatus: http.StatusRequestEntityTooLarge},
	} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
		r.ContentLength = -1
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tt.wantStatus {
			t.Fatalf("body=%q status=%d want=%d response=%q", tt.body, w.Code, tt.wantStatus, w.Body.String())
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("service calls=%d, want exactly one successful request", got)
	}
}

func TestLimitBodyOuterMiddlewareUsesSharedJSONErrorEncoder(t *testing.T) {
	mapper := StatusMapperFunc(DefaultStatusMapper)
	encoder := JSONErrorEncoder(mapper, map[int]string{http.StatusRequestEntityTooLarge: "payload too large"})
	handler := Adapt(Chain(FromHTTPFunc(func(_ http.ResponseWriter, r *http.Request) error {
		_, err := io.ReadAll(r.Body)
		return err
	}), LimitBody(4)), WithStatusMapper(mapper), WithErrorEncoder(encoder))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	r.ContentLength = -1
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), `"error":"payload too large"`) {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func newLimitedJSONHandler(calls *atomic.Int32, mapper StatusMapper, encoder ErrorEncoder) http.Handler {
	return JSON(func(_ context.Context, req decodeRequest) (Response[decodeRequest], error) {
		calls.Add(1)
		return OK(req), nil
	},
		WithDecoder[decodeRequest, decodeRequest](JSONDecoder[decodeRequest](DecodeOptions{RequireObject: true})),
		WithStatusMapper(mapper),
		WithErrorEncoder(encoder),
	)
}

func TestJSONErrorEncoderDoesNotLeakInternalError(t *testing.T) {
	mapper := StatusMapperFunc(func(error) int { return http.StatusServiceUnavailable })
	encoder := JSONErrorEncoder(mapper, nil)
	w := httptest.NewRecorder()
	encoder.Encode(w, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("filesystem secret path"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Body.String(); got != "{\"error\":\"internal server error\"}\n" {
		t.Fatalf("body=%q", got)
	}
}

func TestAdaptDoesNotEncodeAfterResponseWasWritten(t *testing.T) {
	encoder := JSONErrorEncoder(StatusMapperFunc(DefaultStatusMapper), nil)
	handler := Adapt(FromHTTPFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusAccepted)
		return errors.New("late error")
	}), WithErrorEncoder(encoder))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthCookieAndBearerPrecedence(t *testing.T) {
	var verified []string
	verifier := verifierFunc(func(_ context.Context, token string) (middlewarex.Identity, error) {
		verified = append(verified, token)
		if token == "bad" {
			return middlewarex.Identity{}, errors.New("invalid")
		}
		return middlewarex.Identity{Subject: token}, nil
	})
	handler := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source, ok := AuthSourceFromContext(r.Context())
		if !ok {
			http.Error(w, "missing source", http.StatusInternalServerError)
			return
		}
		identity, _ := middlewarex.IdentityFromContext(r.Context())
		_, _ = w.Write([]byte(string(source) + ":" + identity.Subject))
	})), Auth(verifier,
		WithAuthCookie("sid"),
		WithAuthPriority(AuthSourceCookie, AuthSourceBearer),
	)))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "sid", Value: "cookie-token"})
	request.Header.Set("Authorization", "Bearer bearer-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusOK || w.Body.String() != "cookie:cookie-token" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "sid", Value: "bad"})
	request.Header.Set("Authorization", "Bearer bearer-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid selected cookie fell back: status=%d", w.Code)
	}
	if len(verified) != 2 || verified[0] != "cookie-token" || verified[1] != "bad" {
		t.Fatalf("verified tokens=%v", verified)
	}
}

func TestCSRFMiddlewareOriginFetchMetadataAndBearer(t *testing.T) {
	makeHandler := func() http.Handler {
		return Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})), CSRF(CSRFConfig{
			TrustedOrigins:     []string{"https://ui.example"},
			CheckFetchMetadata: true,
			RequireOrigin:      true,
		})))
	}
	handler := makeHandler()
	for _, tt := range []struct {
		name       string
		origin     string
		fetchSite  string
		method     string
		wantStatus int
	}{
		{name: "same origin", origin: "https://api.example", fetchSite: "same-origin", method: http.MethodPost, wantStatus: http.StatusNoContent},
		{name: "trusted origin", origin: "https://ui.example", fetchSite: "cross-site", method: http.MethodPost, wantStatus: http.StatusNoContent},
		{name: "untrusted origin", origin: "https://evil.example", fetchSite: "same-site", method: http.MethodPost, wantStatus: http.StatusForbidden},
		{name: "missing origin", fetchSite: "same-origin", method: http.MethodPost, wantStatus: http.StatusForbidden},
		{name: "cross-site metadata", origin: "https://api.example", fetchSite: "cross-site", method: http.MethodPost, wantStatus: http.StatusForbidden},
		{name: "safe method", method: http.MethodGet, wantStatus: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "https://api.example/resource", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d want=%d body=%q", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}

	bearerHandler := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), Auth(verifierFunc(func(_ context.Context, _ string) (middlewarex.Identity, error) {
		return middlewarex.Identity{Subject: "u"}, nil
	})), CSRF(CSRFConfig{RequireOrigin: true, AllowBearer: true})))
	r := httptest.NewRequest(http.MethodPost, "https://api.example/resource", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	bearerHandler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("verified bearer should bypass CSRF: status=%d body=%q", w.Code, w.Body.String())
	}

	cookieFirst := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), Auth(verifierFunc(func(_ context.Context, token string) (middlewarex.Identity, error) {
		return middlewarex.Identity{Subject: token}, nil
	}), WithAuthCookie("sid"), WithAuthPriority(AuthSourceCookie, AuthSourceBearer)), CSRF(CSRFConfig{
		RequireOrigin: true,
		AllowBearer:   true,
	})))
	r = httptest.NewRequest(http.MethodPost, "https://api.example/resource", nil)
	r.AddCookie(&http.Cookie{Name: "sid", Value: "cookie-token"})
	r.Header.Set("Authorization", "Bearer bearer-token")
	w = httptest.NewRecorder()
	cookieFirst.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cookie-first auth must not infer bearer CSRF exemption: status=%d body=%q", w.Code, w.Body.String())
	}
}
