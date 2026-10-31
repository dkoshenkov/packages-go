// Package passkeys implements an experimental, ES256/none-only WebAuthn relying
// party. Applications own account authorization, storage and atomic consumption
// of State. A successful Finish call alone must never create a session.
package passkeys

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dkoshenkov/packages-go/consterr"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/net/publicsuffix"
)

// Stable error categories; errors never include browser payloads.
const (
	ErrInvalidInput consterr.Error = "passkeys: invalid input"
	ErrUnsupported  consterr.Error = "passkeys: unsupported feature"
	ErrExpired      consterr.Error = "passkeys: expired state"
	ErrContext      consterr.Error = "passkeys: incorrect operation context"
	ErrCredential   consterr.Error = "passkeys: invalid credential or signature"
	ErrPolicy       consterr.Error = "passkeys: user presence or verification required"
	ErrConfig       consterr.Error = "passkeys: invalid configuration"
)

const (
	maxResponse     = 64 << 10
	maxClientData   = 8 << 10
	maxAuthData     = 16 << 10
	maxCredentialID = 1023
	maxCredentials  = 64
	policy          = "passkeys-v1:ES256:none:UP:UV:resident-required"
)

// Config is copied by New. Now must be concurrency-safe if supplied.
type Config struct {
	RPID           string
	RPName         string
	Origins        []string
	Timeout        time.Duration    // Zero uses five minutes; minimum one millisecond.
	AllowLocalhost bool             // Allows http://localhost[:port], only for RP ID localhost.
	Now            func() time.Time // Nil uses time.Now.
}

// RP is immutable and safe for concurrent independent operations. Use New.
type RP struct {
	config  Config
	binding []byte
	cbor    cbor.DecMode
	random  io.Reader
}

// User.ID is a random opaque handle, 1–64 bytes, never an email address.
type User struct {
	ID          []byte
	Name        string
	DisplayName string
}

// CredentialDescriptor identifies an existing key. Transports are UI hints.
type CredentialDescriptor struct {
	ID         []byte
	Transports []string
}

type RegistrationRequest struct {
	FlowID  string
	User    User
	Exclude []CredentialDescriptor
}

// LoginRequest with no UserID and no Allow starts discoverable login.
// Known-user login requires both UserID and a nonempty Allow list.
type LoginRequest struct {
	FlowID string
	UserID []byte
	Allow  []CredentialDescriptor
}

// State is trusted server data. Store it with a browser-bound flow and never
// accept an unprotected copy from the client. JSON round trips are supported.
type State struct {
	Operation     string
	FlowID        string
	Challenge     []byte
	IssuedAt      time.Time
	ExpiresAt     time.Time
	Binding       []byte
	UserID        []byte
	CredentialIDs [][]byte
}

// Credential contains only public key material. PublicKey is the COSE_Key CBOR
// encoding (EC2/P-256/ES256), suitable for importing compatible stored keys.
type Credential struct {
	ID             []byte
	UserID         []byte
	PublicKey      []byte
	Algorithm      int
	SignCount      uint32
	BackupEligible bool
	BackupState    bool
	AAGUID         [16]byte
	Transports     []string
}

// LoginResult proposes updates; callers must commit them with challenge
// consumption and credential/account generation checks in one transaction.
type LoginResult struct {
	CredentialID   []byte
	UserID         []byte
	SignCount      uint32
	BackupState    bool
	CounterAnomaly bool
}

// Browser binary fields use unpadded base64url strings. These options match
// PublicKeyCredential.parseCreationOptionsFromJSON / parseRequestOptionsFromJSON.
type CreationOptions struct {
	RP                     RPEntity               `json:"rp"`
	User                   UserEntity             `json:"user"`
	Challenge              string                 `json:"challenge"`
	PubKeyCredParams       []CredentialParameter  `json:"pubKeyCredParams"`
	Timeout                int64                  `json:"timeout"`
	ExcludeCredentials     []Descriptor           `json:"excludeCredentials"`
	AuthenticatorSelection AuthenticatorSelection `json:"authenticatorSelection"`
	Attestation            string                 `json:"attestation"`
}

