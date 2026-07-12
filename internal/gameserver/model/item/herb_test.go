package item

import "testing"

func TestSplitHerbDrop(t *testing.T) {
	t.Run("zero amount yields nothing", func(t *testing.T) {
		if got := SplitHerbDrop(100, 0, false); got != nil {
			t.Fatalf("SplitHerbDrop() = %v, want nil", got)
		}
	})

	t.Run("auto loot always collapses to one stack", func(t *testing.T) {
		got := SplitHerbDrop(100, 5, true)
		want := []HerbPickup{{ItemID: 100, Amount: 1, AutoLoot: true}}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("SplitHerbDrop() = %v, want %v", got, want)
		}
	})

	t.Run("manual pickup yields one pickup per unit", func(t *testing.T) {
		got := SplitHerbDrop(100, 3, false)
		if len(got) != 3 {
			t.Fatalf("SplitHerbDrop() len = %d, want 3", len(got))
		}
		for _, p := range got {
			if p != (HerbPickup{ItemID: 100, Amount: 1}) {
				t.Fatalf("SplitHerbDrop() entry = %v, want {100 1 false}", p)
			}
		}
	})
}
