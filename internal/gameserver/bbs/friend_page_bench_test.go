package bbs

import "testing"

type benchContacts struct{}

func (benchContacts) Name(int32) (string, bool) { return "Name", true }
func (benchContacts) Online(int32) bool         { return false }

// BenchmarkRenderBlockListAtCap renders a 5,000-id block list with every id
// picked, the most a selection can hold (#5102): the cost follows the list
// plus its picks.
func BenchmarkRenderBlockListAtCap(b *testing.B) {
	ids := make([]int32, 5000)
	var picks Selection
	for i := range ids {
		ids[i] = 268436456 + int32(i)
		picks.Add(ids[i], len(ids))
	}
	if got := len(picks.IDs()); got != len(ids) {
		b.Fatalf("selection holds %d, want %d", got, len(ids))
	}
	picks.Add(1, len(ids))
	picked := picks.IDs()
	if len(picked) != len(ids) {
		b.Fatalf("selection past its cap holds %d, want %d", len(picked), len(ids))
	}
	for b.Loop() {
		RenderBlockList(BlockListPage, ids, picked, benchContacts{}, false)
	}
}
