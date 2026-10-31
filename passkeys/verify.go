package passkeys

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"slices"
)

func (rp *RP) checkAuth(auth authData) error {
	hash := sha256.Sum256([]byte(rp.config.RPID))
	if !bytes.Equal(hash[:], auth.rpHash) {
		return ErrContext
	}
	if auth.flags&0x05 != 0x05 {
		return ErrPolicy
	}
	return nil
}

// FinishRegistration verifies a none attestation. It does not prove credential
// uniqueness, resident-key storage, or the right to add a key to an account.
func (rp *RP) FinishRegistration(state State, flowID string, response []byte) (Credential, error) {
	if err := rp.checkState(state, flowID, "webauthn.create"); err != nil {
		return Credential{}, err
	}
	env, err := parseEnvelope(response)
	if err != nil {
		return Credential{}, err
	}
	if _, err := rp.clientData(env.response, state); err != nil {
		return Credential{}, err
	}
	encoded, err := binaryField(env.response, "attestationObject", maxResponse)
	if err != nil {
		return Credential{}, err
	}
	var att map[any]any
	if err := rp.cbor.Unmarshal(encoded, &att); err != nil || att == nil {
		return Credential{}, ErrInvalidInput
	}
	fmt, ok := att["fmt"].(string)
	if !ok {
		return Credential{}, ErrInvalidInput
	}
	if fmt != "none" {
		return Credential{}, ErrUnsupported
	}
	stmt, ok := att["attStmt"].(map[any]any)
	if !ok || stmt == nil || len(stmt) != 0 {
		return Credential{}, ErrInvalidInput
	}
	raw, ok := att["authData"].([]byte)
	if !ok {
		return Credential{}, ErrInvalidInput
	}
	auth, err := rp.parseAuth(raw, true)
	if err != nil {
		return Credential{}, err
	}
	if err := rp.checkAuth(auth); err != nil {
		return Credential{}, err
	}
	if !bytes.Equal(env.id, auth.id) || hasID(state.CredentialIDs, auth.id) {
		return Credential{}, ErrCredential
	}
	if _, err := rp.publicKey(auth.key); err != nil {
		return Credential{}, err
	}
	transports, err := parseTransports(env.response)
	if err != nil {
		return Credential{}, err
	}
	return Credential{ID: slices.Clone(auth.id), UserID: slices.Clone(state.UserID),
		PublicKey: slices.Clone(auth.key), Algorithm: -7, SignCount: auth.count,
		BackupEligible: auth.flags&8 != 0, BackupState: auth.flags&16 != 0,
		AAGUID: auth.aaguid, Transports: transports}, nil
}

func parseTransports(response map[string]json.RawMessage) ([]string, error) {
	if _, exists := response["transports"]; !exists {
		return nil, nil
	}
	values, err := field[[]string](response, "transports")
	if err != nil || len(values) > 16 {
		return nil, ErrInvalidInput
	}
	for _, value := range values {
		if !validText(value) {
			return nil, ErrInvalidInput
		}
	}
	return values, nil
}

func hasID(ids [][]byte, id []byte) bool {
	for _, allowed := range ids {
		if bytes.Equal(allowed, id) {
			return true
		}
	}
	return false
}

// FinishLogin checks the signature over the original response bytes and returns
// proposed metadata. Calling it again with the same arguments may succeed:
// only the application's transaction can enforce single-use challenges.
func (rp *RP) FinishLogin(state State, flowID string, credential Credential, response []byte) (LoginResult, error) {
	if err := rp.checkState(state, flowID, "webauthn.get"); err != nil {
		return LoginResult{}, err
	}
	if !validID(credential.ID) || !validUser(credential.UserID) || credential.BackupState && !credential.BackupEligible {
		return LoginResult{}, ErrCredential
	}
	if credential.Algorithm != -7 {
		return LoginResult{}, ErrUnsupported
	}
	env, err := parseEnvelope(response)
	if err != nil {
		return LoginResult{}, err
	}
	if !bytes.Equal(env.id, credential.ID) {
		return LoginResult{}, ErrCredential
	}
	if len(state.UserID) != 0 && (!bytes.Equal(state.UserID, credential.UserID) || !hasID(state.CredentialIDs, credential.ID)) {
		return LoginResult{}, ErrCredential
	}
	if err := checkHandle(env.response, state, credential); err != nil {
		return LoginResult{}, err
	}
	client, err := rp.clientData(env.response, state)
	if err != nil {
		return LoginResult{}, err
	}
	raw, err := binaryField(env.response, "authenticatorData", maxAuthData)
	if err != nil {
		return LoginResult{}, err
	}
	auth, err := rp.parseAuth(raw, false)
	if err != nil {
		return LoginResult{}, err
	}
	if err := rp.checkAuth(auth); err != nil {
		return LoginResult{}, err
	}
	if (auth.flags&8 != 0) != credential.BackupEligible {
		return LoginResult{}, ErrCredential
	}
	key, err := rp.publicKey(credential.PublicKey)
	if err != nil {
		return LoginResult{}, err
	}
	signature, err := binaryField(env.response, "signature", 80)
	if err != nil {
		return LoginResult{}, err
	}
	clientHash := sha256.Sum256(client)
	signed := make([]byte, 0, len(raw)+len(clientHash))
	signed = append(signed, raw...)
	signed = append(signed, clientHash[:]...)
	digest := sha256.Sum256(signed)
	if !ecdsa.VerifyASN1(key, digest[:], signature) {
		return LoginResult{}, ErrCredential
	}
	return LoginResult{CredentialID: slices.Clone(credential.ID), UserID: slices.Clone(credential.UserID),
		SignCount: auth.count, BackupState: auth.flags&16 != 0,
		CounterAnomaly: (auth.count != 0 || credential.SignCount != 0) && auth.count <= credential.SignCount}, nil
}

func checkHandle(response map[string]json.RawMessage, state State, credential Credential) error {
	raw, exists := response["userHandle"]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if len(state.UserID) == 0 {
			return ErrCredential
		}
		return nil
	}
	handle, err := binaryField(response, "userHandle", 64)
	if err != nil {
		return err
	}
	if !bytes.Equal(handle, credential.UserID) {
		return ErrCredential
	}
	return nil
}
