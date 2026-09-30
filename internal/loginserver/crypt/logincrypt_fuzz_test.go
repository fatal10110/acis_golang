package crypt

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// FuzzLoginCryptDecrypt feeds the login listener's inbound decryption an
// arbitrary frame payload. It must reject or accept it without panicking or
// allocating, and a payload the session itself sealed must decrypt back to
// its plaintext with a valid checksum.
func FuzzLoginCryptDecrypt(f *testing.F) {
	key := mustHex(f, "5f3b3d2a1c0e4b6a79881726354453bf")
	f.Add([]byte{})
	f.Add(make([]byte, crypt.BlockSize-1))
	f.Add(make([]byte, crypt.BlockSize))
	f.Add([]byte{0x07, 0x6d, 0x5c, 0x4b, 0x3a, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // AuthGameGuard plaintext
	f.Fuzz(func(t *testing.T, data []byte) {
		lc, err := NewLoginCrypt(key)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, len(data))
		decodefuzz.Bounded(t, "LoginCrypt.Decrypt", data, func() {
			copy(buf, data)
			_ = lc.Decrypt(buf)
		})

		lc.Encrypt(nil) // the first outbound packet uses the static key
		sealed := lc.Encrypt(data)
		if err := lc.Decrypt(sealed); err != nil {
			t.Fatalf("Decrypt(Encrypt(% x)): %v", data, err)
		}
		if !bytes.Equal(sealed[:len(data)], data) {
			t.Fatalf("Decrypt(Encrypt(% x)) = % x", data, sealed[:len(data)])
		}
	})
}
