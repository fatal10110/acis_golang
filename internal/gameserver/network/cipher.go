package network

import (
	"encoding/binary"
	"fmt"
)

// keySize is the fixed length of both the inbound and outbound rolling keys.
const keySize = 16

var gameCipherStaticKey = [8]byte{0xc8, 0x27, 0x93, 0x01, 0xa1, 0x6c, 0x31, 0x97}

// Cipher is the XOR rolling-stream cipher used for game client<->server
// packets. Encrypt and Decrypt use separate keys seeded from the same
// initial bytes, each rolled forward by the size of every packet processed.
// Encryption stays off until the first Encrypt call, which arms the cipher
// without transforming its input — so the first outbound packet (which
// carries the key itself) is sent in cleartext, and any packet decrypted
// before that point is left untouched too.
type Cipher struct {
	inKey, outKey [keySize]byte
	enabled       bool
}

// NewCipher builds a Cipher whose inbound and outbound keys both start from
// key, which must be exactly 16 bytes.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("game cipher key must be %d bytes, got %d", keySize, len(key))
	}
	c := &Cipher{}
	copy(c.inKey[:], key)
	copy(c.outKey[:], key)
	return c, nil
}

// Decrypt transforms buf in place using the inbound key, or leaves it
// untouched if the cipher has not been armed yet by a call to Encrypt.
func (c *Cipher) Decrypt(buf []byte) {
	if !c.enabled {
		return
	}
	var prev byte
	for i, b := range buf {
		buf[i] = b ^ c.inKey[i&15] ^ prev
		prev = b
	}
	rollKey(&c.inKey, len(buf))
}

// Encrypt transforms buf in place using the outbound key. The first call
// arms the cipher and returns with buf untouched; every call after that
// encrypts.
func (c *Cipher) Encrypt(buf []byte) {
	if !c.enabled {
		c.enabled = true
		return
	}
	var prev byte
	for i, b := range buf {
		prev = b ^ c.outKey[i&15] ^ prev
		buf[i] = prev
	}
	rollKey(&c.outKey, len(buf))
}

// rollKey advances key's bytes 8..11, read as a little-endian uint32, by
// size — the key roll every GameCrypt call applies after processing a
// packet.
func rollKey(key *[keySize]byte, size int) {
	old := binary.LittleEndian.Uint32(key[8:12])
	binary.LittleEndian.PutUint32(key[8:12], old+uint32(size))
}
