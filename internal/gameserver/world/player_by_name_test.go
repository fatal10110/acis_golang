package world

import "testing"

type namedPlayerObject struct {
	id   int32
	name string
}

func (o namedPlayerObject) ObjectID() int32       { return o.id }
func (o namedPlayerObject) CharacterName() string { return o.name }

func TestState_PlayerByName(t *testing.T) {
	s := New()
	p := namedPlayerObject{id: 42, name: "Newbie"}
	s.AddPlayer(p)

	t.Run("found, case-insensitive", func(t *testing.T) {
		got, ok := s.PlayerByName("newBIE")
		if !ok {
			t.Fatal("PlayerByName() ok = false, want true")
		}
		if got.ObjectID() != p.id {
			t.Errorf("PlayerByName() = %v, want %v", got.ObjectID(), p.id)
		}
	})

	t.Run("missing", func(t *testing.T) {
		if _, ok := s.PlayerByName("Ghost"); ok {
			t.Error("PlayerByName(\"Ghost\") ok = true, want false")
		}
	})

	t.Run("removed", func(t *testing.T) {
		s.RemovePlayer(p.id)
		if _, ok := s.PlayerByName("Newbie"); ok {
			t.Error("PlayerByName() after RemovePlayer ok = true, want false")
		}
	})
}
