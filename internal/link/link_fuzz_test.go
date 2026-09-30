package link

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// linkDecoder is one GS-LS link packet decoder under fuzz, with a
// well-formed seed packet for it.
type linkDecoder struct {
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

// oracleSeed decodes a hex packet vector (the same Java payloads
// TestJavaOracleLinkPacketVectors pins) into a seed.
func oracleSeed(tb testing.TB, vector string) []byte {
	tb.Helper()
	seed, err := hex.DecodeString(vector)
	if err != nil {
		tb.Fatalf("oracle vector %q: %v", vector, err)
	}
	return seed
}

// linkDecoders lists every link packet decoder either end of the GS-LS link
// dispatches to. key is the login server's link RSA key BlowFishKey
// decrypts with.
func linkDecoders(tb testing.TB, key *rsa.PrivateKey) []linkDecoder {
	maxPlayers := int32(1000)
	status := ServerStatus{MaxPlayers: &maxPlayers}
	return []linkDecoder{
		// Game server -> login server.
		{"BlowFishKey", func(payload []byte) error {
			_, err := DecodeBlowFishKey(payload, key)
			return err
		}, EncodeBlowFishKey(&key.PublicKey, []byte("0123456789abcdef0123456789abcdef0123456789"))},
		{"GameServerAuth", decodes(DecodeGameServerAuth), oracleSeed(tb, "010701003100320037002e0030002e0030002e0031000000611e6400000002000000aabb")},
		{"PlayerInGame", decodes(DecodePlayerInGame), oracleSeed(tb, "02020061006c00690063006500000062006f0062000000")},
		{"PlayerLogout", decodes(DecodePlayerLogout), oracleSeed(tb, "0361006c006900630065000000")},
		{"ChangeAccessLevel", decodes(DecodeChangeAccessLevel), oracleSeed(tb, "04ffffffff61006c006900630065000000")},
		{"PlayerAuthRequest", decodes(DecodePlayerAuthRequest), oracleSeed(tb, "0561006c00690063006500000001000000020000000300000004000000")},
		{"ServerStatus", decodes(DecodeServerStatus), EncodeServerStatus(status)},
		// Login server -> game server.
		{"InitLS", func(payload []byte) error {
			_, _, err := DecodeInitLS(payload)
			return err
		}, oracleSeed(tb, "000201000003000000aabbcc")},
		{"LoginServerFail", decodes(DecodeLoginServerFail), oracleSeed(tb, "0107")},
		{"AuthResponse", func(payload []byte) error {
			_, _, err := DecodeAuthResponse(payload)
			return err
		}, oracleSeed(tb, "020747006900720061006e000000")},
		{"PlayerAuthResponse", func(payload []byte) error {
			_, _, err := DecodePlayerAuthResponse(payload)
			return err
		}, oracleSeed(tb, "0361006c00690063006500000001")},
		{"KickPlayer", decodes(DecodeKickPlayer), oracleSeed(tb, "0461006c006900630065000000")},
	}
}

func newFuzzLinkKey(tb testing.TB) *rsa.PrivateKey {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		tb.Fatalf("GenerateKey: %v", err)
	}
	return key
}

// TestLinkDecoderFuzzSeedsDecode keeps the fuzz seeds honest: each one must
// be a packet its own decoder accepts.
func TestLinkDecoderFuzzSeedsDecode(t *testing.T) {
	for _, d := range linkDecoders(t, newFuzzLinkKey(t)) {
		if err := d.decode(d.seed); err != nil {
			t.Errorf("%s seed % x: %v", d.name, d.seed, err)
		}
	}
}

// FuzzLinkPackets feeds every GS-LS link packet decoder the same arbitrary
// payload. Each must return rather than panic, and none may allocate more
// than the payload's length justifies.
func FuzzLinkPackets(f *testing.F) {
	decoders := linkDecoders(f, newFuzzLinkKey(f))
	for _, d := range decoders {
		f.Add(d.seed)
		f.Add(d.seed[:len(d.seed)-1])
	}
	f.Add([]byte{})
	// A PlayerInGame count of 65535 with no accounts behind it: the count
	// must not size or drive the decode past the bytes present.
	f.Add([]byte{OpcodePlayerInGame, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, payload []byte) {
		for _, d := range decoders {
			decodefuzz.Bounded(t, d.name, payload, func() { _ = d.decode(payload) })
		}
	})
}
