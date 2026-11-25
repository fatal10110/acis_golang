package network

import (
	"strings"
	"sync"
)

// petNameClaims holds the pet names renames have claimed and not yet
// written to the pets table. A rename claims its name before it reads the
// table and keeps it until its row write is done, or drops it when it does
// not apply the name, so of two renames to one name that would both read it
// free, the second is refused (#5060). Names are compared without ASCII case,
// as strictly as a case-insensitive pets.name column: a valid pet name is
// ASCII letters and digits only.
type petNameClaims struct {
	mu    sync.Mutex
	names map[string]struct{}
}

// claim reserves name and reports whether it was free.
func (c *petNameClaims) claim(name string) bool {
	key := strings.ToLower(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.names[key]; ok {
		return false
	}
	if c.names == nil {
		c.names = make(map[string]struct{})
	}
	c.names[key] = struct{}{}
	return true
}

// release frees a name claim took.
func (c *petNameClaims) release(name string) {
	c.mu.Lock()
	delete(c.names, strings.ToLower(name))
	c.mu.Unlock()
}
