package world

import (
	"sync"
	"testing"
)

func TestRegistry_AddIsNoopIfPresent(t *testing.T) {
	r := newRegistry()

	first := stubObject{id: 1, gen: 1}
	second := stubObject{id: 1, gen: 2}

	r.add(first.id, first)
	r.add(second.id, second)

	got, ok := r.get(1)
	if !ok {
		t.Fatal("get(1) missing after add")
	}
	if got != first {
		t.Errorf("get(1) = %+v, want first registration %+v (add must not overwrite)", got, first)
	}
}

func TestRegistry_RemoveAndRemoveAll(t *testing.T) {
	r := newRegistry()
	r.add(1, stubObject{id: 1})
	r.add(2, stubObject{id: 2})
	r.add(3, stubObject{id: 3})

	r.remove(1)
	if _, ok := r.get(1); ok {
		t.Error("get(1) present after remove")
	}

	r.removeAll([]int32{2, 3})
	if got := r.all(); len(got) != 0 {
		t.Errorf("all() = %v, want empty after removeAll", got)
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	r := newRegistry()

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			r.add(id, stubObject{id: id})
			r.get(id)
			r.all()
			r.remove(id)
		}(i)
	}
	wg.Wait()

	if got := r.all(); len(got) != 0 {
		t.Errorf("all() after concurrent add/remove = %v, want empty", got)
	}
}
