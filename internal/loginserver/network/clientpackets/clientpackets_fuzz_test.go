package clientpackets

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"testing"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// loginDecoder is one login client packet decoder under fuzz, with a
// well-formed seed packet for it.
type loginDecoder struct {
	name   string
	decode func([]byte) error
	seed   []byte
}

func decodes[T any](decode func([]byte) (T, error)) func([]byte) error {
	return func(payload []byte) error {
		_, err := decode(payload)
		return err
	}
}

// decryptedPayload lays plaintext out the way the login listener hands it
// to a decoder after decryption: padded to a whole Blowfish block with the
// XOR checksum in the final word.
func decryptedPayload(plaintext []byte) []byte {
	buf := make([]byte, commoncrypt.PaddedSize(len(plaintext)+4))
	copy(buf, plaintext)
	commoncrypt.AppendChecksum(buf)
	return buf
}

func le32(values ...uint32) []byte {
	out := make([]byte, 0, 4*len(values))
	for _, v := range values {
		out = binary.LittleEndian.AppendUint32(out, v)
	}
	return out
}

// loginDecoders lists every login client packet decoder the login listener
// dispatches to. key is the session RSA key RequestAuthLogin decrypts with.
func loginDecoders(tb testing.TB, key *rsa.PrivateKey) []loginDecoder {
	var credentials [credentialBlockSize]byte
	copy(credentials[usernameOffset:], "testaccount")
	copy(credentials[passwordOffset:], "s3cr3t")
	authLogin := append([]byte{OpcodeRequestAuthLogin}, encryptBlock(tb, &key.PublicKey, credentials[:])...)

	return []loginDecoder{
		{"AuthGameGuard", decodes(DecodeAuthGameGuard), decryptedPayload(append([]byte{OpcodeAuthGameGuard}, le32(0x3a4b5c6d, 0, 0, 0, 0)...))},
		{"RequestAuthLogin", func(payload []byte) error {
			_, err := DecodeRequestAuthLogin(payload, key)
			return err
		}, decryptedPayload(authLogin)},
		{"RequestServerList", decodes(DecodeRequestServerList), decryptedPayload(append([]byte{OpcodeRequestServerList}, le32(0x11223344, 0x55667788)...))},
		{"RequestServerLogin", decodes(DecodeRequestServerLogin), decryptedPayload(append([]byte{OpcodeRequestServerLogin}, append(le32(0x11223344, 0x55667788), 1)...))},
	}
}

func newFuzzKey(tb testing.TB) *rsa.PrivateKey {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, credentialBlockSize*8)
	if err != nil {
		tb.Fatalf("GenerateKey: %v", err)
	}
	return key
}

// TestLoginDecoderFuzzSeedsDecode keeps the fuzz seeds honest: each one must
// be a packet its own decoder accepts.
func TestLoginDecoderFuzzSeedsDecode(t *testing.T) {
	for _, d := range loginDecoders(t, newFuzzKey(t)) {
		if err := d.decode(d.seed); err != nil {
			t.Errorf("%s seed % x: %v", d.name, d.seed, err)
		}
	}
}

// FuzzLoginClientPackets feeds every login client packet decoder the same
// arbitrary decrypted payload. Each must return rather than panic, and none
// may allocate more than the payload's length justifies.
func FuzzLoginClientPackets(f *testing.F) {
	decoders := loginDecoders(f, newFuzzKey(f))
	for _, d := range decoders {
		f.Add(d.seed)
		f.Add(d.seed[:len(d.seed)/2])
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		for _, d := range decoders {
			decodefuzz.Bounded(t, d.name, payload, func() { _ = d.decode(payload) })
		}
	})
}
