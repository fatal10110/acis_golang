package manager

import (
	"net"
	"slices"
	"testing"
)

// All lists servers in ascending id order however they were registered, so
// every server list carries the same order. Ascending order is the exact
// reference order for ids 1-15 (hash-bucket order of a 16-bucket table);
// ids of 16 and above are outside that guarantee.
func TestServerRegistryAllAscendingByID(t *testing.T) {
	r := NewServerRegistry()
	for _, id := range []int{9, 3, 15, 7, 1} {
		r.Register(id, []byte{byte(id)})
		r.MarkOnline(id, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)
	}
	for range 20 {
		var ids []int
		for _, e := range r.All() {
			ids = append(ids, e.ID)
		}
		if want := []int{1, 3, 7, 9, 15}; !slices.Equal(ids, want) {
			t.Fatalf("All() ids = %v, want %v", ids, want)
		}
	}
}
