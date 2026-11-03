package world

import "testing"

// indexed reports the id PlayerByName resolves name to, or 0 when none.
func indexed(s *State, name string) int32 {
	p, ok := s.PlayerByName(name)
	if !ok {
		return 0
	}
	return p.ObjectID()
}

func TestState_RenamePlayer(t *testing.T) {
	t.Run("name another online player holds, in another case", func(t *testing.T) {
		s := New()
		a := &namedPlayerObject{id: 1, name: "Alpha"}
		b := &namedPlayerObject{id: 2, name: "Bravo"}
		s.AddPlayer(a)
		s.AddPlayer(b)

		called := false
		if s.RenamePlayer(a.id, "bRAVO", func() { called = true }) {
			t.Fatal("RenamePlayer() = true, want false")
		}
		if called {
			t.Fatal("RenamePlayer() ran rename for a refused name")
		}
		if got := indexed(s, "Alpha"); got != a.id {
			t.Errorf("PlayerByName(Alpha) = %d, want %d", got, a.id)
		}
		if got := indexed(s, "Bravo"); got != b.id {
			t.Errorf("PlayerByName(Bravo) = %d, want %d", got, b.id)
		}
	})

	t.Run("free name", func(t *testing.T) {
		s := New()
		a := &namedPlayerObject{id: 1, name: "Alpha"}
		s.AddPlayer(a)

		if !s.RenamePlayer(a.id, "Charlie", func() { a.name = "Charlie" }) {
			t.Fatal("RenamePlayer() = false, want true")
		}
		if got := indexed(s, "Alpha"); got != 0 {
			t.Errorf("PlayerByName(Alpha) = %d, want none", got)
		}
		if got := indexed(s, "charlie"); got != a.id {
			t.Errorf("PlayerByName(charlie) = %d, want %d", got, a.id)
		}
	})

	t.Run("own name in another case", func(t *testing.T) {
		s := New()
		a := &namedPlayerObject{id: 1, name: "Alpha"}
		s.AddPlayer(a)

		if !s.RenamePlayer(a.id, "ALPHA", func() { a.name = "ALPHA" }) {
			t.Fatal("RenamePlayer() = false, want true")
		}
		if got := indexed(s, "alpha"); got != a.id {
			t.Errorf("PlayerByName(alpha) = %d, want %d", got, a.id)
		}
	})

	t.Run("id not online", func(t *testing.T) {
		s := New()
		a := &namedPlayerObject{id: 1, name: "Alpha"}
		s.AddPlayer(a)

		called := false
		if !s.RenamePlayer(9, "Delta", func() { called = true }) {
			t.Fatal("RenamePlayer() = false, want true")
		}
		if !called {
			t.Fatal("RenamePlayer() did not run rename")
		}
		if got := indexed(s, "Delta"); got != 0 {
			t.Errorf("PlayerByName(Delta) = %d, want none", got)
		}
		if got := indexed(s, "Alpha"); got != a.id {
			t.Errorf("PlayerByName(Alpha) = %d, want %d", got, a.id)
		}
	})
}
