package crypt

import "testing"

// TestNewSessionKeyHasNoZeroByte draws enough keys that a generator allowing
// a zero byte would produce one with near certainty, and checks every byte of
// every key is in 1..255.
func TestNewSessionKeyHasNoZeroByte(t *testing.T) {
	for range 4096 {
		key, err := NewSessionKey()
		if err != nil {
			t.Fatalf("NewSessionKey() error: %v", err)
		}
		if len(key) != sessionKeySize {
			t.Fatalf("len(key) = %d, want %d", len(key), sessionKeySize)
		}
		for i, b := range key {
			if b == 0 {
				t.Fatalf("key %x byte %d is 0, want 1..255", key, i)
			}
		}
	}
}
