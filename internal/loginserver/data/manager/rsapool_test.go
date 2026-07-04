package manager

import "testing"

func TestNewRSAKeyPool(t *testing.T) {
	pool, err := NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}
	if len(pool.keys) != gsKeyPoolSize {
		t.Fatalf("len(pool.keys) = %d, want %d", len(pool.keys), gsKeyPoolSize)
	}
	for i, k := range pool.keys {
		if got := k.N.BitLen(); got != gsKeyBits {
			t.Fatalf("pool.keys[%d] bit length = %d, want %d", i, got, gsKeyBits)
		}
	}

	for i := 0; i < 50; i++ {
		k := pool.Random()
		if k == nil {
			t.Fatal("Random() returned nil")
		}
	}
}
