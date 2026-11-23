package manager

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/link"
)

// Take hands a session to exactly one of any number of concurrent callers
// presenting its key, and never removes a session with a different key.
func TestSessionStoreTakeIsCompareAndDelete(t *testing.T) {
	s := NewSessionStore()
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	s.Put("player1", key, NoServer)

	if s.Take("player1", link.SessionKey{LoginKey1: 1}) {
		t.Fatal("Take with a different key = true")
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Take("player1", key) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := wins.Load(); n != 1 {
		t.Fatalf("concurrent Take winners = %d, want 1", n)
	}
	if _, ok := s.Get("player1"); ok {
		t.Fatal("session still stored after Take")
	}
}

// Claim admits only the server the session was bound to, leaves the session
// for that server after any refusal, and Bind never touches a session with a
// different key.
func TestSessionStoreClaimRequiresBoundServer(t *testing.T) {
	s := NewSessionStore()
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	s.Put("player1", key, NoServer)

	if s.Claim("player1", key, 1) {
		t.Fatal("Claim before any server was chosen = true")
	}
	if s.Bind("player1", link.SessionKey{LoginKey1: 1}, 2) {
		t.Fatal("Bind with a different key = true")
	}
	if !s.Bind("player1", key, 1) {
		t.Fatal("Bind with the session key = false")
	}
	if s.Claim("player1", key, 2) {
		t.Fatal("Claim by another server = true")
	}
	if s.Claim("player1", link.SessionKey{LoginKey1: 1}, 1) {
		t.Fatal("Claim with a different key = true")
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Claim("player1", key, 1) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := wins.Load(); n != 1 {
		t.Fatalf("concurrent Claim winners = %d, want 1", n)
	}
	if _, ok := s.Get("player1"); ok {
		t.Fatal("session still stored after Claim")
	}
}
