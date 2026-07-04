package clientpackets

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"
)

func TestDecodeRequestAuthLogin(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, credentialBlockSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var block [credentialBlockSize]byte
	copy(block[usernameOffset:], "TestUser\x00\x00\x00\x00\x00\x00")
	copy(block[passwordOffset:], "s3cr3t   \x00\x00\x00\x00\x00\x00\x00")

	payload := append([]byte{OpcodeRequestAuthLogin}, encryptBlock(t, &key.PublicKey, block[:])...)

	got, err := DecodeRequestAuthLogin(payload, key)
	if err != nil {
		t.Fatalf("DecodeRequestAuthLogin: %v", err)
	}
	want := RequestAuthLogin{Username: "testuser", Password: "s3cr3t"}
	if got != want {
		t.Errorf("DecodeRequestAuthLogin = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAuthLoginShort(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, credentialBlockSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := DecodeRequestAuthLogin(make([]byte, 10), key); err == nil {
		t.Error("DecodeRequestAuthLogin: want error on short payload, got nil")
	}
}

func TestTrimControlBytes(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"no padding", []byte("hello"), "hello"},
		{"null padded", []byte("hello\x00\x00\x00"), "hello"},
		{"space padded both ends", []byte("  hello  "), "hello"},
		{"all blank", []byte("\x00\x00 \x00"), ""},
		{"empty", []byte{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimControlBytes(tt.in); got != tt.want {
				t.Errorf("trimControlBytes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// encryptBlock RSA-encrypts a full-size block with no padding scheme,
// mirroring how the client encrypts the credential block: c = m^e mod n.
func encryptBlock(t *testing.T, pub *rsa.PublicKey, plaintext []byte) []byte {
	t.Helper()
	m := new(big.Int).SetBytes(plaintext)
	c := new(big.Int).Exp(m, big.NewInt(int64(pub.E)), pub.N)
	out := make([]byte, credentialBlockSize)
	c.FillBytes(out)
	return out
}
