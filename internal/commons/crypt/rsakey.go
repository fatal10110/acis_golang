package crypt

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"math/big"
)

// modulusSize is the byte length of a 1024-bit RSA modulus.
const modulusSize = 128

// LoginKeyPair pairs an RSA private key with the scrambled form of its
// public modulus that the L2 client expects to receive during the key
// exchange handshake.
type LoginKeyPair struct {
	Private          *rsa.PrivateKey
	ScrambledModulus []byte
}

// NewLoginKeyPair generates a 1024-bit RSA key pair (public exponent
// 65537) and scrambles its modulus for transmission to the client.
func NewLoginKeyPair() (*LoginKeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, modulusSize*8)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	scrambled, err := scrambleModulus(priv.PublicKey.N)
	if err != nil {
		return nil, fmt.Errorf("scramble RSA modulus: %w", err)
	}
	return &LoginKeyPair{
		Private:          priv,
		ScrambledModulus: scrambled,
	}, nil
}

// ModulusBytes returns pub's modulus as a big-endian byte slice compatible
// with a reader that treats the bytes as signed two's-complement (as the L2
// game server does when reconstructing this login server's public key from
// InitLS): since an RSA modulus is always positive, this is pub.N's
// big-endian bytes with a leading 0x00 prepended whenever the top bit of the
// first byte is set, so a signed reader never mistakes the value for
// negative.
func ModulusBytes(pub *rsa.PublicKey) []byte {
	b := pub.N.Bytes()
	if len(b) > 0 && b[0]&0x80 != 0 {
		return append([]byte{0x00}, b...)
	}
	return b
}

// scrambleModulus obfuscates a 1024-bit RSA public modulus the way the L2
// client expects before it is sent over the wire: swap the first and last
// 4 bytes of the buffer, XOR the first half against the second half, XOR
// 4 bytes at offset 0x0d against 4 bytes at offset 0x34, then XOR the
// second half against the (now-modified) first half. Returns an error if
// modulus is not a 1024-bit RSA modulus.
func scrambleModulus(modulus *big.Int) ([]byte, error) {
	b := modulus.Bytes()
	if len(b) != modulusSize {
		return nil, fmt.Errorf("modulus is %d bytes, want %d (not a 1024-bit RSA modulus)", len(b), modulusSize)
	}

	scrambled := make([]byte, modulusSize)
	copy(scrambled, b)

	for i := 0; i < 4; i++ {
		scrambled[i], scrambled[0x4d+i] = scrambled[0x4d+i], scrambled[i]
	}
	for i := 0; i < 0x40; i++ {
		scrambled[i] ^= scrambled[0x40+i]
	}
	for i := 0; i < 4; i++ {
		scrambled[0x0d+i] ^= scrambled[0x34+i]
	}
	for i := 0; i < 0x40; i++ {
		scrambled[0x40+i] ^= scrambled[i]
	}
	return scrambled, nil
}

// UnscrambleModulus reverses the client-bound modulus scrambling and returns
// the login server's RSA public key (exponent 65537), as a client recovers it
// from Init. Returns an error if scrambled is not 128 bytes.
func UnscrambleModulus(scrambled []byte) (*rsa.PublicKey, error) {
	if len(scrambled) != modulusSize {
		return nil, fmt.Errorf("scrambled modulus is %d bytes, want %d", len(scrambled), modulusSize)
	}
	b := make([]byte, modulusSize)
	copy(b, scrambled)
	for i := 0; i < 0x40; i++ {
		b[0x40+i] ^= b[i]
	}
	for i := 0; i < 4; i++ {
		b[0x0d+i] ^= b[0x34+i]
	}
	for i := 0; i < 0x40; i++ {
		b[i] ^= b[0x40+i]
	}
	for i := 0; i < 4; i++ {
		b[i], b[0x4d+i] = b[0x4d+i], b[i]
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(b), E: 65537}, nil
}
