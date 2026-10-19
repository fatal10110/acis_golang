package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const randomizeHateMonsterID = 31911

// randomizeHateWorld boots a player and a bystander with extra, puts the
// bystander 800 east of the player, and spawns through the server's NPC
// population a monster 50 east of the player hating the player most and the
// bystander less. It returns the monster and both players.
func randomizeHateWorld(t *testing.T, extra ...gameservertest.Option) (*npc.Hostile, attackable.Combatant, attackable.Combatant) {
	t.Helper()
	tmpl := &npc.Template{
		ID: randomizeHateMonsterID, TemplateID: randomizeHateMonsterID, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hated", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(nil),
	}, extra...)...)
	c, playerID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	bystanderID := srv.SeedCharacterFor(t, "bystander", "Bystander", 10, 0).ID
	bc := srv.DialClient(t, "bystander", 1)
	startInWorld(t, bc)
	combatant := func(id int32) attackable.Combatant {
		obj, ok := srv.State.Player(id)
		if !ok {
			t.Fatalf("world.Player(%d) missing", id)
		}
		return obj.(attackable.Combatant)
	}
	hated, bystander := combatant(playerID), combatant(bystanderID)
	x, y, z := srv.PlayerPosition(t, playerID)
	obj, _ := srv.State.Player(bystanderID)
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	pc.TeleportTo(x+800, y, z, 0)
	if err := srv.NpcSpawns.SpawnFixed(tmpl, x+50, y, z, 0, 0); err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	var monster *npc.Hostile
	for _, obj := range srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == randomizeHateMonsterID {
			monster = h
		}
	}
	if monster == nil {
		t.Fatal("monster not spawned")
	}
	drainUntilQuiet(t, bc)
	drainUntilQuiet(t, c)
	onQueue(t, monster.Queue(), func() {
		monster.AddDamageHate(hated, 0, 1000)
		monster.AddDamageHate(bystander, 0, 500)
	})
	return monster, hated, bystander
}

// randomizeHate lands a RandomizeHate effect (Confusion's) on monster and
// returns the attacker it hates most afterwards.
func randomizeHate(t *testing.T, monster *npc.Hostile) int32 {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "RandomizeHate", Count: 1, Time: 1})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = monster, monster
	var most int32
	onQueue(t, monster.Queue(), func() {
		monster.EffectList().Add(e)
		if top, ok := monster.AI().Threats().MostHated(); ok {
			most = top.Attacker.ObjectID()
		}
	})
	return most
}

// TestRandomizeHateGatesByPartyRange: attack randomization only turns a
// monster on an attacker within players.properties' PartyRange
// (AggroList.randomizeAttack's canAutoAttack(attacker, PARTY_RANGE, true)).
// A bystander 750 from the monster takes the top of its hate at the
// shipped 1500, and stays below the player it hates most at 500.
func TestRandomizeHateGatesByPartyRange(t *testing.T) {
	t.Parallel()
	t.Run("shipped range", func(t *testing.T) {
		t.Parallel()
		monster, _, bystander := randomizeHateWorld(t)
		if got := randomizeHate(t, monster); got != bystander.ObjectID() {
			t.Fatalf("most hated after the randomization = %d, want the bystander %d", got, bystander.ObjectID())
		}
	})
	t.Run("range 500", func(t *testing.T) {
		t.Parallel()
		monster, hated, _ := randomizeHateWorld(t, gameservertest.WithPartyRange(500))
		if got := randomizeHate(t, monster); got != hated.ObjectID() {
			t.Fatalf("most hated after the randomization = %d, want still the player %d", got, hated.ObjectID())
		}
	})
}
