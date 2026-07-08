package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/link"
)

func TestSessionStorePutGetDelete(t *testing.T) {
	s := NewSessionStore()

	if _, ok := s.Get("acc1"); ok {
		t.Fatal("Get() on empty store = true, want false")
	}

	key := link.SessionKey{PlayKey1: 1, PlayKey2: 2, LoginKey1: 3, LoginKey2: 4}
	s.Put("acc1", key)

	got, ok := s.Get("acc1")
	if !ok || got != key {
		t.Fatalf("Get() = %+v, %v, want %+v, true", got, ok, key)
	}

	s.Delete("acc1")
	if _, ok := s.Get("acc1"); ok {
		t.Fatal("Get() after Delete() = true, want false")
	}
}
