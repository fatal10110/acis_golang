package rnd

import (
	"math"
	"testing"
)

// The reference's Get(n) is ThreadLocalRandom.nextInt(n), which throws on a
// non-positive bound; Get(min, max) is nextInt(min, max+1), inclusive of both
// ends, which throws when max < min. Go panics where the reference throws.

func TestGetStaysBelowBound(t *testing.T) {
	t.Parallel()
	seen := map[int]bool{}
	for range 2000 {
		v := Get(5)
		if v < 0 || v >= 5 {
			t.Fatalf("Get(5) = %d, want [0, 5)", v)
		}
		seen[v] = true
	}
	if len(seen) != 5 {
		t.Errorf("Get(5) produced %v, want every value 0..4", seen)
	}
	if got := Get(1); got != 0 {
		t.Errorf("Get(1) = %d, want 0", got)
	}
}

func TestGetRangeIncludesBothEnds(t *testing.T) {
	t.Parallel()
	seen := map[int]bool{}
	for range 2000 {
		v := GetRange(-2, 2)
		if v < -2 || v > 2 {
			t.Fatalf("GetRange(-2, 2) = %d", v)
		}
		seen[v] = true
	}
	if len(seen) != 5 {
		t.Errorf("GetRange(-2, 2) produced %v, want -2 through 2", seen)
	}
	if got := GetRange(7, 7); got != 7 {
		t.Errorf("GetRange(7, 7) = %d, want 7", got)
	}
}

func TestGetRangeAtInt32Limits(t *testing.T) {
	t.Parallel()
	for range 200 {
		if v := GetRange(math.MaxInt32-1, math.MaxInt32); v < math.MaxInt32-1 || v > math.MaxInt32 {
			t.Fatalf("GetRange(MaxInt32-1, MaxInt32) = %d", v)
		}
		if v := GetRange(math.MinInt32, math.MinInt32+1); v < math.MinInt32 || v > math.MinInt32+1 {
			t.Fatalf("GetRange(MinInt32, MinInt32+1) = %d", v)
		}
		if v := GetRange(0, math.MaxInt32); v < 0 || v > math.MaxInt32 {
			t.Fatalf("GetRange(0, MaxInt32) = %d", v)
		}
	}
}

func TestInvalidBoundsPanic(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]func(){
		"Get(0)":         func() { Get(0) },
		"Get(-1)":        func() { Get(-1) },
		"GetRange(5, 4)": func() { GetRange(5, 4) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}
