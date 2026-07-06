package crypt

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestDecryptDynamicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, modulusSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
		want      []byte // nil means same as plaintext
	}{
		{name: "16-byte key", plaintext: []byte{0x03, 0x0a, 0x11, 0x18, 0x1f, 0x26, 0x2d, 0x34, 0x3b, 0x42, 0x49, 0x50, 0x57, 0x5e, 0x65, 0x6c}},
		{name: "single byte key", plaintext: []byte{0x2a}},
		// A leading zero byte in the key itself is indistinguishable from the
		// RSA block's own zero padding and gets stripped along with it -
		// matching the reference implementation's identical loop over the
		// decrypted block.
		{name: "leading zero byte in key is stripped like padding", plaintext: []byte{0x00, 0x01, 0x02, 0x03}, want: []byte{0x01, 0x02, 0x03}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = tt.plaintext
			}
			ciphertext := EncryptDynamicKey(&key.PublicKey, tt.plaintext)
			got := DecryptDynamicKey(key, ciphertext)
			if !bytes.Equal(got, want) {
				t.Fatalf("DecryptDynamicKey() = %x, want %x", got, want)
			}
		})
	}
}

func TestDecryptDynamicKeyEmptyPlaintext(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, modulusSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ciphertext := EncryptDynamicKey(&key.PublicKey, nil)
	got := DecryptDynamicKey(key, ciphertext)
	if len(got) != 0 {
		t.Fatalf("DecryptDynamicKey() = %x, want empty", got)
	}
}

func TestDecryptDynamicKeyIntoSetKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, modulusSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dynamicKey := mustHex(t, "030a11181f262d343b424950575e656c")
	ciphertext := EncryptDynamicKey(&key.PublicKey, dynamicKey)

	enc := NewLinkCrypt()
	if err := enc.SetKey(DecryptDynamicKey(key, ciphertext)); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	dec := NewLinkCrypt()
	if err := dec.SetKey(dynamicKey); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	payload := []byte("hello, gameserver link")
	got := enc.Encrypt(payload)
	if err := dec.Decrypt(got); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got[:len(payload)], payload) {
		t.Fatalf("round trip = %x, want prefix %x", got, payload)
	}
}
