package sim

import (
	"slices"
	"testing"
)

func drainFIFO(f *fifo[int]) []int {
	var got []int
	for f.len() > 0 {
		got = append(got, f.pop())
	}
	return got
}

func TestFIFOKeepsOrderAcrossCompaction(t *testing.T) {
	var f fifo[int]
	var want []int
	next := 0
	// Pop fewer than pushed each round so the queue never empties and push
	// has to compact the consumed prefix.
	for range 50 {
		for range 3 {
			f.push(next)
			next++
		}
		for range 2 {
			want = append(want, f.pop())
		}
	}
	want = append(want, drainFIFO(&f)...)
	for i, v := range want {
		if v != i {
			t.Fatalf("item %d = %d, want %d (order %v)", i, v, i, want)
		}
	}
}

func TestFIFOTakeLeavesTheTailInOrder(t *testing.T) {
	var f fifo[int]
	for i := range 10 {
		f.push(i)
	}
	var dst [4]int
	if n := f.take(dst[:]); n != 4 || dst != [4]int{0, 1, 2, 3} {
		t.Fatalf("take = %d %v, want 4 [0 1 2 3]", n, dst)
	}
	f.push(10)
	if got := drainFIFO(&f); !slices.Equal(got, []int{4, 5, 6, 7, 8, 9, 10}) {
		t.Fatalf("tail = %v", got)
	}
	for _, v := range f.buf[:cap(f.buf)] {
		if v != 0 {
			t.Fatalf("buffer keeps a consumed item: %v", f.buf[:cap(f.buf)])
		}
	}
}

func TestFIFOUnshiftPutsItemsBackInFront(t *testing.T) {
	for _, tc := range []struct {
		name  string
		taken int // items taken before unshift; fewer than put back forces a grow
	}{{"into consumed prefix", 3}, {"grows", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			var f fifo[int]
			for i := range 6 {
				f.push(i)
			}
			for range tc.taken {
				f.pop()
			}
			f.unshift([]int{-2, -1})
			want := append([]int{-2, -1}, []int{0, 1, 2, 3, 4, 5}[tc.taken:]...)
			if got := drainFIFO(&f); !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestFIFOReleaseDropsOnlyALargeIdleBuffer(t *testing.T) {
	var f fifo[int]
	for i := range 8 {
		f.push(i)
	}
	f.release(4)
	if f.len() != 8 {
		t.Fatal("release dropped a non-empty buffer")
	}
	drainFIFO(&f)
	f.release(cap(f.buf))
	if f.buf == nil {
		t.Fatal("release dropped a buffer within keep")
	}
	f.release(4)
	if f.buf != nil {
		t.Fatalf("release kept a %d-item idle buffer above keep 4", cap(f.buf))
	}
}

func TestFIFOWarmCyclesAllocateNothing(t *testing.T) {
	var f fifo[int]
	var dst [4]int
	// Never empty: one in, one out over a standing backlog.
	for i := range 5 {
		f.push(i)
	}
	if n := testing.AllocsPerRun(1000, func() { f.push(1); f.pop() }); n != 0 {
		t.Errorf("standing-backlog push/pop allocates %v times, want 0", n)
	}
	// Batches that leave a tail behind.
	if n := testing.AllocsPerRun(1000, func() {
		for range 6 {
			f.push(1)
		}
		f.take(dst[:])
		f.take(dst[:])
	}); n != 0 {
		t.Errorf("batched take allocates %v times, want 0", n)
	}
}
