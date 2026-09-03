package pets

import "testing"

func TestAutosavePersistsLivePetState(t *testing.T) {
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	actor.SetHP(37)

	h.srv.TickAutosave()

	if got := h.savedPetState(t).CurHP; got != 37 {
		t.Fatalf("autosaved pet HP = %v, want 37", got)
	}
}
