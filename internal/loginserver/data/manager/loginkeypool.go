package manager

import (
	"fmt"
	mathrand "math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
)

// loginKeyPoolSize is how many RSA key pairs are cached for the client-facing
// login handshake.
const loginKeyPoolSize = 10

// LoginKeyPool is a fixed set of RSA key pairs generated at boot, one of
// which is handed to each incoming login client connection for its Init/
// RequestAuthLogin handshake.
type LoginKeyPool struct {
	keys []*crypt.LoginKeyPair
}

// NewLoginKeyPool generates a fresh pool of login RSA key pairs.
func NewLoginKeyPool() (*LoginKeyPool, error) {
	keys := make([]*crypt.LoginKeyPair, loginKeyPoolSize)
	for i := range keys {
		pair, err := crypt.NewLoginKeyPair()
		if err != nil {
			return nil, fmt.Errorf("generate login RSA key %d: %w", i, err)
		}
		keys[i] = pair
	}
	return &LoginKeyPool{keys: keys}, nil
}

// Random returns an arbitrary key pair from the pool.
func (p *LoginKeyPool) Random() *crypt.LoginKeyPair {
	return p.keys[mathrand.IntN(len(p.keys))]
}
