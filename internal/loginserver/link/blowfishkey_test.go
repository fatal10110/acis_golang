package link

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/loginserver/crypt"
)

func TestDecodeBlowFishKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dynamicKey := []byte{0x03, 0x0a, 0x11, 0x18, 0x1f, 0x26, 0x2d, 0x34, 0x3b, 0x42, 0x49, 0x50, 0x57, 0x5e, 0x65, 0x6c}
	ciphertext := crypt.EncryptDynamicKey(&priv.PublicKey, dynamicKey)

	var payload []byte
	payload = append(payload, OpcodeBlowFishKey)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(ciphertext)))
	payload = append(payload, ciphertext...)

	got, err := DecodeBlowFishKey(payload, priv)
	if err != nil {
		t.Fatalf("DecodeBlowFishKey: %v", err)
	}
	if !bytes.Equal(got, dynamicKey) {
		t.Fatalf("DecodeBlowFishKey() = %x, want %x", got, dynamicKey)
	}
}

func TestDecodeBlowFishKeyShort(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	payload := []byte{OpcodeBlowFishKey, 0xff, 0xff, 0xff, 0x7f} // claims ~2GB of ciphertext
	if _, err := DecodeBlowFishKey(payload, priv); err == nil {
		t.Error("DecodeBlowFishKey: want error on truncated payload, got nil")
	}
}

func TestEncodeBlowFishKeyRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dynamicKey := []byte{0x03, 0x0a, 0x11, 0x18, 0x1f, 0x26, 0x2d, 0x34, 0x3b, 0x42, 0x49, 0x50, 0x57, 0x5e, 0x65, 0x6c}

	payload := EncodeBlowFishKey(&priv.PublicKey, dynamicKey)
	got, err := DecodeBlowFishKey(payload, priv)
	if err != nil {
		t.Fatalf("DecodeBlowFishKey(EncodeBlowFishKey()): %v", err)
	}
	if !bytes.Equal(got, dynamicKey) {
		t.Fatalf("round trip = %x, want %x", got, dynamicKey)
	}
}
