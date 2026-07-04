package manager

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	mathrand "math/rand/v2"
)

// gsKeyPoolSize is how many RSA key pairs are cached for the GS-LS link
// handshake, matching the reference server's fixed pool size.
const gsKeyPoolSize = 10

// gsKeyBits is the RSA modulus size used for the GS-LS link handshake. The
// reference server uses 512 bits, but the pair is regenerated fresh every
// boot with no cross-version wire contract on its size (InitLS carries the
// modulus length explicitly), and Go's crypto/rsa refuses to generate keys
// below 1024 bits — so this uses the minimum Go allows instead.
const gsKeyBits = 1024

// RSAKeyPool is a fixed set of RSA key pairs generated at boot, one of
// which is handed to each incoming game-server link connection for its
// InitLS/BlowFishKey handshake.
type RSAKeyPool struct {
	keys []*rsa.PrivateKey
}

// NewRSAKeyPool generates a fresh pool of RSA key pairs.
func NewRSAKeyPool() (*RSAKeyPool, error) {
	keys := make([]*rsa.PrivateKey, gsKeyPoolSize)
	for i := range keys {
		priv, err := rsa.GenerateKey(rand.Reader, gsKeyBits)
		if err != nil {
			return nil, fmt.Errorf("generate gs-link RSA key %d: %w", i, err)
		}
		keys[i] = priv
	}
	return &RSAKeyPool{keys: keys}, nil
}

// Random returns an arbitrary key pair from the pool.
func (p *RSAKeyPool) Random() *rsa.PrivateKey {
	return p.keys[mathrand.IntN(len(p.keys))]
}
