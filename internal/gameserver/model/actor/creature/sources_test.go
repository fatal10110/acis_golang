package creature

import (
	"sync"
	"testing"
)

func TestSourceSetHoldsUntilTheLastSourceEnds(t *testing.T) {
	var s SourceSet
	type effectSource struct{ id int }
	a, b := &effectSource{1}, &effectSource{2}

	if !s.Set(a, true) || !s.Held() {
		t.Fatal("first source did not hold the status")
	}
	if s.Set(b, true) || s.Set(a, true) {
		t.Fatal("a second source, or the first set again, reported a change")
	}
	if s.Set(a, false) || !s.Held() {
		t.Fatal("ending one of two sources ended the status")
	}
	if s.Set(a, false) {
		t.Fatal("ending a source twice reported a change")
	}
	if !s.Set(b, false) || s.Held() {
		t.Fatal("ending the last source left the status held")
	}
	if s.Set(b, false) {
		t.Fatal("clearing an empty set reported a change")
	}
}

func TestSourceSetConcurrentSourcesEndClear(t *testing.T) {
	var s SourceSet
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			for range 100 {
				s.Set(i, true)
				if !s.Held() {
					t.Error("status not held while this goroutine's source is set")
				}
				s.Set(i, false)
			}
		})
	}
	wg.Wait()
	if s.Held() {
		t.Fatal("status held after every source ended")
	}
}
