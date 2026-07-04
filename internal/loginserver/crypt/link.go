package crypt

import (
	"crypto/rsa"
	"fmt"
	"math/big"
)

// linkBootstrapKey is the fixed Blowfish key that protects the login-to-
// game-server link before the game server supplies its own session key.
var linkBootstrapKey = []byte{
	0x5f, 0x3b, 0x76, 0x2e, 0x5d, 0x30, 0x35, 0x2d,
	0x33, 0x31, 0x21, 0x7c, 0x2b, 0x2d, 0x25, 0x78,
	0x54, 0x21, 0x5e, 0x5b, 0x24, 0x00,
}

// LinkCrypt encrypts and decrypts packets on the login-to-game-server link.
// Every packet, inbound and outbound, is padded to a Blowfish block
// boundary, checksummed, and encrypted with the link's current key: the
// static bootstrap key until the game server supplies a session key via
// SetKey, and that session key for the rest of the connection.
type LinkCrypt struct {
	cipher *BlowfishCipher
}

// NewLinkCrypt builds a LinkCrypt using the link's static bootstrap key.
func NewLinkCrypt() *LinkCrypt {
	cipher, _ := NewBlowfishCipher(linkBootstrapKey) // fixed-length constant key, always valid
	return &LinkCrypt{cipher: cipher}
}

// DecryptDynamicKey RSA-decrypts an encrypted dynamic link key with no
// padding scheme (m = c^d mod n): the game server encrypts its chosen
// Blowfish key, zero-padded to the RSA modulus size, with the link's public
// key. big.Int.Bytes() drops that leading zero padding, yielding the raw
// key ready for SetKey.
func DecryptDynamicKey(priv *rsa.PrivateKey, ciphertext []byte) []byte {
	c := new(big.Int).SetBytes(ciphertext)
	m := new(big.Int).Exp(c, priv.D, priv.N)
	return m.Bytes()
}

// SetKey switches the link to a new Blowfish key (1-56 bytes), used once the
// game server supplies its own session key.
func (c *LinkCrypt) SetKey(key []byte) error {
	cipher, err := NewBlowfishCipher(key)
	if err != nil {
		return fmt.Errorf("link session key: %w", err)
	}
	c.cipher = cipher
	return nil
}

// Encrypt pads payload to a Blowfish block boundary, appends a checksum, and
// encrypts it with the link's current key, returning the (longer) encrypted
// packet body.
func (c *LinkCrypt) Encrypt(payload []byte) []byte {
	buf := make([]byte, paddedSize(len(payload)+4))
	copy(buf, payload)
	appendChecksum(buf)
	encryptBlocks(c.cipher, buf)
	return buf
}

// Decrypt decrypts payload in place with the link's current key and
// verifies its checksum, returning an error if payload's length is not a
// positive multiple of the Blowfish block size or the checksum does not
// match.
func (c *LinkCrypt) Decrypt(payload []byte) error {
	if len(payload) == 0 || len(payload)%BlockSize != 0 {
		return fmt.Errorf("link packet length %d is not a positive multiple of %d", len(payload), BlockSize)
	}
	decryptBlocks(c.cipher, payload)
	if !verifyChecksum(payload) {
		return fmt.Errorf("link packet checksum verification failed")
	}
	return nil
}
