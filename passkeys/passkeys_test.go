package passkeys

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

var testTime = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func testRP(t testing.TB) *RP {
	t.Helper()
	rp, err := New(Config{RPID: "example.com", RPName: "Example", Origins: []string{"https://example.com"}, Now: func() time.Time { return testTime }})
	if err != nil {
		t.Fatal(err)
	}
	return rp
}
func jsonBytes(t testing.TB, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func cborBytes(t testing.TB, v any) []byte {
	t.Helper()
	b, e := cbor.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func unbase(t testing.TB, v string) []byte {
	t.Helper()
	b, e := decode(v, maxResponse)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func document(t testing.TB, data []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if e := json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func responseMap(t testing.TB, data []byte) (map[string]any, map[string]any) {
	t.Helper()
	v := document(t, data)
	r, ok := v["response"].(map[string]any)
	if !ok {
		t.Fatal("response")
	}
	return v, r
}
func envelopeBytes(t testing.TB, id []byte, response map[string]any) []byte {
	return jsonBytes(t, map[string]any{"id": encode(id), "rawId": encode(id), "type": "public-key", "response": response, "clientExtensionResults": map[string]any{}})
}
func clientBytes(t testing.TB, state State) []byte {
	return jsonBytes(t, map[string]any{"type": state.Operation, "challenge": encode(state.Challenge), "origin": "https://example.com"})
}
func keyPair(t testing.TB) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key, cborBytes(t, map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
}
func authBytes(flags byte, count uint32) []byte {
	hash := sha256.Sum256([]byte("example.com"))
	b := append([]byte(nil), hash[:]...)
	b = append(b, flags, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(b[33:], count)
	return b
}
func registerResponse(t testing.TB, state State, id, key []byte, flags byte) []byte {
	auth := authBytes(flags, 0)
	auth = append(auth, make([]byte, 16)...)
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(id)))
	auth = append(auth, id...)
	auth = append(auth, key...)
	return envelopeBytes(t, id, map[string]any{"clientDataJSON": encode(clientBytes(t, state)), "attestationObject": encode(cborBytes(t, map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": auth})), "transports": []string{"internal"}})
}
func assertion(t testing.TB, state State, cred Credential, key *ecdsa.PrivateKey, flags byte, count uint32) []byte {
	return signedResponse(t, state, cred, key, clientBytes(t, state), authBytes(flags, count))
}
func signedResponse(t testing.TB, state State, cred Credential, key *ecdsa.PrivateKey, client, auth []byte) []byte {
	t.Helper()
	h := sha256.Sum256(client)
	b := append(append([]byte(nil), auth...), h[:]...)
	digest := sha256.Sum256(b)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return envelopeBytes(t, cred.ID, map[string]any{"clientDataJSON": encode(client), "authenticatorData": encode(auth), "signature": encode(sig), "userHandle": encode(cred.UserID)})
}
func registration(t testing.TB, rp *RP) (State, []byte, *ecdsa.PrivateKey) {
	t.Helper()
	_, s, e := rp.BeginRegistration(RegistrationRequest{FlowID: "flow", User: User{ID: []byte("user"), Name: "user", DisplayName: "User"}})
	if e != nil {
		t.Fatal(e)
	}
	key, cose := keyPair(t)
	return s, registerResponse(t, s, []byte("credential"), cose, 0x45), key
}
func loginSetup(t testing.TB, rp *RP) (State, Credential, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	s, r, key := registration(t, rp)
	cred, e := rp.FinishRegistration(s, "flow", r)
	if e != nil {
		t.Fatal(e)
	}
	_, s, e = rp.BeginLogin(LoginRequest{FlowID: "flow"})
	if e != nil {
		t.Fatal(e)
	}
	return s, cred, key, assertion(t, s, cred, key, 5, 1)
}
func wantError(t testing.TB, err, want error, result any) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if !reflect.ValueOf(result).IsZero() {
		t.Fatalf("partial result: %#v", result)
	}
}

func TestRegistrationAndLogin(t *testing.T) {
	rp := testRP(t)
	s, cred, key, _ := loginSetup(t, rp)
	for _, known := range []bool{false, true} {
		t.Run(fmt.Sprint(known), func(t *testing.T) {
			req := LoginRequest{FlowID: "flow"}
			if known {
				req.UserID = cred.UserID
				req.Allow = []CredentialDescriptor{{ID: cred.ID}}
			}
			opts, state, err := rp.BeginLogin(req)
			if err != nil {
				t.Fatal(err)
			}
			if opts.UserVerification != "required" || len(state.Challenge) != 32 {
				t.Fatal("policy")
			}
			r := assertion(t, state, cred, key, 5, 1)
			if known {
				v, resp := responseMap(t, r)
				resp["userHandle"] = nil
				r = jsonBytes(t, v)
			}
			before := jsonBytes(t, []any{state, cred})
			got, err := rp.FinishLogin(state, "flow", cred, r)
			if err != nil {
				t.Fatal(err)
			}
			if got.SignCount != 1 || got.CounterAnomaly || !bytes.Equal(got.UserID, cred.UserID) {
				t.Fatalf("%+v", got)
			}
			if !bytes.Equal(before, jsonBytes(t, []any{state, cred})) {
				t.Fatal("mutated arguments")
			}
		})
	}
	// State persists across a process/relying-party restart.
	var restored State
	if err := json.Unmarshal(jsonBytes(t, s), &restored); err != nil {
		t.Fatal(err)
	}
	if _, err := testRP(t).FinishLogin(restored, "flow", cred, assertion(t, s, cred, key, 5, 1)); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentOpenSSLVector(t *testing.T) {
	data, err := os.ReadFile("testdata/openssl-es256.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Challenge    string
		UserID       string
		PublicKey    string
		Registration json.RawMessage
		Login        json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	rp := testRP(t)
	_, s, err := rp.BeginRegistration(RegistrationRequest{FlowID: "vector", User: User{ID: unbase(t, fixture.UserID), Name: "vector", DisplayName: "Vector"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Challenge = unbase(t, fixture.Challenge)
	cred, err := rp.FinishRegistration(s, "vector", fixture.Registration)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cred.PublicKey, unbase(t, fixture.PublicKey)) {
		t.Fatal("COSE key mismatch")
	}
	_, s, err = rp.BeginLogin(LoginRequest{FlowID: "vector"})
	if err != nil {
		t.Fatal(err)
	}
	s.Challenge = unbase(t, fixture.Challenge)
	result, err := rp.FinishLogin(s, "vector", cred, fixture.Login)
	if err != nil {
		t.Fatal(err)
	}
	if result.SignCount != 2 || !result.BackupState || result.CounterAnomaly {
		t.Fatalf("%+v", result)
	}
}

func TestConfig(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*Config)
	}{
		{"empty rp", func(c *Config) { c.RPID = "" }}, {"public suffix", func(c *Config) { c.RPID = "co.uk"; c.Origins = []string{"https://a.co.uk"} }},
		{"private suffix", func(c *Config) { c.RPID = "github.io"; c.Origins = []string{"https://a.github.io"} }},
		{"wildcard", func(c *Config) { c.Origins = []string{"https://*.example.com"} }}, {"http", func(c *Config) { c.Origins = []string{"http://example.com"} }},
		{"unrelated", func(c *Config) { c.Origins = []string{"https://example.com.evil.test"} }}, {"path", func(c *Config) { c.Origins = []string{"https://example.com/"} }},
		{"userinfo", func(c *Config) { c.Origins = []string{"https://user@example.com"} }}, {"query", func(c *Config) { c.Origins = []string{"https://example.com?"} }},
		{"fragment", func(c *Config) { c.Origins = []string{"https://example.com#x"} }}, {"port", func(c *Config) { c.Origins = []string{"https://example.com:99999"} }},
		{"default port", func(c *Config) { c.Origins = []string{"https://example.com:443"} }}, {"uppercase", func(c *Config) { c.Origins = []string{"https://EXAMPLE.com"} }},
		{"ip", func(c *Config) { c.RPID = "127.0.0.1" }}, {"empty origins", func(c *Config) { c.Origins = nil }}, {"empty name", func(c *Config) { c.RPName = "" }},
		{"negative ttl", func(c *Config) { c.Timeout = -time.Second }}, {"tiny ttl", func(c *Config) { c.Timeout = time.Nanosecond }}, {"localhost disabled", func(c *Config) { c.RPID = "localhost"; c.Origins = []string{"http://localhost:8080"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Config{RPID: "example.com", RPName: "Example", Origins: []string{"https://example.com"}}
			tc.modify(&c)
			rp, e := New(c)
			if !errors.Is(e, ErrConfig) || rp != nil {
				t.Fatalf("%v %v", rp, e)
			}
		})
	}
	for _, c := range []Config{{RPID: "localhost", RPName: "Local", Origins: []string{"http://localhost:8080"}, AllowLocalhost: true}, {RPID: "example.com", RPName: "Example", Origins: []string{"https://login.example.com:8443"}}} {
		if _, e := New(c); e != nil {
			t.Fatal(e)
		}
	}
	origins := []string{"https://example.com"}
	rp, e := New(Config{RPID: "example.com", RPName: "Example", Origins: origins})
	if e != nil {
		t.Fatal(e)
	}
	origins[0] = "https://evil.test"
	if rp.config.Origins[0] != "https://example.com" {
		t.Fatal("aliased config")
	}
}

func TestBeginLimitsAndRandomFailure(t *testing.T) {
	rp := testRP(t)
	for _, n := range []int{0, 65} {
		_, _, e := rp.BeginRegistration(RegistrationRequest{FlowID: "flow", User: User{ID: make([]byte, n), Name: "u", DisplayName: "U"}})
		if !errors.Is(e, ErrInvalidInput) {
			t.Fatal(e)
		}
	}
	for _, req := range []LoginRequest{{}, {FlowID: "x", UserID: []byte("u")}, {FlowID: "x", Allow: []CredentialDescriptor{{ID: []byte("x")}}}, {FlowID: "x", UserID: []byte("u"), Allow: make([]CredentialDescriptor, 65)}} {
		_, _, e := rp.BeginLogin(req)
		if !errors.Is(e, ErrInvalidInput) {
			t.Fatal(e)
		}
	}
	for _, n := range []int{0, 1024} {
		_, _, e := rp.BeginLogin(LoginRequest{FlowID: "x", UserID: []byte("u"), Allow: []CredentialDescriptor{{ID: make([]byte, n)}}})
		if !errors.Is(e, ErrInvalidInput) {
			t.Fatal(e)
		}
	}
	allow := make([]CredentialDescriptor, 64)
	for i := range allow {
		allow[i].ID = bytes.Repeat([]byte{byte(i)}, 1023)
	}
	opts, s, e := rp.BeginLogin(LoginRequest{FlowID: "x", UserID: bytes.Repeat([]byte{1}, 64), Allow: allow})
	if e != nil {
		t.Fatal(e)
	}
	allow[0].ID[0] = 99
	if s.CredentialIDs[0][0] == 99 {
		t.Fatal("aliased ID")
	}
	if len(opts.AllowCredentials) != 64 {
		t.Fatal("allow limit")
	}
	rp.random = bytes.NewReader(nil)
	o, state, e := rp.BeginLogin(LoginRequest{FlowID: "x"})
	if e == nil || !reflect.ValueOf(o).IsZero() || !reflect.ValueOf(state).IsZero() {
		t.Fatal("random failure returned options")
	}
}

func TestStateChecks(t *testing.T) {
	rp := testRP(t)
	s, c, _, r := loginSetup(t, rp)
	for _, tc := range []struct {
		name   string
		change func(*State)
		err    error
	}{
		{"deadline", func(s *State) { s.IssuedAt = testTime.Add(-5 * time.Minute); s.ExpiresAt = testTime }, ErrExpired},
		{"future", func(s *State) { s.IssuedAt = testTime.Add(time.Second); s.ExpiresAt = s.IssuedAt.Add(5 * time.Minute) }, ErrContext},
		{"operation", func(s *State) { s.Operation = "webauthn.create" }, ErrContext}, {"flow", func(s *State) { s.FlowID = "other" }, ErrContext},
		{"binding", func(s *State) { s.Binding = []byte("bad") }, ErrContext}, {"challenge", func(s *State) { s.Challenge = []byte("bad") }, ErrContext},
		{"extended", func(s *State) { s.ExpiresAt = s.ExpiresAt.Add(time.Second) }, ErrContext},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := s
			tc.change(&copy)
			result, e := rp.FinishLogin(copy, "flow", c, r)
			wantError(t, e, tc.err, result)
		})
	}
	before := s
	before.IssuedAt = testTime.Add(-5*time.Minute + time.Nanosecond)
	before.ExpiresAt = testTime.Add(time.Nanosecond)
	if _, e := rp.FinishLogin(before, "flow", c, r); e != nil {
		t.Fatal(e)
	}
	other, e := New(Config{RPID: "example.com", RPName: "X", Origins: []string{"https://other.example.com"}, Now: func() time.Time { return testTime }})
	if e != nil {
		t.Fatal(e)
	}
	result, e := other.FinishLogin(s, "flow", c, r)
	wantError(t, e, ErrContext, result)
}

func TestLoginTampering(t *testing.T) {
	rp := testRP(t)
	s, c, _, original := loginSetup(t, rp)
	mutations := []struct {
		name   string
		change func(map[string]any, map[string]any)
		want   error
	}{
		{"id", func(v, r map[string]any) { v["id"] = encode([]byte("other")) }, ErrInvalidInput},
		{"rawId", func(v, r map[string]any) { v["rawId"] = encode([]byte("other")) }, ErrInvalidInput},
		{"credential type", func(v, r map[string]any) { v["type"] = "other" }, ErrInvalidInput},
		{"handle", func(v, r map[string]any) { r["userHandle"] = encode([]byte("other")) }, ErrCredential},
		{"missing handle", func(v, r map[string]any) { delete(r, "userHandle") }, ErrCredential},
		{"null handle", func(v, r map[string]any) { r["userHandle"] = nil }, ErrCredential},
		{"signature", func(v, r map[string]any) { r["signature"] = encode([]byte("invalid")) }, ErrCredential},
		{"auth wrong type", func(v, r map[string]any) { r["authenticatorData"] = 42 }, ErrInvalidInput},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			v, r := responseMap(t, original)
			tc.change(v, r)
			result, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
			wantError(t, e, tc.want, result)
		})
	}
	for _, tc := range []struct {
		name  string
		value any
		want  error
	}{{"type", "webauthn.create", ErrContext}, {"challenge", encode(bytes.Repeat([]byte{9}, 32)), ErrContext}, {"origin", "https://evil.test", ErrContext}, {"crossOrigin", true, ErrUnsupported}, {"crossOrigin", nil, ErrInvalidInput}, {"topOrigin", "https://evil.test", ErrUnsupported}} {
		t.Run(fmt.Sprint(tc.name, tc.value), func(t *testing.T) {
			v, r := responseMap(t, original)
			client := document(t, unbase(t, r["clientDataJSON"].(string)))
			client[tc.name] = tc.value
			r["clientDataJSON"] = encode(jsonBytes(t, client))
			result, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
			wantError(t, e, tc.want, result)
		})
	}
	for _, tc := range []struct {
		name   string
		change func([]byte)
		want   error
	}{
		{"rp hash", func(b []byte) { b[0] ^= 1 }, ErrContext}, {"up", func(b []byte) { b[32] &^= 1 }, ErrPolicy}, {"uv", func(b []byte) { b[32] &^= 4 }, ErrPolicy},
		{"be", func(b []byte) { b[32] |= 8 }, ErrCredential}, {"bs without be", func(b []byte) { b[32] |= 16 }, ErrInvalidInput}, {"at", func(b []byte) { b[32] |= 64 }, ErrInvalidInput}, {"ed without data", func(b []byte) { b[32] |= 128 }, ErrInvalidInput},
		{"counter unsigned tamper", func(b []byte) { b[36]++ }, ErrCredential},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, r := responseMap(t, original)
			auth := unbase(t, r["authenticatorData"].(string))
			tc.change(auth)
			r["authenticatorData"] = encode(auth)
			result, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
			wantError(t, e, tc.want, result)
		})
	}
	bad := c
	bad.UserID = []byte("other")
	result, e := rp.FinishLogin(s, "flow", bad, original)
	wantError(t, e, ErrCredential, result)
}

func TestCountersBackupAndOriginalBytes(t *testing.T) {
	rp := testRP(t)
	s, c, key, _ := loginSetup(t, rp)
	for _, tc := range []struct {
		old, new uint32
		anomaly  bool
	}{{0, 0, false}, {0, 1, false}, {1, 2, false}, {1, 1, true}, {2, 1, true}, {1, 0, true}} {
		t.Run(fmt.Sprintf("%d-%d", tc.old, tc.new), func(t *testing.T) {
			cred := c
			cred.SignCount = tc.old
			r := assertion(t, s, cred, key, 5, tc.new)
			got, e := rp.FinishLogin(s, "flow", cred, r)
			if e != nil || got.CounterAnomaly != tc.anomaly {
				t.Fatalf("%+v %v", got, e)
			}
		})
	}
	for _, flags := range []byte{0x0d, 0x1d, 0x2f} {
		cred := c
		cred.BackupEligible = true
		r := assertion(t, s, cred, key, flags, 1)
		got, e := rp.FinishLogin(s, "flow", cred, r)
		if e != nil || got.BackupState != (flags&16 != 0) {
			t.Fatalf("%+v %v", got, e)
		}
	}
	client := []byte(fmt.Sprintf("{\n  \"origin\": \"https://example.com\", \"challenge\": %q, \"type\": \"webauthn.get\", \"future\": {\"ok\": true}, \"topOrigin\": \"https://example.com\"\n}", encode(s.Challenge)))
	auth := append(authBytes(0x85, 1), cborBytes(t, map[string]any{"future": map[string]any{"x": uint64(1)}})...)
	r := signedResponse(t, s, c, key, client, auth)
	if _, e := rp.FinishLogin(s, "flow", c, r); e != nil {
		t.Fatal(e)
	}
	v, resp := responseMap(t, r)
	resp["clientDataJSON"] = encode(jsonBytes(t, document(t, client)))
	got, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
	wantError(t, e, ErrCredential, got)
}

func TestRegistrationTampering(t *testing.T) {
	rp := testRP(t)
	s, original, _ := registration(t, rp)
	for _, tc := range []struct {
		name   string
		modify func(map[string]any)
		want   error
	}{
		{"format", func(a map[string]any) { a["fmt"] = "packed" }, ErrUnsupported}, {"statement", func(a map[string]any) { a["attStmt"] = map[string]any{"sig": []byte{1}} }, ErrInvalidInput}, {"null statement", func(a map[string]any) { a["attStmt"] = nil }, ErrInvalidInput},
		{"up", func(a map[string]any) { a["authData"].([]byte)[32] &^= 1 }, ErrPolicy}, {"uv", func(a map[string]any) { a["authData"].([]byte)[32] &^= 4 }, ErrPolicy}, {"rp", func(a map[string]any) { a["authData"].([]byte)[0] ^= 1 }, ErrContext}, {"at", func(a map[string]any) { a["authData"].([]byte)[32] &^= 64 }, ErrInvalidInput},
		{"credential ID", func(a map[string]any) { a["authData"].([]byte)[55] ^= 1 }, ErrCredential}, {"trailing", func(a map[string]any) { a["authData"] = append(a["authData"].([]byte), 0) }, ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, r := responseMap(t, original)
			var a map[string]any
			if e := cbor.Unmarshal(unbase(t, r["attestationObject"].(string)), &a); e != nil {
				t.Fatal(e)
			}
			tc.modify(a)
			r["attestationObject"] = encode(cborBytes(t, a))
			result, e := rp.FinishRegistration(s, "flow", jsonBytes(t, v))
			wantError(t, e, tc.want, result)
		})
	}
	for _, tc := range []struct {
		name  string
		label int
		value any
		want  error
	}{{"algorithm", 3, -257, ErrUnsupported}, {"curve", -1, 2, ErrUnsupported}, {"type", 1, 3, ErrUnsupported}, {"private", -4, make([]byte, 32), ErrCredential}, {"point", -2, make([]byte, 32), ErrCredential}, {"short", -3, make([]byte, 31), ErrCredential}} {
		t.Run(tc.name, func(t *testing.T) {
			_, key := keyPair(t)
			var m map[int]any
			if e := cbor.Unmarshal(key, &m); e != nil {
				t.Fatal(e)
			}
			m[tc.label] = tc.value
			r := registerResponse(t, s, []byte("credential"), cborBytes(t, m), 0x45)
			result, e := rp.FinishRegistration(s, "flow", r)
			wantError(t, e, tc.want, result)
		})
	}
	s.CredentialIDs = [][]byte{[]byte("credential")}
	result, e := rp.FinishRegistration(s, "flow", original)
	wantError(t, e, ErrCredential, result)
}

func TestStrictParsing(t *testing.T) {
	rp := testRP(t)
	s, c, _, original := loginSetup(t, rp)
	invalidJSON := [][]byte{[]byte(`{"a":1,"a":2}`), []byte(`{"a":{"b":1,"b":2}}`), []byte(`{"a":1,"\u0061":2}`), []byte(`{} {}`), []byte(`null`), []byte(`[]`), []byte("{\"x\":\"\xff\"}"), []byte(`{"x":` + strings.Repeat("[", 17) + `0` + strings.Repeat("]", 17) + `}`)}
	for _, b := range invalidJSON {
		if _, e := object(b, maxResponse); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("accepted %q: %v", b, e)
		}
	}
	if _, e := object([]byte(`{"x":`+strings.Repeat(" ", maxResponse)+`0}`), maxResponse); e == nil {
		t.Fatal("oversized JSON")
	}
	if _, e := object([]byte(`{}`+strings.Repeat(" ", maxResponse-2)), maxResponse); e != nil {
		t.Fatal(e)
	}
	for _, b := range []string{"a", "AA=", "AA\n", "AB", "+/", ""} {
		if _, e := decode(b, 64); e == nil {
			t.Fatalf("accepted base64 %q", b)
		}
	}
	for i := 0; i < len(original); i++ {
		if _, e := CredentialID(original[:i]); e == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	for _, raw := range []string{`{"type":"webauthn.get","type":"webauthn.get"}`, `{"type":null}`, `{"Type":"webauthn.get"}`} {
		v, r := responseMap(t, original)
		r["clientDataJSON"] = encode([]byte(raw))
		result, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
		wantError(t, e, ErrInvalidInput, result)
	}
	for _, raw := range [][]byte{{0xa2, 0x61, 'x', 0, 0x61, 'x', 1}, {0xa1, 0x61, 'x', 0xa2, 0x61, 'y', 0, 0x61, 'y', 1}, {0xbf, 0xff}, {0xc0, 0x60}, append(bytes.Repeat([]byte{0x81}, 17), 0), {0x99, 1, 1}} {
		auth := append(authBytes(0x85, 1), raw...)
		if _, e := rp.parseAuth(auth, false); e == nil {
			t.Fatalf("accepted CBOR %x", raw)
		}
	}
	for _, n := range []int{0, 1, 36, maxAuthData + 1} {
		if _, e := rp.parseAuth(make([]byte, n), false); e == nil {
			t.Fatal("auth length")
		}
	}
	// Failure categories contain no submitted data.
	secret := "SECRET-PAYLOAD"
	v, r := responseMap(t, original)
	r["signature"] = secret
	result, e := rp.FinishLogin(s, "flow", c, jsonBytes(t, v))
	if e == nil || strings.Contains(e.Error(), secret) || !reflect.ValueOf(result).IsZero() {
		t.Fatal("unsafe error")
	}
}

func TestConcurrentIndependentOperations(t *testing.T) {
	rp := testRP(t)
	s, c, _, r := loginSetup(t, rp)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := rp.FinishLogin(s, "flow", c, r); e != nil {
				t.Error(e)
			}
			if _, _, e := rp.BeginLogin(LoginRequest{FlowID: "parallel"}); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
}

func FuzzJSON(f *testing.F) {
	f.Add([]byte(`{"id":"YQ","rawId":"YQ","type":"public-key","response":{}}`))
	f.Add([]byte(`{"x":1,"x":2}`))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = object(b, maxResponse); _, _ = CredentialID(b) })
}
func FuzzAuthenticatorData(f *testing.F) {
	rp := testRP(f)
	f.Add(authBytes(5, 0), false)
	s, r, _ := registration(f, rp)
	_ = s
	_, resp := responseMap(f, r)
	var att map[string]any
	if e := cbor.Unmarshal(unbase(f, resp["attestationObject"].(string)), &att); e != nil {
		f.Fatal(e)
	}
	f.Add(att["authData"].([]byte), true)
	f.Fuzz(func(t *testing.T, b []byte, registration bool) { _, _ = rp.parseAuth(b, registration) })
}
func FuzzCBOR(f *testing.F) {
	rp := testRP(f)
	_, key := keyPair(f)
	f.Add(key)
	f.Add([]byte{0xa2, 1, 2, 1, 3})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = rp.publicKey(b) })
}
func FuzzFinish(f *testing.F) {
	rp := testRP(f)
	data, err := os.ReadFile("testdata/openssl-es256.json")
	if err != nil {
		f.Fatal(err)
	}
	var fixture struct {
		Challenge    string
		UserID       string
		PublicKey    string
		Registration json.RawMessage
		Login        json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		f.Fatal(err)
	}
	_, rs, err := rp.BeginRegistration(RegistrationRequest{FlowID: "vector", User: User{ID: unbase(f, fixture.UserID), Name: "vector", DisplayName: "Vector"}})
	if err != nil {
		f.Fatal(err)
	}
	rs.Challenge = unbase(f, fixture.Challenge)
	c, err := rp.FinishRegistration(rs, "vector", fixture.Registration)
	if err != nil || !bytes.Equal(c.PublicKey, unbase(f, fixture.PublicKey)) {
		f.Fatalf("invalid registration seed: %v", err)
	}
	_, ls, err := rp.BeginLogin(LoginRequest{FlowID: "vector"})
	if err != nil {
		f.Fatal(err)
	}
	ls.Challenge = unbase(f, fixture.Challenge)
	if _, err := rp.FinishLogin(ls, "vector", c, fixture.Login); err != nil {
		f.Fatalf("invalid login seed: %v", err)
	}
	f.Add([]byte(fixture.Registration))
	f.Add([]byte(fixture.Login))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, e := rp.FinishRegistration(rs, "vector", b)
		if e != nil && !reflect.ValueOf(r).IsZero() {
			t.Fatal("partial registration")
		}
		l, e := rp.FinishLogin(ls, "vector", c, b)
		if e != nil && !reflect.ValueOf(l).IsZero() {
			t.Fatal("partial login")
		}
	})
}