type RequestOptions struct {
	Challenge        string       `json:"challenge"`
	RPID             string       `json:"rpId"`
	Timeout          int64        `json:"timeout"`
	AllowCredentials []Descriptor `json:"allowCredentials,omitempty"`
	UserVerification string       `json:"userVerification"`
}

type RPEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type CredentialParameter struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

type Descriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

type AuthenticatorSelection struct {
	ResidentKey        string `json:"residentKey"`
	RequireResidentKey bool   `json:"requireResidentKey"`
	UserVerification   string `json:"userVerification"`
}

// New validates explicit RP/origin configuration without network access.
func New(config Config) (*RP, error) {
	if !validDomain(config.RPID) || !validText(config.RPName) || len(config.Origins) == 0 {
		return nil, ErrConfig
	}
	if config.RPID == "localhost" {
		if !config.AllowLocalhost {
			return nil, ErrConfig
		}
	} else {
		suffix, _ := publicsuffix.PublicSuffix(config.RPID)
		if suffix == config.RPID {
			return nil, ErrConfig
		}
	}
	config.Origins = slices.Clone(config.Origins)
	for _, origin := range config.Origins {
		if !validOrigin(config, origin) {
			return nil, ErrConfig
		}
	}
	slices.Sort(config.Origins)
	config.Origins = slices.Compact(config.Origins)
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.Timeout < time.Millisecond {
		return nil, ErrConfig
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	mode, err := cbor.DecOptions{
		DupMapKey:       cbor.DupMapKeyEnforcedAPF,
		MaxNestedLevels: 16, MaxArrayElements: 256, MaxMapPairs: 256,
		IndefLength: cbor.IndefLengthForbidden, TagsMd: cbor.TagsForbidden,
		NaN: cbor.NaNDecodeForbidden,
	}.DecMode()
	if err != nil {
		return nil, ErrConfig
	}
	// Bind persisted operations to the exact RP, origins and fixed policy.
	binding, err := json.Marshal([]any{policy, config.RPID, config.Origins, int64(config.Timeout), config.AllowLocalhost})
	if err != nil {
		return nil, ErrConfig
	}
	hash := sha256.Sum256(binding)
	return &RP{config: config, binding: hash[:], cbor: mode, random: rand.Reader}, nil
}

func validText(s string) bool  { return s != "" && len(s) <= 1024 && utf8.ValidString(s) }
func validUser(id []byte) bool { return len(id) > 0 && len(id) <= 64 }
func validID(id []byte) bool   { return len(id) > 0 && len(id) <= maxCredentialID }

func validDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 || net.ParseIP(domain) != nil {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func validOrigin(config Config, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.Opaque != "" || u.String() != origin {
		return false
	}
	host := u.Hostname()
	if !validDomain(host) || (host != config.RPID && !strings.HasSuffix(host, "."+config.RPID)) {
		return false
	}
	if u.Host != host {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() {
			return false
		}
		if u.Host != host+":"+u.Port() || u.Scheme == "https" && port == 443 || u.Scheme == "http" && port == 80 {
			return false
		}
	}
	return u.Scheme == "https" || config.AllowLocalhost && config.RPID == "localhost" && host == "localhost" && u.Scheme == "http"
}

func descriptors(values []CredentialDescriptor) ([]Descriptor, [][]byte, error) {
	if len(values) > maxCredentials {
		return nil, nil, ErrInvalidInput
	}
	options := make([]Descriptor, 0, len(values))
	ids := make([][]byte, 0, len(values))
	for _, value := range values {
		if !validID(value.ID) || len(value.Transports) > 16 {
			return nil, nil, ErrInvalidInput
		}
		for _, transport := range value.Transports {
			if !validText(transport) {
				return nil, nil, ErrInvalidInput
			}
		}
		options = append(options, Descriptor{Type: "public-key", ID: encode(value.ID), Transports: slices.Clone(value.Transports)})
		ids = append(ids, slices.Clone(value.ID))
	}
	return options, ids, nil
}

func encode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (rp *RP) begin(operation, flow string, user []byte, ids [][]byte) (State, error) {
	if !validText(flow) {
		return State{}, ErrInvalidInput
	}
	challenge := make([]byte, 32)
	if _, err := io.ReadFull(rp.random, challenge); err != nil {
		return State{}, consterr.Error("passkeys: challenge generation failed")
	}
	now := rp.config.Now()
	return State{Operation: operation, FlowID: flow, Challenge: challenge,
		IssuedAt: now, ExpiresAt: now.Add(rp.config.Timeout), Binding: slices.Clone(rp.binding),
		UserID: slices.Clone(user), CredentialIDs: ids}, nil
}

func (rp *RP) BeginRegistration(request RegistrationRequest) (CreationOptions, State, error) {
	if !validUser(request.User.ID) || !validText(request.User.Name) || !validText(request.User.DisplayName) {
		return CreationOptions{}, State{}, ErrInvalidInput
	}
	exclude, ids, err := descriptors(request.Exclude)
	if err != nil {
		return CreationOptions{}, State{}, err
	}
	state, err := rp.begin("webauthn.create", request.FlowID, request.User.ID, ids)
	if err != nil {
		return CreationOptions{}, State{}, err
	}
	options := CreationOptions{
		RP:        RPEntity{ID: rp.config.RPID, Name: rp.config.RPName},
		User:      UserEntity{ID: encode(request.User.ID), Name: request.User.Name, DisplayName: request.User.DisplayName},
		Challenge: encode(state.Challenge), PubKeyCredParams: []CredentialParameter{{Type: "public-key", Alg: -7}},
		Timeout: rp.config.Timeout.Milliseconds(), ExcludeCredentials: exclude,
		AuthenticatorSelection: AuthenticatorSelection{ResidentKey: "required", RequireResidentKey: true, UserVerification: "required"},
		Attestation:            "none",
	}
	return options, state, nil
}

func (rp *RP) BeginLogin(request LoginRequest) (RequestOptions, State, error) {
	if len(request.UserID) == 0 && len(request.Allow) != 0 || len(request.UserID) != 0 && (!validUser(request.UserID) || len(request.Allow) == 0) {
		return RequestOptions{}, State{}, ErrInvalidInput
	}
	allow, ids, err := descriptors(request.Allow)
	if err != nil {
		return RequestOptions{}, State{}, err
	}
	state, err := rp.begin("webauthn.get", request.FlowID, request.UserID, ids)
	if err != nil {
		return RequestOptions{}, State{}, err
	}
	return RequestOptions{Challenge: encode(state.Challenge), RPID: rp.config.RPID,
		Timeout: rp.config.Timeout.Milliseconds(), AllowCredentials: allow, UserVerification: "required"}, state, nil
}

func (rp *RP) checkState(state State, flow, operation string) error {
	if state.Operation != operation || !validText(flow) || state.FlowID != flow || len(state.Challenge) != 32 || !bytes.Equal(state.Binding, rp.binding) {
		return ErrContext
	}
	now := rp.config.Now()
	if !now.Before(state.ExpiresAt) {
		return ErrExpired
	}
	if now.Before(state.IssuedAt) || !state.ExpiresAt.Equal(state.IssuedAt.Add(rp.config.Timeout)) || len(state.CredentialIDs) > maxCredentials {
		return ErrContext
	}
	for _, id := range state.CredentialIDs {
		if !validID(id) {
			return ErrContext
		}
	}
	if operation == "webauthn.create" && !validUser(state.UserID) {
		return ErrContext
	}
	if operation == "webauthn.get" && (len(state.UserID) == 0 && len(state.CredentialIDs) != 0 || len(state.UserID) != 0 && (!validUser(state.UserID) || len(state.CredentialIDs) == 0)) {
		return ErrContext
	}
	return nil
}
