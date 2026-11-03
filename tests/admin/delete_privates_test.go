package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminDeleteLeavesPrivates pins //delete of a party leader: only the
// selected master leaves the world with its spawn; the privates its
// behavior created stay in the world, alive.
func TestAdminDeleteLeavesPrivates(t *testing.T) {
	t.Parallel()
	const pupID = 20121
	tmpl := func(id int, name string) *npc.Template {
		return &npc.Template{ID: id, TemplateID: id, Type: "Monster", Name: name, Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true}
	}
	wolf := tmpl(wolfID, "Wolf")
	wolf.AIParams = npc.AIParams{"Party_Type": "2"}
	wolf.Privates = []npc.PrivateEntry{{NpcID: pupID, Weight: 1}, {NpcID: pupID, Weight: 1}}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, tmpl(pupID, "Wolf Pup")})),
		gameservertest.WithNpcSpawns(nil), withPartyLeader(wolfID))
	enterWorld(t, srv.Client)
	gm := srv.Client

	exchange(t, gm, encodeBuildCmd("spawn 20120"))
	srv.Settle(t)
	leaders, pups := npcsOf(srv, wolfID), npcsOf(srv, pupID)
	if len(leaders) != 1 || len(pups) != 2 {
		t.Fatalf("spawned %d leaders and %d privates, want 1 and 2", len(leaders), len(pups))
	}
	leader := leaders[0].(*npc.Hostile)
	for _, p := range pups {
		if !slices.Contains(leader.Minions(), npc.PartyMember(p.(*npc.Hostile))) {
			t.Fatalf("private %d is not the leader's", p.ObjectID())
		}
	}

	drain(t, gm)
	exchange(t, gm, encodeAction(leader.ObjectID()))
	frames := exchange(t, gm, encodeBuildCmd("delete"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You deleted Wolf."}) {
		t.Fatalf("//delete messages = %q, want the delete notice", got)
	}
	srv.Settle(t)
	if _, ok := srv.State.Object(leader.ObjectID()); ok {
		t.Fatal("deleted leader still in the world")
	}
	for _, p := range pups {
		pup := p.(*npc.Hostile)
		if _, ok := srv.State.Object(pup.ObjectID()); !ok || pup.Decayed() || pup.Dead() {
			t.Fatalf("private %d left with its master: in world %v, decayed %v, dead %v", pup.ObjectID(), ok, pup.Decayed(), pup.Dead())
		}
	}
}
