package link

import (
	"crypto/rsa"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
)

// OpcodeBlowFishKey is the wire opcode for BlowFishKey, the game server's
// proposed dynamic Blowfish key for the rest of the link.
const OpcodeBlowFishKey = 0x00

// DecodeBlowFishKey parses a raw BlowFishKey payload (opcode byte included)
// and RSA-decrypts the embedded key with the link's private key, returning
// a key ready for LinkCrypt.SetKey.
func DecodeBlowFishKey(payload []byte, priv *rsa.PrivateKey) ([]byte, error) {
	r := newReader(payload)
	size := int(r.ReadInt32())
	ciphertext := r.ReadBytes(size)
	if r.Err() != nil {
		return nil, fmt.Errorf("link: BlowFishKey: %w", r.Err())
	}
	return crypt.DecryptDynamicKey(priv, ciphertext), nil
}

// EncodeBlowFishKey builds the BlowFishKey packet: key RSA-encrypted with
// the login server's public key (recovered from InitLS), the dynamic
// Blowfish key the game server proposes for the rest of the link.
func EncodeBlowFishKey(pub *rsa.PublicKey, key []byte) []byte {
	ciphertext := crypt.EncryptDynamicKey(pub, key)

	w := newWriter(OpcodeBlowFishKey)
	w.WriteInt32(int32(len(ciphertext)))
	w.WriteBytes(ciphertext)
	return w.Bytes()
}
