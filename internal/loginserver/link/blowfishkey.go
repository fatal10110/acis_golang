package link

import (
	"crypto/rsa"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/loginserver/crypt"
)

// OpcodeBlowFishKey is the wire opcode for BlowFishKey, the game server's
// proposed dynamic Blowfish key for the rest of the link.
const OpcodeBlowFishKey = 0x00

// DecodeBlowFishKey parses a raw BlowFishKey payload (opcode byte included)
// and RSA-decrypts the embedded key with the link's private key, returning
// a key ready for LinkCrypt.SetKey.
func DecodeBlowFishKey(payload []byte, priv *rsa.PrivateKey) ([]byte, error) {
	r := newReader(payload)
	size := int(r.readInt32())
	ciphertext := r.readBytes(size)
	if r.err != nil {
		return nil, fmt.Errorf("link: BlowFishKey: %w", r.err)
	}
	return crypt.DecryptDynamicKey(priv, ciphertext), nil
}
