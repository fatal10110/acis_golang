package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// placedLifeTowers has the castles' control towers placed at boot, as the
// game server does once its spawns are up, with the life tower's working
// form as the datapack defines it.
func placedLifeTowers() []gameservertest.Option {
	return []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{lifeTowerWorking()})),
		gameservertest.WithNpcSpawns(nil),
	}
}

// lifeTowerWorking is the life control tower's working form as the
// datapack defines it.
func lifeTowerWorking() *npc.Template {
	return &npc.Template{
		ID: npc.LifeTowerWorkingNpcID, TemplateID: npc.LifeTowerWorkingNpcID, Type: "LifeTower", Name: "Life Control Tower",
		Level: 20, HPMax: 342, MPMax: 213, WalkSpeed: 49.5, CollisionRadius: 13, CollisionHeight: 54,
		CanBeAttacked: true, CanMove: true,
	}
}

// gludioLifeTower is the castle's one placed life control tower, working
// under the siege.
func (w *folkWorld) gludioLifeTower(t *testing.T) (*siege.Siege, *npc.Folk) {
	t.Helper()
	gludio, ok := w.srv.Sieges.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	placed := gludio.Castle().Towers()
	if len(placed) != 1 {
		t.Fatalf("towers placed = %d, want 1", len(placed))
	}
	tower, ok := placed[0].(*npc.Folk)
	if !ok || !tower.TowerLife() || !tower.TowerWorking() {
		t.Fatalf("placed tower %T, want a working life control tower", placed[0])
	}
	return gludio, tower
}

// TestMassGatekeeperLifeTowerFalls pins the mass gatekeeper on the live
// tower count: under the siege, with the castle's one life control tower
// working, the chat window opens MassGK.htm; once the tower is driven to
// its dead form, it opens MassGK-2.htm and tele waits 480 seconds.
func TestMassGatekeeperLifeTowerFalls(t *testing.T) {
	t.Parallel()
	w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{siege: true}, room: true, lifeTower: true})
	gludio, tower := w.gludioLifeTower(t)
	f := w.spawnFolk(t, folkTemplate("CastleGatekeeper", massGatekeeperID), 30)
	w.assertMassGatekeeperChat(t, f, filled("castleteleporter/MassGK.htm", f))

	obj, _ := w.srv.State.Object(w.player)
	attacker, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player is %T, no combatant", obj)
	}
	tower.TakeDamage(tower.MaxHP()*2, attacker)
	if tower.TowerWorking() || gludio.AliveLifeTowers() != 0 {
		t.Fatalf("felled tower working %v, alive life towers %d, want dead and 0", tower.TowerWorking(), gludio.AliveLifeTowers())
	}
	drainUntilQuiet(t, w.c)

	w.assertMassGatekeeperChat(t, f, filled("castleteleporter/MassGK-2.htm", f))
	assertAnswer(t, w.command(t, f, "tele"), pageThenFinal, f, asRead("castleteleporter/MassGK-1.htm", "%delay%", "480"))
}
