package passkeys

import (
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestRegistrationExtensionNaN(t *testing.T) {
	for _, tc := range []struct {
		name string
		ext  []byte
		want error
	}{
		{"duplicate NaN keys", []byte{0xa1, 0x61, 'x', 0xa2, 0xf9, 0x7e, 0x00, 0x00, 0xf9, 0x7e, 0x00, 0x01}, ErrInvalidInput},
		{"NaN value", []byte{0xa1, 0x61, 'x', 0xf9, 0x7e, 0x00}, ErrInvalidInput},
		{"unknown extension", []byte{0xa1, 0x61, 'x', 0xa1, 0x61, 'y', 0x01}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rp := testRP(t)
			state, response, _ := registration(t, rp)
			outer, resp := responseMap(t, response)
			var att map[string]any
			if err := cbor.Unmarshal(unbase(t, resp["attestationObject"].(string)), &att); err != nil {
				t.Fatal(err)
			}
			auth := att["authData"].([]byte)
			auth[32] |= 0x80
			att["authData"] = append(auth, tc.ext...)
			resp["attestationObject"] = encode(cborBytes(t, att))
			credential, err := rp.FinishRegistration(state, "flow", jsonBytes(t, outer))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.want != nil && len(credential.ID) != 0 {
				t.Fatalf("partial credential: %#v", credential)
			}
			if tc.want == nil && len(credential.ID) == 0 {
				t.Fatal("missing credential")
			}
		})
	}
}
