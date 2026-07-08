package crypt

import (
	"fmt"

	"golang.org/x/crypto/blowfish"
)

// BlockSize is the Blowfish block size in bytes.
const BlockSize = 8

// BlowfishCipher encrypts and decrypts 8-byte blocks with Blowfish in ECB
// mode, reading and writing each 4-byte half of a block least-significant
// byte first rather than the standard big-endian order. This exact byte
// order is required to interoperate with the L2 client's key exchange.
type BlowfishCipher struct {
	block *blowfish.Cipher
}

// NewBlowfishCipher builds a cipher from key, which must be 1-56 bytes.
func NewBlowfishCipher(key []byte) (*BlowfishCipher, error) {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("blowfish key: %w", err)
	}
	return &BlowfishCipher{block: block}, nil
}

// Encrypt encrypts the 8-byte block src into dst. dst and src may overlap
// exactly or not at all.
func (c *BlowfishCipher) Encrypt(dst, src []byte) {
	var swapped [BlockSize]byte
	swapHalves(swapped[:], src)
	c.block.Encrypt(swapped[:], swapped[:])
	swapHalves(dst, swapped[:])
}

// Decrypt decrypts the 8-byte block src into dst. dst and src may overlap
// exactly or not at all.
func (c *BlowfishCipher) Decrypt(dst, src []byte) {
	var swapped [BlockSize]byte
	swapHalves(swapped[:], src)
	c.block.Decrypt(swapped[:], swapped[:])
	swapHalves(dst, swapped[:])
}

// swapHalves reverses the byte order within each 4-byte half of an 8-byte
// block, converting between the client's least-significant-byte-first
// convention and Blowfish's standard big-endian word order.
func swapHalves(dst, src []byte) {
	dst[0], dst[1], dst[2], dst[3] = src[3], src[2], src[1], src[0]
	dst[4], dst[5], dst[6], dst[7] = src[7], src[6], src[5], src[4]
}
