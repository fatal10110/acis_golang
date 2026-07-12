package crypt

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
)

// sessionKeySize is the length of the per-connection dynamic Blowfish key
// NewSessionKey generates.
const sessionKeySize = 16

// NewSessionKey returns a fresh random Blowfish key for one client
// connection's dynamic (post-Init) encryption.
func NewSessionKey() ([]byte, error) {
	key := make([]byte, sessionKeySize)
	if _, err := cryptorand.Read(key); err != nil {
		return nil, fmt.Errorf("generate session Blowfish key: %w", err)
	}
	return key, nil
}

// staticBootstrapKey is the fixed Blowfish key that encrypts the very first
// server-to-client login packet. That packet itself delivers the session's
// dynamic key, used for everything after.
var staticBootstrapKey = []byte{
	0x6b, 0x60, 0xcb, 0x5b, 0x82, 0xce, 0x90, 0xb1,
	0xcc, 0x2b, 0x6c, 0x55, 0x6c, 0x6c, 0x6c, 0x6c,
}

var staticBootstrapCipher = sync.OnceValues(func() (*crypt.BlowfishCipher, error) {
	return crypt.NewBlowfishCipher(staticBootstrapKey)
})

// LoginCrypt encrypts and decrypts the login server's client-facing packets.
// The very first outbound packet is padded, obfuscated with a rolling XOR
// pass, and encrypted with the static bootstrap key; every packet after that
// is padded, given an XOR checksum, and encrypted with the session's dynamic
// key. Inbound packets are always decrypted with the dynamic key and their
// checksum verified.
type LoginCrypt struct {
	static  *crypt.BlowfishCipher
	dynamic *crypt.BlowfishCipher
	first   bool
}

// NewLoginCrypt builds a LoginCrypt for one client session using its dynamic
// Blowfish key (1-56 bytes).
func NewLoginCrypt(dynamicKey []byte) (*LoginCrypt, error) {
	dynamic, err := crypt.NewBlowfishCipher(dynamicKey)
	if err != nil {
		return nil, fmt.Errorf("dynamic session cipher: %w", err)
	}
	static, err := staticBootstrapCipher()
	if err != nil {
		return nil, fmt.Errorf("static bootstrap cipher: %w", err)
	}
	// BlowfishCipher is immutable after construction, so the static first-packet cipher is safe to share.
	return &LoginCrypt{static: static, dynamic: dynamic, first: true}, nil
}

// Encrypt pads payload to a Blowfish block boundary and encrypts it,
// returning the (longer) encrypted packet body. The first call uses the
// static bootstrap key and a rolling XOR pass; every call after that uses
// the session's dynamic key and an appended checksum.
func (c *LoginCrypt) Encrypt(payload []byte) []byte {
	if c.first {
		c.first = false
		return c.encryptFirst(payload)
	}
	return c.encryptDynamic(payload)
}

func (c *LoginCrypt) encryptFirst(payload []byte) []byte {
	buf := make([]byte, crypt.PaddedSize(len(payload)+4+4))
	copy(buf, payload)
	xorPass(buf, rand.Uint32())
	crypt.EncryptBlocks(c.static, buf)
	return buf
}

func (c *LoginCrypt) encryptDynamic(payload []byte) []byte {
	buf := make([]byte, crypt.PaddedSize(len(payload)+4))
	copy(buf, payload)
	crypt.AppendChecksum(buf)
	crypt.EncryptBlocks(c.dynamic, buf)
	return buf
}

// Decrypt decrypts payload in place with the session's dynamic key and
// verifies its checksum, returning an error if payload's length is not a
// whole number of Blowfish blocks or the checksum does not match.
func (c *LoginCrypt) Decrypt(payload []byte) error {
	if len(payload) == 0 || len(payload)%crypt.BlockSize != 0 {
		return fmt.Errorf("login packet length %d is not a positive multiple of %d", len(payload), crypt.BlockSize)
	}
	crypt.DecryptBlocks(c.dynamic, payload)
	if !crypt.VerifyChecksum(payload) {
		return fmt.Errorf("login packet checksum verification failed")
	}
	return nil
}

// xorPass runs the rolling XOR pass the login server applies to the very
// first outbound packet: each 4-byte little-endian word starting at offset 4
// is XORed against a running key that accumulates the word's original
// value, and the final rolled key is stored in the last 4 bytes of buf.
// Requires len(buf) >= 8.
func xorPass(buf []byte, key uint32) {
	stop := len(buf) - 8
	ecx := key
	pos := 4
	for pos < stop {
		edx := binary.LittleEndian.Uint32(buf[pos : pos+4])
		ecx += edx
		edx ^= ecx
		binary.LittleEndian.PutUint32(buf[pos:pos+4], edx)
		pos += 4
	}
	binary.LittleEndian.PutUint32(buf[pos:pos+4], ecx)
}
