package manager

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	mathrand "math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
)

// keyPoolSize is how many RSA key pairs each pool caches.
const keyPoolSize = 10

// gsKeyBits is the RSA modulus size used for the GS-LS link handshake. The
// pair is regenerated fresh every boot with no cross-version wire contract
// on its size (InitLS carries the modulus length explicitly), and Go's
// crypto/rsa refuses to generate keys below 1024 bits — so this uses the
// minimum Go allows.
const gsKeyBits = 1024

// KeyPool is a fixed set of key pairs generated at boot, one of which is
// handed to each incoming connection for its handshake.
type KeyPool[K any] struct {
	keys []K
}

// LoginKeyPool serves the client-facing login handshake (Init/
// RequestAuthLogin).
type LoginKeyPool = KeyPool[*crypt.LoginKeyPair]

// RSAKeyPool serves the game-server link handshake (InitLS/BlowFishKey).
type RSAKeyPool = KeyPool[*rsa.PrivateKey]

// NewLoginKeyPool generates a fresh pool of login RSA key pairs.
func NewLoginKeyPool() (*LoginKeyPool, error) {
	return newKeyPool("login", crypt.NewLoginKeyPair)
}

// NewRSAKeyPool generates a fresh pool of game-server link RSA key pairs.
func NewRSAKeyPool() (*RSAKeyPool, error) {
	return newKeyPool("gs-link", func() (*rsa.PrivateKey, error) {
		return rsa.GenerateKey(rand.Reader, gsKeyBits)
	})
}

func newKeyPool[K any](kind string, generate func() (K, error)) (*KeyPool[K], error) {
	keys := make([]K, keyPoolSize)
	for i := range keys {
		key, err := generate()
		if err != nil {
			return nil, fmt.Errorf("generate %s RSA key %d: %w", kind, i, err)
		}
		keys[i] = key
	}
	return &KeyPool[K]{keys: keys}, nil
}

// Random returns an arbitrary key pair from the pool.
func (p *KeyPool[K]) Random() K {
	return p.keys[mathrand.IntN(len(p.keys))]
}
