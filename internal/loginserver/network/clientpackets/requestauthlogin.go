package clientpackets

import (
	"crypto/rsa"
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

// RequestAuthLogin carries the login/password the client encrypted with the
// session's RSA public key.
type RequestAuthLogin struct {
	Username string
	Password string
}

// DecodeRequestAuthLogin parses a raw RequestAuthLogin payload (opcode byte
// included), RSA-decrypting the embedded credential block with the
// session's private key and extracting login/password from their fixed
// offsets in the decrypted block.
func DecodeRequestAuthLogin(payload []byte, key *rsa.PrivateKey) (RequestAuthLogin, error) {
	r := newReader(payload)
	if r.Remaining() < credentialBlockSize {
		return RequestAuthLogin{}, fmt.Errorf("clientpackets: RequestAuthLogin: need %d bytes, got %d", credentialBlockSize, r.Remaining())
	}
	block := decryptCredentialBlock(key, r.ReadBytes(credentialBlockSize))
	return RequestAuthLogin{
		Username: strings.ToLower(trimControlBytes(block[usernameOffset : usernameOffset+usernameSize])),
		Password: trimControlBytes(block[passwordOffset : passwordOffset+passwordSize]),
	}, nil
}

// decryptCredentialBlock decrypts an RSA block with no padding scheme (the
// client encrypts it the same way): m = c^d mod n. The result always fits
// the modulus size since m < n regardless of the input, so this never
// truncates.
func decryptCredentialBlock(key *rsa.PrivateKey, ciphertext []byte) [credentialBlockSize]byte {
	c := new(big.Int).SetBytes(ciphertext)
	m := new(big.Int).Exp(c, key.D, key.N)
	var out [credentialBlockSize]byte
	m.FillBytes(out[:])
	return out
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