func TestKnownUserBinding(t *testing.T) {
	rp := testRP(t)
	_, cred, key, _ := loginSetup(t, rp)
	for _, tc := range []struct {
		name     string
		user, id []byte
	}{{"owner", []byte("other"), cred.ID}, {"allow list", cred.UserID, []byte("other-key")}} {
		t.Run(tc.name, func(t *testing.T) {
			_, s, e := rp.BeginLogin(LoginRequest{FlowID: "flow", UserID: tc.user, Allow: []CredentialDescriptor{{ID: tc.id}}})
			if e != nil {
				t.Fatal(e)
			}
			result, e := rp.FinishLogin(s, "flow", cred, assertion(t, s, cred, key, 5, 1))
			wantError(t, e, ErrCredential, result)
		})
	}
}

func TestBinaryBoundaries(t *testing.T) {
	rp := testRP(t)
	state, _, _ := registration(t, rp)
	_, key := keyPair(t)
	for _, n := range []int{1, 1023} {
		r := registerResponse(t, state, bytes.Repeat([]byte{1}, n), key, 0x45)
		if _, e := rp.FinishRegistration(state, "flow", r); e != nil {
			t.Fatalf("ID size %d: %v", n, e)
		}
	}
	var cose map[int]any
	if e := cbor.Unmarshal(key, &cose); e != nil {
		t.Fatal(e)
	}
	cose[2] = []byte("optional kid")
	result, e := rp.FinishRegistration(state, "flow", registerResponse(t, state, []byte("id"), cborBytes(t, cose), 0x45))
	wantError(t, e, ErrCredential, result)
	for _, n := range []int{256, 257} {
		values := make([]any, n)
		for i := range values {
			values[i] = uint64(0)
		}
		auth := append(authBytes(0x85, 0), cborBytes(t, map[string]any{"future": values})...)
		_, e := rp.parseAuth(auth, false)
		if (e == nil) != (n == 256) {
			t.Fatalf("array size %d: %v", n, e)
		}
		pairs := make(map[string]any, n)
		for i := 0; i < n; i++ {
			pairs[fmt.Sprint(i)] = uint64(0)
		}
		auth = append(authBytes(0x85, 0), cborBytes(t, pairs)...)
		_, e = rp.parseAuth(auth, false)
		if (e == nil) != (n == 256) {
			t.Fatalf("map size %d: %v", n, e)
		}
	}
	// Exactly 16 levels are accepted; the 17th is rejected by the CBOR mode.
	for _, depth := range []int{16, 17} {
		var v any = uint64(0)
		for i := 1; i < depth; i++ {
			v = []any{v}
		}
		auth := append(authBytes(0x85, 0), cborBytes(t, map[string]any{"future": v})...)
		_, e := rp.parseAuth(auth, false)
		if (e == nil) != (depth == 16) {
			t.Fatalf("depth %d: %v", depth, e)
		}
	}
	// Extensions make an authenticatorData value exactly 16 KiB long.
	for _, n := range []int{maxAuthData, maxAuthData + 1} {
		auth := append(authBytes(0x85, 0), cborBytes(t, map[string]any{"x": make([]byte, n-43)})...)
		if len(auth) != n {
			t.Fatalf("test construction %d != %d", len(auth), n)
		}
		_, e := rp.parseAuth(auth, false)
		if (e == nil) != (n == maxAuthData) {
			t.Fatalf("auth size %d: %v", n, e)
		}
	}
	client := clientBytes(t, state)
	for _, n := range []int{maxClientData, maxClientData + 1} {
		raw := append(append([]byte(nil), client...), bytes.Repeat([]byte(" "), n-len(client))...)
		_, e := rp.clientData(map[string]json.RawMessage{"clientDataJSON": jsonBytes(t, encode(raw))}, state)
		if (e == nil) != (n == maxClientData) {
			t.Fatalf("client size %d: %v", n, e)
		}
	}
	auth := authBytes(0x45, 0)
	auth = append(auth, make([]byte, 16)...)
	auth = binary.BigEndian.AppendUint16(auth, 2)
	auth = append(auth, 1, 2)
	auth = append(auth, key...)
	for n := 0; n < len(auth); n++ {
		if _, e := rp.parseAuth(auth[:n], true); e == nil {
			t.Fatalf("auth truncation %d", n)
		}
	}
	// Duplicate COSE keys must be rejected even when both values agree.
	duplicate := append([]byte{0xa6, 1, 2}, key[1:]...)
	if _, e := rp.publicKey(duplicate); !errors.Is(e, ErrInvalidInput) {
		t.Fatal(e)
	}
}
