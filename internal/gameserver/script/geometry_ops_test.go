package script

import (
	"slices"
	"testing"
)

// TestAngleFrom pins MathUtil.calculateAngleFrom: atan2 of the ground-plane
// offset in degrees, a negative angle wrapped into [0, 360).
func TestAngleFrom(t *testing.T) {
	from := Loc{X: 100, Y: 100, Z: 50}
	for _, tc := range []struct {
		to   Loc
		want float64
	}{
		{Loc{X: 200, Y: 100}, 0},
		{Loc{X: 100, Y: 200}, 90},
		{Loc{X: 0, Y: 100}, 180},
		{Loc{X: 100, Y: 0}, 270},
		{Loc{X: 200, Y: 0}, 315},
		{Loc{X: 100, Y: 100, Z: 900}, 0},
		{Loc{X: -200, Y: -191}, 224.12754287709797},
	} {
		if got := AngleFrom(from, tc.to); got != tc.want {
			t.Errorf("AngleFrom(%v, %v) = %v, want %v", from, tc.to, got, tc.want)
		}
	}
}

// TestClosestAndFarthest pins Location.getClosestPosition (the nearest in
// 3D, the first on a tie, none from an empty list) and
// getClosestPositionList (sorted by descending 3D distance, ties in list
// order, cut to count).
func TestClosestAndFarthest(t *testing.T) {
	from := Loc{}
	a := Loc{X: 300}
	b := Loc{Y: 100}
	c := Loc{Z: -100}
	d := Loc{X: 200, Heading: 7}
	positions := []Loc{a, b, c, d}

	if got, ok := Closest(from, positions); !ok || got != b {
		t.Fatalf("Closest = %v, %v; want %v (b before c on the tie)", got, ok, b)
	}
	if _, ok := Closest(from, nil); ok {
		t.Fatal("Closest of no position found one")
	}
	if got, want := Farthest(from, positions, 3), []Loc{a, d, b}; !slices.Equal(got, want) {
		t.Fatalf("Farthest(3) = %v, want %v", got, want)
	}
	if got, want := Farthest(from, positions, 9), []Loc{a, d, b, c}; !slices.Equal(got, want) {
		t.Fatalf("Farthest(9) = %v, want %v", got, want)
	}
	if got := Farthest(from, positions, 0); len(got) != 0 {
		t.Fatalf("Farthest(0) = %v, want none", got)
	}
	if positions[0] != a || positions[3] != d {
		t.Fatalf("Farthest reordered its argument: %v", positions)
	}
}

// draws returns a random source handing out the values in order, and a
// record of the bounds it was asked for.
func draws(values ...int) (rand func(int) int, bounds *[]int) {
	bounds = &[]int{}
	return func(n int) int {
		*bounds = append(*bounds, n)
		v := values[0]
		values = values[1:]
		return v
	}, bounds
}

// TestRandomOffset pins Point2D.addRandomOffset: x then y each move by
// Rnd.get(-offset, offset), a draw over 2*offset+1 values.
func TestRandomOffset(t *testing.T) {
	rand, bounds := draws(0, 100)
	s := &Script{env: &Env{Rand: rand}}
	got := s.RandomOffset(Loc{X: 1000, Y: 2000, Z: 30, Heading: 5}, 50)
	if want := (Loc{X: 950, Y: 2050, Z: 30, Heading: 5}); got != want {
		t.Fatalf("RandomOffset = %v, want %v", got, want)
	}
	if want := []int{101, 101}; !slices.Equal(*bounds, want) {
		t.Fatalf("draw bounds %v, want %v", *bounds, want)
	}
}

// TestScramble pins StringUtil.scrambleString, which is Collections.shuffle
// over the characters: from the last position down to the second, swap
// with a draw below the position's count.
func TestScramble(t *testing.T) {
	// "123456789": i=9 swaps [8] with [0], i=8 [7] with [7], i=7 [6] with
	// [2], i=6 [5] with [5], i=5 [4] with [1], i=4 [3] with [0], i=3 [2]
	// with [2], i=2 [1] with [0].
	rand, bounds := draws(0, 7, 2, 5, 1, 0, 2, 0)
	s := &Script{env: &Env{Rand: rand}}
	if got, want := s.Scramble("123456789"), "547926381"; got != want {
		t.Fatalf("Scramble = %q, want %q", got, want)
	}
	if want := []int{9, 8, 7, 6, 5, 4, 3, 2}; !slices.Equal(*bounds, want) {
		t.Fatalf("draw bounds %v, want %v", *bounds, want)
	}
	if got := s.Scramble(""); got != "" {
		t.Fatalf("Scramble(\"\") = %q", got)
	}
}
