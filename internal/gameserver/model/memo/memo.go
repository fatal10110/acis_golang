// Package memo holds a player's memos: named string values that scripts
// keep across sessions, saved as character_memo rows.
package memo

import "sync"

// Writer saves a player's memo changes. Memos calls it in the order the
// changes are made, while holding its lock, so an implementation must not
// block and must keep that order.
type Writer interface {
	// Set saves value under key, replacing any value saved there.
	Set(key, value string)
	// Unset deletes the value saved under key.
	Unset(key string)
}

// Memos is one player's memos. The zero value holds none and saves
// nothing.
type Memos struct {
	mu   sync.Mutex
	vals map[string]string
	w    Writer
}

// Restore replaces the memos with vals, as they were saved, and saves every
// later change through w.
func (m *Memos) Restore(vals map[string]string, w Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vals = vals
	m.w = w
}

// Get returns the memo key.
func (m *Memos) Get(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[key]
	return v, ok
}

// Set sets the memo key to value and saves it, even when it already held
// that value.
func (m *Memos) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vals == nil {
		m.vals = make(map[string]string)
	}
	m.vals[key] = value
	if m.w != nil {
		m.w.Set(key, value)
	}
}

// Unset removes the memo key and deletes its saved value, even when it
// held none.
func (m *Memos) Unset(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vals, key)
	if m.w != nil {
		m.w.Unset(key)
	}
}
