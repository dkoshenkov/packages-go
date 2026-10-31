package passkeys

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/big"
	"slices"
	"unicode/utf8"
)

// scanJSON validates every object, including ignored optional fields. Decoding
// into a struct directly would accept duplicate and case-insensitive keys.
func scanJSON(dec *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrInvalidInput
	}
	token, err := dec.Token()
	if err != nil {
		return ErrInvalidInput
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return ErrInvalidInput
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrInvalidInput
			}
			seen[name] = true
			if err := scanJSON(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := scanJSON(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidInput
	}
	if _, err := dec.Token(); err != nil {
		return ErrInvalidInput
	}
	return nil
}

func object(data []byte, limit int) (map[string]json.RawMessage, error) {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) {
		return nil, ErrInvalidInput
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanJSON(dec, 0); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, ErrInvalidInput
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil || value == nil {
		return nil, ErrInvalidInput
	}
	return value, nil
}

func field[T any](obj map[string]json.RawMessage, name string) (T, error) {
	var value T
	raw, ok := obj[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return value, ErrInvalidInput
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, ErrInvalidInput
	}
	return value, nil
}

func decode(value string, max int) ([]byte, error) {
	if len(value) == 0 || len(value) > base64.RawURLEncoding.EncodedLen(max) {
		return nil, ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	// DecodeString accepts CR/LF; canonical base64url does not.
	if err != nil || len(b) > max || encode(b) != value {
		return nil, ErrInvalidInput
	}
	return b, nil
}

func binaryField(obj map[string]json.RawMessage, name string, max int) ([]byte, error) {
	value, err := field[string](obj, name)
	if err != nil {
		return nil, err
	}
	return decode(value, max)
}

type envelope struct {
	id       []byte
	response map[string]json.RawMessage
}

func parseEnvelope(data []byte) (envelope, error) {
	obj, err := object(data, maxResponse)
	if err != nil {
		return envelope{}, err
	}
	typ, err := field[string](obj, "type")
	if err != nil || typ != "public-key" {
		return envelope{}, ErrInvalidInput
	}
	id, err := binaryField(obj, "id", maxCredentialID)
	if err != nil {
		return envelope{}, err
	}
	rawID, err := binaryField(obj, "rawId", maxCredentialID)
	if err != nil || !bytes.Equal(id, rawID) {
		return envelope{}, ErrInvalidInput
	}
	response, err := object(obj["response"], maxResponse)
	if err != nil {
		return envelope{}, err
	}
	if ext, ok := obj["clientExtensionResults"]; ok {
		if _, err := object(ext, maxResponse); err != nil {
			return envelope{}, err
		}
	}
	if _, ok := obj["authenticatorAttachment"]; ok && !bytes.Equal(bytes.TrimSpace(obj["authenticatorAttachment"]), []byte("null")) {
		if _, err := field[string](obj, "authenticatorAttachment"); err != nil {
			return envelope{}, err
		}
	}
	return envelope{id: id, response: response}, nil
}

// CredentialID extracts an UNVERIFIED lookup hint. Always call FinishLogin
// after loading the credential; never grant access based on this function.
func CredentialID(response []byte) ([]byte, error) {
	value, err := parseEnvelope(response)
	if err != nil {
		return nil, err
	}
	return value.id, nil
}

func (rp *RP) clientData(response map[string]json.RawMessage, state State) ([]byte, error) {
	raw, err := binaryField(response, "clientDataJSON", maxClientData)
	if err != nil {
		return nil, err
	}
	obj, err := object(raw, maxClientData)
	if err != nil {
		return nil, err
	}
	typ, err := field[string](obj, "type")
	if err != nil {
		return nil, err
	}
	challenge, err := binaryField(obj, "challenge", 32)
	if err != nil {
		return nil, err
	}
	origin, err := field[string](obj, "origin")
	if err != nil {
		return nil, err
	}
	if typ != state.Operation || !bytes.Equal(challenge, state.Challenge) || !slices.Contains(rp.config.Origins, origin) {
		return nil, ErrContext
	}
	if _, exists := obj["crossOrigin"]; exists {
		cross, err := field[bool](obj, "crossOrigin")
		if err != nil {
			return nil, err
		}
		if cross {
			return nil, ErrUnsupported
		}
	}
	if _, exists := obj["topOrigin"]; exists {
		top, err := field[string](obj, "topOrigin")
		if err != nil {
			return nil, err
		}
		if top != origin {
			return nil, ErrUnsupported
		}
	}
	return raw, nil
}

type authData struct {
	rpHash []byte
	flags  byte
	count  uint32
	aaguid [16]byte
	id     []byte
	key    []byte
}

func (rp *RP) parseAuth(data []byte, registration bool) (authData, error) {
	if len(data) < 37 || len(data) > maxAuthData {
		return authData{}, ErrInvalidInput
	}
	auth := authData{rpHash: data[:32], flags: data[32], count: binary.BigEndian.Uint32(data[33:37])}
	if auth.flags&0x10 != 0 && auth.flags&0x08 == 0 {
		return authData{}, ErrInvalidInput
	}
	if (auth.flags&0x40 != 0) != registration {
		return authData{}, ErrInvalidInput
	}
	rest := data[37:]
	if registration {
		if len(rest) < 18 {
			return authData{}, ErrInvalidInput
		}
		copy(auth.aaguid[:], rest[:16])
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		if n == 0 || n > maxCredentialID || len(rest) < 18+n {
			return authData{}, ErrInvalidInput
		}
		auth.id = rest[18 : 18+n]
		rest = rest[18+n:]
		var key map[any]any
		tail, err := rp.cbor.UnmarshalFirst(rest, &key)
		if err != nil || key == nil {
			return authData{}, ErrInvalidInput
		}
		auth.key = rest[:len(rest)-len(tail)]
		rest = tail
	}
	if auth.flags&0x80 != 0 {
		var extensions map[any]any
		if err := rp.cbor.Unmarshal(rest, &extensions); err != nil || extensions == nil {
			return authData{}, ErrInvalidInput
		}
		for key := range extensions {
			if _, ok := key.(string); !ok {
				return authData{}, ErrInvalidInput
			}
		}
		rest = nil
	}
	if len(rest) != 0 {
		return authData{}, ErrInvalidInput
	}
	// RFU bits are intentionally ignored (WebAuthn section 6.1).
	return auth, nil
}

func number(value any, expected int64) bool {
	switch value := value.(type) {
	case uint64:
		return expected >= 0 && value == uint64(expected)
	case int64:
		return value == expected
	default:
		return false
	}
}

func (rp *RP) publicKey(data []byte) (*ecdsa.PublicKey, error) {
	if len(data) == 0 || len(data) > maxAuthData {
		return nil, ErrCredential
	}
	var key map[any]any
	if err := rp.cbor.Unmarshal(data, &key); err != nil || key == nil {
		return nil, ErrInvalidInput
	}
	if !number(key[uint64(1)], 2) || !number(key[uint64(3)], -7) || !number(key[int64(-1)], 1) {
		return nil, ErrUnsupported
	}
	if _, private := key[int64(-4)]; private {
		return nil, ErrCredential
	}
	// WebAuthn 6.5.1 prohibits optional COSE parameters other than alg.
	if len(key) != 5 {
		return nil, ErrCredential
	}
	x, xok := key[int64(-2)].([]byte)
	y, yok := key[int64(-3)].([]byte)
	if !xok || !yok || len(x) != 32 || len(y) != 32 {
		return nil, ErrCredential
	}
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
		return nil, ErrCredential
	}
	return pub, nil
}
