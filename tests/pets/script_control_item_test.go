package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScriptPetControlItemID pins Summon.getControlItemId and
// Pet.getControlItemId (Q421): a pet's control item is the object id of
// the collar that called it out; the owner, no summon, has none.
func TestScriptPetControlItemID(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithScripts([]script.Listing{{Path: "quest.PetProbe"}}, script.Catalog{
			"quest.PetProbe": func() script.Script { return script.Script{} },
		}),
	})
	h.spawnWolf(t)

	var pet, owner int32
	if !h.srv.RunScript(t, h.ownerID, "PetProbe", func(_ *script.Script, p *script.Player) {
		pet, owner = script.PetControlItemID(p.ActiveSummon()), script.PetControlItemID(p)
	}) {
		t.Fatal("probe invocation panicked")
	}
	if pet != h.collarID || owner != 0 {
		t.Fatalf("PetControlItemID: pet %d, owner %d; want the collar %d, 0", pet, owner, h.collarID)
	}
}
