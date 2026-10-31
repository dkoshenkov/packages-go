// Standalone interoperability check; run in the temporary module described in
// ../README.md. go-webauthn is never a dependency of the passkeys module.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dkoshenkov/packages-go/passkeys"
	"github.com/go-webauthn/webauthn/protocol"
	reference "github.com/go-webauthn/webauthn/webauthn"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 {
		return errors.New("usage: interop fixture.json")
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var f struct {
		Challenge, UserID   string
		Registration, Login json.RawMessage
	}
	if err = json.Unmarshal(data, &f); err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(f.Registration)
	if err != nil {
		return err
	}
	origins := []string{"https://example.com"}
	hash, err := parsed.Verify(f.Challenge, "example.com", origins, nil, nil, protocol.TopOriginDefaultVerificationMode, false, true, true, nil, []protocol.CredentialParameter{{Type: protocol.PublicKeyCredentialType, Algorithm: -7}}, protocol.AttestationPolicy{}, protocol.SignaturePolicy{})
	if err != nil {
		return fmt.Errorf("reference registration: %w", err)
	}
	legacy, err := reference.NewCredential(hash, parsed)
	if err != nil {
		return err
	}
	// Exercise the actual reference storage JSON format before mapping fields.
	persisted, err := json.Marshal(legacy)
	if err != nil {
		return err
	}
	var restored reference.Credential
	if err = json.Unmarshal(persisted, &restored); err != nil {
		return err
	}
	assertion, err := protocol.ParseCredentialRequestResponseBytes(f.Login)
	if err != nil {
		return err
	}
	if err = assertion.Verify(f.Challenge, "example.com", "", origins, nil, nil, protocol.TopOriginDefaultVerificationMode, false, true, true, restored.PublicKey, protocol.SignaturePolicy{}); err != nil {
		return fmt.Errorf("reference login: %w", err)
	}
	user, err := base64.RawURLEncoding.DecodeString(f.UserID)
	if err != nil {
		return err
	}
	challenge, err := base64.RawURLEncoding.DecodeString(f.Challenge)
	if err != nil {
		return err
	}
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	rp, err := passkeys.New(passkeys.Config{RPID: "example.com", RPName: "Example", Origins: origins, Now: func() time.Time { return now }})
	if err != nil {
		return err
	}
	_, state, err := rp.BeginLogin(passkeys.LoginRequest{FlowID: "migration"})
	if err != nil {
		return err
	}
	state.Challenge = challenge
	imported := passkeys.Credential{ID: restored.ID, UserID: user, PublicKey: restored.PublicKey, Algorithm: -7, SignCount: restored.Authenticator.SignCount, BackupEligible: restored.Flags.BackupEligible, BackupState: restored.Flags.BackupState}
	copy(imported.AAGUID[:], restored.Authenticator.AAGUID)
	result, err := rp.FinishLogin(state, "migration", imported, f.Login)
	if err != nil {
		return fmt.Errorf("imported login: %w", err)
	}
	if !bytes.Equal(result.UserID, user) || result.CounterAnomaly || result.SignCount != 2 {
		return errors.New("wrong imported result")
	}
	imported.Algorithm = -257
	if _, err = rp.FinishLogin(state, "migration", imported, f.Login); !errors.Is(err, passkeys.ErrUnsupported) {
		return errors.New("unsupported legacy algorithm accepted")
	}
	fmt.Println("PASS: go-webauthn v0.18.2 registration/assertion; credential JSON roundtrip; imported COSE key login; unsupported algorithm refusal")
	return nil
}
