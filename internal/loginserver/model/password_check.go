package model

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/crypto/blowfish"
)

// ErrUnusableHash reports a stored password hash that cannot be checked at
// all: not a "$2$" or "$2a$" bcrypt string, a cost outside 4-30, a salt that
// is too short or not bcrypt base64, or an empty password against a "$2$"
// hash, whose key would be empty.
var ErrUnusableHash = errors.New("unusable password hash")

// bcryptEncoding is bcrypt's unpadded base64 alphabet.
var bcryptEncoding = base64.NewEncoding("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789").WithPadding(base64.NoPadding)

const (
	bcryptSaltLen    = 16
	bcryptSaltChars  = 22
	bcryptMinCost    = 4
	bcryptMaxCost    = 30
	bcryptHashedSize = 23
)

// CheckPassword reports whether password matches hash, a stored bcrypt hash
// of revision "$2$" or "$2a$". It rehashes password with the stored cost and
// salt and compares the whole canonical string, so a hash with trailing or
// missing hash characters, a non-canonical salt, or a cost written as "+4"
// is a mismatch rather than an error. Revision "$2a$" keys bcrypt with the
// password and its NUL terminator, "$2$" with the password alone. Any hash
// it cannot use returns an error wrapping ErrUnusableHash.
func CheckPassword(hash, password string) (bool, error) {
	if len(hash) < 3 || hash[0] != '$' || hash[1] != '2' {
		return false, fmt.Errorf("%w: invalid version", ErrUnusableHash)
	}
	off, nulTerminated := 3, false
	if hash[2] != '$' {
		if hash[2] != 'a' || len(hash) < 4 || hash[3] != '$' {
			return false, fmt.Errorf("%w: invalid revision", ErrUnusableHash)
		}
		off, nulTerminated = 4, true
	}
	if len(hash) < off+3+bcryptSaltChars || hash[off+2] > '$' {
		return false, fmt.Errorf("%w: missing cost or salt", ErrUnusableHash)
	}
	cost, err := strconv.Atoi(hash[off : off+2])
	if err != nil || cost < bcryptMinCost || cost > bcryptMaxCost {
		return false, fmt.Errorf("%w: invalid cost %q", ErrUnusableHash, hash[off:off+2])
	}
	salt, ok := decodeBcryptSalt(hash[off+3 : off+3+bcryptSaltChars])
	if !ok {
		return false, fmt.Errorf("%w: invalid salt", ErrUnusableHash)
	}

	key := []byte(password)
	if nulTerminated {
		key = append(key, 0)
	}
	cipher, err := blowfish.NewSaltedCipher(key, salt)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrUnusableHash, err)
	}
	for range 1 << cost {
		blowfish.ExpandKey(key, cipher)
		blowfish.ExpandKey(salt, cipher)
	}
	data := []byte("OrpheanBeholderScryDoubt")
	for i := 0; i < len(data); i += 8 {
		for range 64 {
			cipher.Encrypt(data[i:i+8], data[i:i+8])
		}
	}

	want := fmt.Sprintf("%s%02d$%s%s", hash[:off], cost, bcryptEncoding.EncodeToString(salt), bcryptEncoding.EncodeToString(data[:bcryptHashedSize]))
	return subtle.ConstantTimeCompare([]byte(want), []byte(hash)) == 1, nil
}

// decodeBcryptSalt decodes the 22 salt characters of a bcrypt hash into its
// 16 salt bytes. A character outside the alphabet fails the decode, except
// in the last place of a four-character group, where it decodes as all one
// bits: the rehashed salt then differs, so such a hash only ever mismatches.
func decodeBcryptSalt(s string) ([]byte, bool) {
	out := make([]byte, 0, bcryptSaltLen)
	for i := 0; len(out) < bcryptSaltLen; i += 4 {
		c1, c2 := bcryptChar(s[i]), bcryptChar(s[i+1])
		if c1 < 0 || c2 < 0 {
			return nil, false
		}
		out = append(out, byte(c1<<2|(c2&0x30)>>4))
		if len(out) == bcryptSaltLen {
			break
		}
		c3 := bcryptChar(s[i+2])
		if c3 < 0 {
			return nil, false
		}
		out = append(out, byte((c2&0x0f)<<4|(c3&0x3c)>>2))
		c4 := bcryptChar(s[i+3])
		out = append(out, byte((c3&0x03)<<6|c4))
	}
	return out, true
}

// bcryptChar returns the 6-bit value of c in bcrypt's base64 alphabet, or -1.
func bcryptChar(c byte) int {
	switch {
	case c == '.':
		return 0
	case c == '/':
		return 1
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 2
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 28
	case c >= '0' && c <= '9':
		return int(c-'0') + 54
	}
	return -1
}
