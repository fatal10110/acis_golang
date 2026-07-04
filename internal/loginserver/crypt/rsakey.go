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
	return &LoginKeyPair{
		Private:          priv,
		ScrambledModulus: scrambleModulus(priv.PublicKey.N),
	}, nil
}

// scrambleModulus obfuscates a 1024-bit RSA public modulus the way the L2
// client expects before it is sent over the wire: swap the first and last
// 4 bytes of the buffer, XOR the first half against the second half, XOR
// 4 bytes at offset 0x0d against 4 bytes at offset 0x34, then XOR the
// second half against the (now-modified) first half.
func scrambleModulus(modulus *big.Int) []byte {
	b := modulus.Bytes()
	if len(b) != modulusSize {
		panic(fmt.Sprintf("scrambleModulus: modulus is %d bytes, want %d (not a 1024-bit RSA modulus)", len(b), modulusSize))
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
	return scrambled
}
