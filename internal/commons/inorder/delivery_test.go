package inorder

import (
	"slices"
	"sync"
	"testing"
)

// batch is a one-notice batch whose notice names it.
func batch(id int) []int { return []int{id} }

// TestDeliveryKeepsDecisionOrder pins #5144 and #5489: a change decided while another
// goroutine still applies an earlier change's notices is applied after
// them, by that goroutine, and its own caller returns without waiting.
func TestDeliveryKeepsDecisionOrder(t *testing.T) {
	var d Delivery[int]
	var mu sync.Mutex
	var applied []int
	entered := make(chan struct{})
	release := make(chan struct{})
	apply := func(n []int) {
		id := n[0]
		if id == 1 {
			close(entered)
			<-release
		}
		mu.Lock()
		applied = append(applied, id)
		mu.Unlock()
	}

	first := make(chan struct{})
	go func() {
		defer close(first)
		d.Change(func() []int { return batch(1) }, apply)
	}()
	<-entered
	// Decided second, while the first change is still being applied: it
	// must not overtake it.
	d.Change(func() []int { return batch(2) }, apply)
	mu.Lock()
	early := slices.Clone(applied)
	mu.Unlock()
	if len(early) != 0 {
		t.Fatalf("applied %v before the first change finished, want nothing", early)
	}
	close(release)
	<-first
	if want := []int{1, 2}; !slices.Equal(applied, want) {
		t.Fatalf("applied %v, want %v", applied, want)
	}
}

// TestDeliveryNestedChangeFollows pins that a change apply makes is
// applied after the notices that led to it, and that an empty decision
// applies nothing.
func TestDeliveryNestedChangeFollows(t *testing.T) {
	var d Delivery[int]
	var applied []int
	var apply func([]int)
	apply = func(n []int) {
		id := n[0]
		applied = append(applied, id)
		if id == 1 {
			d.Change(func() []int { return nil }, apply)
			d.Change(func() []int { return batch(2) }, apply)
			if len(applied) != 1 {
				t.Fatalf("nested change applied inside its trigger: %v", applied)
			}
		}
	}
	d.Change(func() []int { return batch(1) }, apply)
	if want := []int{1, 2}; !slices.Equal(applied, want) {
		t.Fatalf("applied %v, want %v", applied, want)
	}
}

// TestDeliveryRecoversFromPanickingApply pins that a panicking apply does
// not stop later changes from being delivered.
func TestDeliveryRecoversFromPanickingApply(t *testing.T) {
	var d Delivery[int]
	func() {
		defer func() { _ = recover() }()
		d.Change(func() []int { return batch(1) }, func([]int) { panic("apply") })
	}()
	var applied []int
	d.Change(func() []int { return batch(2) }, func(n []int) { applied = append(applied, n[0]) })
	if want := []int{2}; !slices.Equal(applied, want) {
		t.Fatalf("applied %v, want %v", applied, want)
	}
}
