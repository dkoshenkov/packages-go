// Command example serves a single-process, ephemeral passkeys demonstration.
// Run from the module root: go run ./passkeys/example
package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/dkoshenkov/packages-go/passkeys"
)

//go:embed index.html
var assets embed.FS

var errCommit = errors.New("operation no longer valid")

// store models a transactional database ONLY inside this demo process. Two
// service instances in tests share this store; separate OS processes do not.
// Production needs shared durable transactions (see transaction.sql).
type store struct {
	mu          sync.Mutex
	pending     map[string]pending
	accounts    map[string]uint64
	credentials map[string]storedCredential
	sessions    map[string]session
	failCommit  bool // Tests model an atomic storage failure before commit.
}
type pending struct {
	State                passkeys.State
	Account              string
	Generation           uint64
	NewAccount           bool
	AuthorizationExpires time.Time
}
type storedCredential struct {
	Credential passkeys.Credential
	Version    uint64
}
type session struct {
	Account    string
	Generation uint64
	Created    time.Time
}
type server struct {
	rp  *passkeys.RP
	db  *store
	now func() time.Time
}

func newStore() *store {
	return &store{pending: map[string]pending{}, accounts: map[string]uint64{}, credentials: map[string]storedCredential{}, sessions: map[string]session{}}
}
func token() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, e := assets.ReadFile("index.html")
		if e != nil {
			http.Error(w, "unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, e := w.Write(data); e != nil {
			return
		}
	})
	mux.HandleFunc("POST /register/begin", s.beginRegistration)
	mux.HandleFunc("POST /register/finish", s.finishRegistration)
	mux.HandleFunc("POST /login/begin", s.beginLogin)
	mux.HandleFunc("POST /login/finish", s.finishLogin)
	mux.HandleFunc("GET /me", s.me)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		// Fixed expected origin; never derive it from Host or forwarded headers.
		if r.Host != "localhost:8080" || r.Method == http.MethodPost && r.Header.Get("Origin") != "http://localhost:8080" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if e := json.NewEncoder(w).Encode(v); e != nil {
		return
	}
}
func fail(w http.ResponseWriter) {
	http.Error(w, "operation failed; start again", http.StatusBadRequest)
}
func setCookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func (s *server) account(r *http.Request) (session, bool) {
	c, e := r.Cookie("session")
	if e != nil {
		return session{}, false
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	value, ok := s.db.sessions[c.Value]
	return value, ok && s.db.accounts[value.Account] == value.Generation && s.now().Before(value.Created.Add(time.Hour))
}
func (s *server) savePending(w http.ResponseWriter, p pending, options any) {
	s.db.mu.Lock()
	for id, old := range s.db.pending {
		if !s.now().Before(old.State.ExpiresAt) {
			delete(s.db.pending, id)
		}
	}
	if len(s.db.pending) >= 1024 {
		s.db.mu.Unlock()
		http.Error(w, "demo capacity reached", http.StatusServiceUnavailable)
		return
	}
	s.db.pending[p.State.FlowID] = p
	s.db.mu.Unlock()
	setCookie(w, "flow", p.State.FlowID, 300)
	writeJSON(w, options)
}
func (s *server) beginRegistration(w http.ResponseWriter, r *http.Request) {
	flow, e := token()
	if e != nil {
		fail(w)
		return
	}
	existing, ok := s.account(r)
	if ok && !s.now().Before(existing.Created.Add(5*time.Minute)) {
		fail(w)
		return
	}
	p := pending{Account: existing.Account, Generation: existing.Generation, NewAccount: !ok, AuthorizationExpires: existing.Created.Add(5 * time.Minute)}
	if !ok {
		p.Account, e = token()
		if e != nil {
			fail(w)
			return
		}
		p.Generation = 1
	}
	var exclude []passkeys.CredentialDescriptor
	s.db.mu.Lock()
	for _, c := range s.db.credentials {
		if string(c.Credential.UserID) == p.Account {
			exclude = append(exclude, passkeys.CredentialDescriptor{ID: c.Credential.ID, Transports: c.Credential.Transports})
		}
	}
	s.db.mu.Unlock()
	options, state, e := s.rp.BeginRegistration(passkeys.RegistrationRequest{FlowID: flow, User: passkeys.User{ID: []byte(p.Account), Name: p.Account, DisplayName: "Demo account"}, Exclude: exclude})
	if e != nil {
		fail(w)
		return
	}
	p.State = state
	s.savePending(w, p, options)
}
func (s *server) beginLogin(w http.ResponseWriter, r *http.Request) {
	flow, e := token()
	if e != nil {
		fail(w)
		return
	}
	options, state, e := s.rp.BeginLogin(passkeys.LoginRequest{FlowID: flow})
	if e != nil {
		fail(w)
		return
	}
	s.savePending(w, pending{State: state}, options)
}
func (s *server) load(r *http.Request, w http.ResponseWriter) (pending, []byte, error) {
	cookie, e := r.Cookie("flow")
	if e != nil {
		return pending{}, nil, e
	}
	s.db.mu.Lock()
	p, ok := s.db.pending[cookie.Value]
	s.db.mu.Unlock()
	if !ok {
		return pending{}, nil, errCommit
	}
	body := http.MaxBytesReader(w, r.Body, 64<<10)
	defer body.Close()
	b, e := io.ReadAll(body)
	return p, b, e
}
func (s *server) finishRegistration(w http.ResponseWriter, r *http.Request) {
	p, body, e := s.load(r, w)
	if e != nil {
		fail(w)
		return
	}
	cred, e := s.rp.FinishRegistration(p.State, p.State.FlowID, body)
	if e != nil {
		fail(w)
		return
	}
	sessionID, e := token()
	if e != nil {
		fail(w)
		return
	}
	if e = s.db.register(p, cred, sessionID, s.now); e != nil {
		fail(w)
		return
	}
	setCookie(w, "session", sessionID, 3600)
	setCookie(w, "flow", "", -1)
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *server) finishLogin(w http.ResponseWriter, r *http.Request) {
	p, body, e := s.load(r, w)
	if e != nil {
		fail(w)
		return
	}
	id, e := passkeys.CredentialID(body)
	if e != nil {
		fail(w)
		return
	}
	s.db.mu.Lock()
	stored, ok := s.db.credentials[string(id)]
	generation := s.db.accounts[string(stored.Credential.UserID)]
	s.db.mu.Unlock()
	if !ok || generation == 0 {
		fail(w)
		return
	}
	result, e := s.rp.FinishLogin(p.State, p.State.FlowID, stored.Credential, body)
	if e != nil {
		fail(w)
		return
	}
	sessionID, e := token()
	if e != nil {
		fail(w)
		return
	}
	if e = s.db.login(p, stored, generation, result, sessionID, s.now); e != nil {
		fail(w)
		return
	}
	setCookie(w, "session", sessionID, 3600)
	setCookie(w, "flow", "", -1)
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	_, ok := s.account(r)
	writeJSON(w, map[string]bool{"authenticated": ok})
}

// current is checked under the transaction lock, after crypto verification.
func (db *store) current(p pending, now time.Time) bool {
	current, ok := db.pending[p.State.FlowID]
	return ok && !db.failCommit && now.Before(current.State.ExpiresAt) && current.State.Operation == p.State.Operation && current.Account == p.Account && current.Generation == p.Generation
}
func (db *store) register(p pending, c passkeys.Credential, sessionID string, clock func() time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	now := clock()
	if !db.current(p, now) || p.State.Operation != "webauthn.create" || string(c.UserID) != p.Account {
		return errCommit
	}
	if _, exists := db.credentials[string(c.ID)]; exists {
		return errCommit
	}
	generation, exists := db.accounts[p.Account]
	if p.NewAccount {
		if exists || len(db.accounts) >= 1024 {
			return errCommit
		}
	} else if !exists || generation != p.Generation || !now.Before(p.AuthorizationExpires) {
		return errCommit
	}
	// No fallible work after this point: model a single atomic commit.
	db.accounts[p.Account] = p.Generation
	db.credentials[string(c.ID)] = storedCredential{Credential: c, Version: 1}
	delete(db.pending, p.State.FlowID)
	db.sessions[sessionID] = session{Account: p.Account, Generation: p.Generation, Created: now}
	return nil
}
func (db *store) login(p pending, expected storedCredential, generation uint64, result passkeys.LoginResult, sessionID string, clock func() time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	now := clock()
	current, exists := db.credentials[string(expected.Credential.ID)]
	account := string(expected.Credential.UserID)
	if !db.current(p, now) || p.State.Operation != "webauthn.get" || !exists || current.Version != expected.Version || db.accounts[account] != generation || generation == 0 || result.CounterAnomaly {
		return errCommit
	}
	if string(result.CredentialID) != string(current.Credential.ID) || string(result.UserID) != account {
		return errCommit
	}
	current.Credential.SignCount = result.SignCount
	current.Credential.BackupState = result.BackupState
	current.Version++
	db.credentials[string(current.Credential.ID)] = current
	delete(db.pending, p.State.FlowID)
	db.sessions[sessionID] = session{Account: account, Generation: generation, Created: now}
	return nil
}
func main() {
	rp, e := passkeys.New(passkeys.Config{RPID: "localhost", RPName: "Passkeys demo", Origins: []string{"http://localhost:8080"}, AllowLocalhost: true})
	if e != nil {
		log.Fatal(e)
	}
	app := &server{rp: rp, db: newStore(), now: time.Now}
	srv := &http.Server{Addr: "127.0.0.1:8080", Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute}
	log.Print("Experimental demo: http://localhost:8080 (all data disappears on restart)")
	log.Fatal(srv.ListenAndServe())
}
