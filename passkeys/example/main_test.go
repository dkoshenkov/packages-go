package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dkoshenkov/packages-go/passkeys"
)

var now = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func fixedNow() time.Time { return now }

func scenario(t *testing.T) (*store, pending, storedCredential, passkeys.LoginResult) {
	t.Helper()
	db := newStore()
	p := pending{State: passkeys.State{Operation: "webauthn.get", FlowID: "flow", ExpiresAt: now.Add(time.Minute)}}
	c := storedCredential{Credential: passkeys.Credential{ID: []byte("key"), UserID: []byte("user")}, Version: 1}
	db.pending["flow"] = p
	db.accounts["user"] = 1
	db.credentials["key"] = c
	return db, p, c, passkeys.LoginResult{CredentialID: []byte("key"), UserID: []byte("user"), SignCount: 1}
}
func TestSingleCommitAcrossServices(t *testing.T) {
	db, p, c, result := scenario(t)
	// Two service instances share one transactional store, as two replicas must
	// share a database. This is a model test, not distributed/database proof.
	services := []*server{{db: db}, {db: db}}
	var count atomic.Int32
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(s *server) {
			defer wg.Done()
			if s.db.login(p, c, 1, result, "session", fixedNow) == nil {
				count.Add(1)
			}
		}(service)
	}
	wg.Wait()
	if count.Load() != 1 || len(db.sessions) != 1 || len(db.pending) != 0 {
		t.Fatal("non-atomic consumption")
	}
	if db.login(p, c, 1, result, "replay", fixedNow) == nil {
		t.Fatal("sequential replay")
	}
	// Losing the response must not permit another session; a new flow can work.
	p.State.FlowID = "next"
	db.pending["next"] = p
	c = db.credentials["key"]
	result.SignCount = 2
	if err := db.login(p, c, 1, result, "new-session", fixedNow); err != nil {
		t.Fatal(err)
	}
}
func TestRecheckAfterVerification(t *testing.T) {
	for _, mutation := range []string{"revoked", "recovery", "changed key", "database failure", "expired", "counter anomaly"} {
		t.Run(mutation, func(t *testing.T) {
			db, p, c, r := scenario(t)
			switch mutation {
			case "revoked":
				delete(db.credentials, "key")
			case "recovery":
				db.accounts["user"]++
			case "changed key":
				updated := c
				updated.Version++
				db.credentials["key"] = updated
			case "database failure":
				db.failCommit = true
			case "expired":
				nowExpired := p
				p.State.ExpiresAt = now
				nowExpired.State.ExpiresAt = now
				db.pending["flow"] = nowExpired
			case "counter anomaly":
				r.CounterAnomaly = true
			}
			if db.login(p, c, 1, r, "session", fixedNow) == nil {
				t.Fatal("stale proof accepted")
			}
			if len(db.sessions) != 0 || len(db.pending) != 1 {
				t.Fatal("partial commit")
			}
			if value, ok := db.credentials["key"]; ok && value.Credential.SignCount != 0 {
				t.Fatal("metadata changed")
			}
		})
	}
}
func TestRegistrationTransactions(t *testing.T) {
	db := newStore()
	first := pending{State: passkeys.State{Operation: "webauthn.create", FlowID: "first", ExpiresAt: now.Add(time.Minute)}, Account: "user", Generation: 1, NewAccount: true}
	db.pending["first"] = first
	credential := passkeys.Credential{ID: []byte("one"), UserID: []byte("user")}
	if err := db.register(first, credential, "first-session", fixedNow); err != nil {
		t.Fatal(err)
	}
	second := first
	second.NewAccount = false
	second.AuthorizationExpires = now.Add(time.Minute)
	second.State.FlowID = "second"
	db.pending["second"] = second
	if db.register(second, credential, "duplicate-session", fixedNow) == nil {
		t.Fatal("duplicate accepted")
	}
	credential.ID = []byte("two")
	if err := db.register(second, credential, "second-session", fixedNow); err != nil {
		t.Fatal(err)
	}
	if len(db.credentials) != 2 || len(db.accounts) != 1 {
		t.Fatal("second key ownership")
	}
	other := first
	other.Account = "other"
	other.State.FlowID = "other"
	db.pending["other"] = other
	credential.UserID = []byte("other")
	if db.register(other, credential, "other-session", fixedNow) == nil {
		t.Fatal("cross-account duplicate accepted")
	}
	db.failCommit = true
	credential.ID = []byte("three")
	if db.register(other, credential, "failed-session", fixedNow) == nil || len(db.accounts) != 1 || len(db.credentials) != 2 {
		t.Fatal("failed registration changed database")
	}
	db.failCommit = false
	second.State.FlowID = "stale"
	db.pending["stale"] = second
	db.accounts["user"]++
	credential.UserID = []byte("user")
	if db.register(second, credential, "stale-session", fixedNow) == nil {
		t.Fatal("registration survived recovery")
	}
}
func request(t *testing.T, s *server, path, flow string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080"+path, bytes.NewReader(body))
	req.Header.Set("Origin", "http://localhost:8080")
	req.AddCookie(&http.Cookie{Name: "flow", Value: flow})
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	return rec
}
func TestHTTPFinishWithIndependentVector(t *testing.T) {
	data, err := os.ReadFile("../testdata/openssl-es256.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Challenge, UserID   string
		Registration, Login json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	user, err := base64.RawURLEncoding.DecodeString(fixture.UserID)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(fixture.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	// Browser transport is localhost; fixed independent protocol vector is example.com.
	rp, err := passkeys.New(passkeys.Config{RPID: "example.com", RPName: "Example", Origins: []string{"https://example.com"}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{rp: rp, db: newStore(), now: func() time.Time { return now }}
	_, state, err := rp.BeginRegistration(passkeys.RegistrationRequest{FlowID: "register", User: passkeys.User{ID: user, Name: "User", DisplayName: "User"}})
	if err != nil {
		t.Fatal(err)
	}
	state.Challenge = challenge
	s.db.pending[state.FlowID] = pending{State: state, Account: string(user), Generation: 1, NewAccount: true}
	rec := request(t, s, "/register/finish", "register", fixture.Registration)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	_, state, err = rp.BeginLogin(passkeys.LoginRequest{FlowID: "login"})
	if err != nil {
		t.Fatal(err)
	}
	state.Challenge = challenge
	s.db.pending[state.FlowID] = pending{State: state}
	rec = request(t, s, "/login/finish", "login", fixture.Login)
	if rec.Code != 200 || len(s.db.sessions) != 2 {
		t.Fatal(rec.Body.String())
	}
	rec = request(t, s, "/login/finish", "login", fixture.Login)
	if rec.Code == 200 || len(s.db.sessions) != 2 {
		t.Fatal("replay")
	}
	rec = request(t, s, "/login/finish", "missing", fixture.Login)
	if rec.Code == 200 {
		t.Fatal("unknown flow")
	}
}
func TestHTTPBoundariesAndCancellation(t *testing.T) {
	rp, err := passkeys.New(passkeys.Config{RPID: "localhost", RPName: "Demo", Origins: []string{"http://localhost:8080"}, AllowLocalhost: true, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{rp: rp, db: newStore(), now: func() time.Time { return now }}
	rec := request(t, s, "/register/begin", "", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if len(s.db.credentials) != 0 || len(s.db.accounts) != 0 {
		t.Fatal("begin created account")
	}
	// Cancellation sends no finish. Expiration removes the abandoned state.
	var old string
	for id := range s.db.pending {
		old = id
	}
	s.now = func() time.Time { return now.Add(6 * time.Minute) }
	rec = request(t, s, "/login/begin", "", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if _, ok := s.db.pending[old]; ok {
		t.Fatal("abandoned flow retained")
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/login/begin", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("CSRF")
	}
	req = httptest.NewRequest(http.MethodGet, "http://evil.test/", nil)
	rec = httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("Host")
	}
}

func TestFreshAuthorizationAtCommit(t *testing.T) {
	db := newStore()
	db.accounts["user"] = 1
	p := pending{State: passkeys.State{Operation: "webauthn.create", FlowID: "flow", ExpiresAt: now.Add(time.Minute)}, Account: "user", Generation: 1, AuthorizationExpires: now}
	db.pending["flow"] = p
	c := passkeys.Credential{ID: []byte("key"), UserID: []byte("user")}
	if db.register(p, c, "session", fixedNow) == nil {
		t.Fatal("expired fresh authorization accepted")
	}
	if len(db.credentials) != 0 || len(db.sessions) != 0 || len(db.pending) != 1 {
		t.Fatal("partial commit")
	}
}

func TestCommitClockUnderLock(t *testing.T) {
	for _, operation := range []string{"login", "registration"} {
		t.Run(operation, func(t *testing.T) {
			db, p, c, result := scenario(t)
			if operation == "registration" {
				p.State.Operation = "webauthn.create"
				p.Account, p.Generation = "user", 1
				p.AuthorizationExpires = now.Add(time.Minute)
				db.pending["flow"] = p
				c.Credential.ID = []byte("new-key")
			}
			clock := func() time.Time {
				if db.mu.TryLock() {
					db.mu.Unlock()
					t.Error("clock read outside transaction lock")
				}
				return now
			}
			var err error
			if operation == "login" {
				err = db.login(p, c, 1, result, "session", clock)
			} else {
				err = db.register(p, c.Credential, "session", clock)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExpiryWhileWaitingForCommit(t *testing.T) {
	for _, operation := range []string{"login", "registration"} {
		t.Run(operation, func(t *testing.T) {
			db, p, c, result := scenario(t)
			if operation == "registration" {
				p.State.Operation = "webauthn.create"
				p.Account, p.Generation = "user", 1
				p.State.ExpiresAt = now.Add(2 * time.Minute)
				p.AuthorizationExpires = now.Add(time.Minute)
				db.pending["flow"] = p
				c.Credential.ID = []byte("new-key")
			}
			current := now
			clock := func() time.Time { return current }
			started := make(chan struct{})
			done := make(chan error, 1)
			db.mu.Lock()
			go func() {
				close(started)
				if operation == "login" {
					done <- db.login(p, c, 1, result, "session", clock)
				} else {
					done <- db.register(p, c.Credential, "session", clock)
				}
			}()
			<-started
			current = now.Add(time.Minute)
			db.mu.Unlock()
			if err := <-done; err == nil {
				t.Fatal("expired operation committed")
			}
			if len(db.sessions) != 0 || len(db.pending) != 1 || len(db.credentials) != 1 || db.credentials["key"].Version != 1 {
				t.Fatal("expired operation changed database")
			}
		})
	}
}
