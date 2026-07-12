package item

import "testing"

func TestSpoilPoolLifecycle(t *testing.T) {
	var pool SpoilPool

	if pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = true before Mark, want false")
	}
	if pool.Sweepable() {
		t.Fatal("Sweepable() = true before any Add, want false")
	}

	pool.Mark(42)
	if !pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = false after Mark, want true")
	}
	if !pool.IsSpoiler(42) {
		t.Fatal("IsSpoiler(42) = false, want true")
	}
	if pool.IsSpoiler(7) {
		t.Fatal("IsSpoiler(7) = true, want false")
	}

	pool.Add(100, 3)
	pool.Add(100, 2)
	pool.Add(200, 1)

	if !pool.Sweepable() {
		t.Fatal("Sweepable() = false after Add, want true")
	}

	got := pool.Sweep()
	want := map[int32]int32{100: 5, 200: 1}
	if len(got) != len(want) || got[100] != want[100] || got[200] != want[200] {
		t.Fatalf("Sweep() = %v, want %v", got, want)
	}

	if pool.Sweepable() {
		t.Fatal("Sweepable() = true after Sweep drained the pool, want false")
	}
	if second := pool.Sweep(); second != nil {
		t.Fatalf("second Sweep() = %v, want nil", second)
	}
	if !pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = false after Sweep, want true (marker survives sweeping)")
	}

	pool.Reset()
	if pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = true after Reset, want false")
	}
	if pool.Sweepable() {
		t.Fatal("Sweepable() = true after Reset, want false")
	}
}
