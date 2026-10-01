package admin

import (
	"sync"
	"testing"
)

// TestGMList pins the roster's membership rules: registration order kept,
// a re-added member keeping its place, hidden members visible only when
// asked for, Toggle flipping members only, and Remove dropping one.
func TestGMList(t *testing.T) {
	var l GMList[string]
	if l.Online(true) {
		t.Fatal("empty list reports a GM online")
	}
	l.Add("a", false)
	l.Add("b", true)
	l.Add("c", false)
	l.Add("a", true)
	if got := l.Entries(true); len(got) != 3 || got[0] != (GMEntry[string]{"a", true}) || got[1].Player != "b" || got[2].Player != "c" {
		t.Fatalf("Entries(true) = %v", got)
	}
	if got := l.Entries(false); len(got) != 1 || got[0].Player != "c" {
		t.Fatalf("Entries(false) = %v", got)
	}
	if hidden, ok := l.Toggle("c"); !ok || !hidden {
		t.Fatalf("Toggle(c) = %v %v, want hidden", hidden, ok)
	}
	if l.Online(false) || !l.Online(true) {
		t.Fatal("all-hidden list: Online(false) must be false, Online(true) true")
	}
	if _, ok := l.Toggle("z"); ok {
		t.Fatal("Toggle of a non-member reported ok")
	}
	l.Remove("b")
	if l.Contains("b") || !l.Contains("a") || len(l.Entries(true)) != 2 {
		t.Fatalf("after Remove(b): %v", l.Entries(true))
	}
}

// TestGMListConcurrent exercises the roster from several goroutines under
// the race detector.
func TestGMListConcurrent(t *testing.T) {
	var l GMList[int]
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 100 {
				l.Add(i, j%2 == 0)
				l.Toggle(i)
				l.Entries(true)
				l.Online(false)
				l.Remove(i)
			}
		})
	}
	wg.Wait()
	if l.Online(true) {
		t.Fatalf("roster after every member left = %v", l.Entries(true))
	}
}
