package crypt

import (
	"sync"
	"testing"
)

// TestLinkCryptSetKeyWhileEncrypting swaps the key while other goroutines
// encrypt and decrypt on the same LinkCrypt; under -race the swap must not
// race the reads, and every call works under one whole key.
func TestLinkCryptSetKeyWhileEncrypting(t *testing.T) {
	c := NewLinkCrypt()
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		for range 200 {
			c.Encrypt([]byte{1, 2, 3})
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		buf := make([]byte, BlockSize)
		for range 200 {
			_ = c.Decrypt(buf)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range 200 {
			if err := c.SetKey([]byte("0123456789abcdef")); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	close(start)
	wg.Wait()

	// Every call after the swaps uses the new key.
	peer := NewLinkCrypt()
	if err := peer.SetKey([]byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	body := c.Encrypt([]byte{7, 8, 9})
	if err := peer.Decrypt(body); err != nil || body[0] != 7 || body[2] != 9 {
		t.Fatalf("peer decrypt = %v, %v", body, err)
	}
}
