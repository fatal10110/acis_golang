package crypt

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// FuzzLinkCryptDecrypt feeds the GS-LS link's inbound decryption an
// arbitrary frame payload. It must reject or accept it without panicking or
// allocating, and a payload the link itself sealed must decrypt back to its
// plaintext with a valid checksum.
func FuzzLinkCryptDecrypt(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, BlockSize-1))
	f.Add(make([]byte, BlockSize))
	f.Add([]byte{0x02, 0x02, 0x00, 'a', 0, 0, 0, 'b', 0, 0, 0}) // PlayerInGame plaintext
	f.Fuzz(func(t *testing.T, data []byte) {
		c := NewLinkCrypt()
		buf := make([]byte, len(data))
		decodefuzz.Bounded(t, "LinkCrypt.Decrypt", data, func() {
			copy(buf, data)
			_ = c.Decrypt(buf)
		})

		sealed := c.Encrypt(data)
		if err := c.Decrypt(sealed); err != nil {
			t.Fatalf("Decrypt(Encrypt(% x)): %v", data, err)
		}
		if !bytes.Equal(sealed[:len(data)], data) {
			t.Fatalf("Decrypt(Encrypt(% x)) = % x", data, sealed[:len(data)])
		}
	})
}
