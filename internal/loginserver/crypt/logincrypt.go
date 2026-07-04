package crypt

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
)

// staticBootstrapKey is the fixed Blowfish key that encrypts the very first
// server-to-client login packet. That packet itself delivers the session's
// dynamic key, used for everything after.
var staticBootstrapKey = []byte{
	0x6b, 0x60, 0xcb, 0x5b, 0x82, 0xce, 0x90, 0xb1,
	0xcc, 0x2b, 0x6c, 0x55, 0x6c, 0x6c, 0x6c, 0x6c,
}

// LoginCrypt encrypts and decrypts the login server's client-facing packets.
// The very first outbound packet is padded, obfuscated with a rolling XOR
// pass, and encrypted with the static bootstrap key; every packet after that
// is padded, given an XOR checksum, and encrypted with the session's dynamic
// key. Inbound packets are always decrypted with the dynamic key and their
// checksum verified.
type LoginCrypt struct {
	static  *BlowfishCipher
	dynamic *BlowfishCipher
	first   bool
}

// NewLoginCrypt builds a LoginCrypt for one client session using its dynamic
// Blowfish key (1-56 bytes).
func NewLoginCrypt(dynamicKey []byte) (*LoginCrypt, error) {
	static, err := NewBlowfishCipher(staticBootstrapKey)
	if err != nil {
		return nil, fmt.Errorf("static bootstrap cipher: %w", err)
	}
	dynamic, err := NewBlowfishCipher(dynamicKey)
	if err != nil {
		return nil, fmt.Errorf("dynamic session cipher: %w", err)
	}
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
	buf := make([]byte, paddedSize(len(payload)+4+4))
	copy(buf, payload)
	xorPass(buf, rand.Uint32())
	encryptBlocks(c.static, buf)
	return buf
}

func (c *LoginCrypt) encryptDynamic(payload []byte) []byte {
	buf := make([]byte, paddedSize(len(payload)+4))
	copy(buf, payload)
	appendChecksum(buf)
	encryptBlocks(c.dynamic, buf)
	return buf
}

// Decrypt decrypts payload in place with the session's dynamic key and
// verifies its checksum, returning an error if payload's length is not a
// whole number of Blowfish blocks or the checksum does not match.
func (c *LoginCrypt) Decrypt(payload []byte) error {
	if len(payload) == 0 || len(payload)%BlockSize != 0 {
		return fmt.Errorf("login packet length %d is not a positive multiple of %d", len(payload), BlockSize)
	}
	decryptBlocks(c.dynamic, payload)
	if !verifyChecksum(payload) {
		return fmt.Errorf("login packet checksum verification failed")
	}
	return nil
}

// paddedSize rounds size up to the next Blowfish block boundary, always
// adding at least one byte of padding — even when size already sits on a
// boundary — to match the login protocol's packet sizing exactly.
func paddedSize(size int) int {
	return size + (BlockSize - size%BlockSize)
}

func encryptBlocks(c *BlowfishCipher, buf []byte) {
	for i := 0; i+BlockSize <= len(buf); i += BlockSize {
		c.Encrypt(buf[i:i+BlockSize], buf[i:i+BlockSize])
	}
}

func decryptBlocks(c *BlowfishCipher, buf []byte) {
	for i := 0; i+BlockSize <= len(buf); i += BlockSize {
		c.Decrypt(buf[i:i+BlockSize], buf[i:i+BlockSize])
	}
}

// appendChecksum XOR-folds every 4-byte little-endian word in buf except the
// last into the last word, so verifyChecksum on the same buf succeeds.
// Requires len(buf) to be a positive multiple of 4 greater than 4.
func appendChecksum(buf []byte) {
	var chksum uint32
	for i := 0; i < len(buf)-4; i += 4 {
		chksum ^= binary.LittleEndian.Uint32(buf[i : i+4])
	}
	binary.LittleEndian.PutUint32(buf[len(buf)-4:], chksum)
}

// verifyChecksum reports whether the last 4-byte little-endian word in buf
// equals the XOR-fold of every word before it. Returns false if len(buf) is
// not a multiple of 4 or is 4 or less.
func verifyChecksum(buf []byte) bool {
	if len(buf)%4 != 0 || len(buf) <= 4 {
		return false
	}
	var chksum uint32
	for i := 0; i < len(buf)-4; i += 4 {
		chksum ^= binary.LittleEndian.Uint32(buf[i : i+4])
	}
	return binary.LittleEndian.Uint32(buf[len(buf)-4:]) == chksum
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
