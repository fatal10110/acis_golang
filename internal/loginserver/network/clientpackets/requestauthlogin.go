package clientpackets

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// OpcodeRequestAuthLogin is the wire opcode for RequestAuthLogin, valid once
// the client has answered the GameGuard challenge.
const OpcodeRequestAuthLogin = 0x00

const (
	credentialBlockSize = 128 // raw RSA block; no padding scheme, block size == modulus size
	usernameOffset      = 0x5e
	usernameSize        = 14
	passwordOffset      = 0x6c
	passwordSize        = 16
)

var (
	// ErrCredentialBlockShort reports a RequestAuthLogin body too short to
	// hold the credential block.
	ErrCredentialBlockShort = errors.New("clientpackets: RequestAuthLogin: credential block truncated")
	// ErrCredentialBlockRange reports a credential block that is not below
	// the RSA modulus, so it cannot be a raw RSA ciphertext.
	ErrCredentialBlockRange = errors.New("clientpackets: RequestAuthLogin: credential block not below the RSA modulus")
)

// RequestAuthLogin carries the login/password the client encrypted with the
// session's RSA public key.
type RequestAuthLogin struct {
	Username string
	Password string
}

// DecodeRequestAuthLogin parses a raw RequestAuthLogin payload (opcode byte
// included), RSA-decrypting the embedded credential block with the
// session's private key and extracting login/password from their fixed
// offsets in the decrypted block. A body shorter than the block fails with
// ErrCredentialBlockShort, a block not below the modulus with
// ErrCredentialBlockRange.
func DecodeRequestAuthLogin(payload []byte, key *rsa.PrivateKey) (RequestAuthLogin, error) {
	r := newReader(payload)
	if r.Remaining() < credentialBlockSize {
		return RequestAuthLogin{}, fmt.Errorf("%w: need %d bytes, got %d", ErrCredentialBlockShort, credentialBlockSize, r.Remaining())
	}
	block, ok := decryptCredentialBlock(key, r.ReadBytes(credentialBlockSize))
	if !ok {
		return RequestAuthLogin{}, ErrCredentialBlockRange
	}
	return RequestAuthLogin{
		Username: strings.ToLower(trimControlBytes(block[usernameOffset : usernameOffset+usernameSize])),
		Password: trimControlBytes(block[passwordOffset : passwordOffset+passwordSize]),
	}, nil
}

// decryptCredentialBlock decrypts an RSA block with no padding scheme (the
// client encrypts it the same way): m = c^d mod n. It reports false for a
// ciphertext c >= n, which no encryption under the key can produce. The
// result always fits the modulus size since m < n, so this never truncates.
func decryptCredentialBlock(key *rsa.PrivateKey, ciphertext []byte) ([credentialBlockSize]byte, bool) {
	var out [credentialBlockSize]byte
	c := new(big.Int).SetBytes(ciphertext)
	if c.Cmp(key.N) >= 0 {
		return out, false
	}
	m := new(big.Int).Exp(c, key.D, key.N)
	m.FillBytes(out[:])
	return out, true
}

// trimControlBytes strips leading and trailing bytes with value <= 0x20
// (spaces and control characters). The client null-pads unused credential
// bytes with 0x00, which strings.TrimSpace would leave behind since it only
// recognizes Unicode whitespace, not the full control-byte range.
func trimControlBytes(b []byte) string {
	start := 0
	for start < len(b) && b[start] <= 0x20 {
		start++
	}
	end := len(b)
	for end > start && b[end-1] <= 0x20 {
		end--
	}
	return string(b[start:end])
}
