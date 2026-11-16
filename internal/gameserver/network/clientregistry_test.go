package network

import "testing"

// TestClientRegistryReleaseReportsOwnership pins that only the connection
// holding an account releases it: a connection a later Take replaced is
// told it no longer owns the account and leaves the newer claim in place.
func TestClientRegistryReleaseReportsOwnership(t *testing.T) {
	r := NewClientRegistry()
	first, second := &Client{}, &Client{}

	if _, replaced := r.Take("acc", first); replaced {
		t.Fatal("first Take replaced a claim")
	}
	if evicted, replaced := r.Take("acc", second); !replaced || evicted != first {
		t.Fatalf("second Take = %p, %v; want first, true", evicted, replaced)
	}
	if r.Release("acc", first) {
		t.Fatal("replaced connection released the account")
	}
	if owner, ok := r.Lookup("acc"); !ok || owner != second {
		t.Fatalf("owner after stale Release = %p, %v; want second", owner, ok)
	}
	if !r.Release("acc", second) {
		t.Fatal("owner did not release the account")
	}
	if _, ok := r.Lookup("acc"); ok {
		t.Fatal("account still claimed after its owner released it")
	}
	if r.Release("acc", second) {
		t.Fatal("second Release of a released account reported ownership")
	}
}
