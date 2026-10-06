package script

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// A handle on a civilian NPC reports the creature it was spawned for, a
// player's handle, and reports the NPC gone once it decays.
func TestNPCHandleSummonerAndDecayed(t *testing.T) {
	summoner := &combatant{objectID: 7}
	inst, err := npc.NewInstance(100, &npc.Template{ID: 30001, TemplateID: 30001, Type: "Merchant", Name: "Grocer", Level: 1, HPMax: 10})
	if err != nil {
		t.Fatal(err)
	}
	inst.Summoner = summoner
	f, err := npc.NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}

	h := NPCOf(f)
	if h == nil || h.brain != nil {
		t.Fatalf("folk handle = %+v, want one with no brain", h)
	}
	p, ok := h.Summoner().(*Player)
	if !ok || combatantOf(p) != summoner {
		t.Fatalf("summoner = %#v, want the player handle on the summoner", h.Summoner())
	}
	if h.Decayed() {
		t.Fatal("live NPC reports gone")
	}
	f.Decay(nil, nil)
	if !h.Decayed() {
		t.Fatal("decayed NPC reports alive")
	}

	inst.Summoner = nil
	if s := NPCOf(f).Summoner(); s != nil {
		t.Fatalf("summoner of an NPC spawned for no one = %#v, want nil", s)
	}
	var none *NPC
	if !none.Decayed() || none.Summoner() != nil || NPCOf(nil) != nil || PlayerOf(nil) != nil {
		t.Fatal("a handle on nothing is not empty")
	}
}
