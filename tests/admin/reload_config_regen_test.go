package admin

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const reloadRegenMonsterID = 31921

// onNPCQueue runs fn on h's queue and waits for it.
func onNPCQueue(t *testing.T, h *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() { defer close(done); fn() }) {
		t.Fatal("post: npc queue closed")
	}
	<-done
}

// TestAdminReloadConfigRegenMultipliersReachEveryone: //reload config puts
// the reloaded HpRegenMultiplier, MpRegenMultiplier and CpRegenMultiplier
// in force for the player already in the world and for the monster the NPC
// population spawned before it, as the reference reads them on each
// regeneration: at 2, 3 and 4 their rates become those multiples of the
// boot ones.
func TestAdminReloadConfigRegenMultipliersReachEveryone(t *testing.T) {
	t.Parallel()
	class := gameservertest.ClassTemplate()
	class.HPRegenTable, class.MPRegenTable, class.CPRegenTable = []float64{2}, []float64{0.9}, []float64{2}
	monster := &npc.Template{
		ID: reloadRegenMonsterID, TemplateID: reloadRegenMonsterID, Type: "Monster", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		HPRegen: 3, MPRegen: 1.5,
	}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			Regen: creature.RegenMultipliers{HP: 2, MP: 3, CP: 4, Configured: true},
		})))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	c := onlineCharacter(t, srv, gmID)
	if err := srv.NpcSpawns.SpawnFixed(monster, 3000, 0, 0, 0, 0); err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	drain(t, gm)
	var m *npc.Hostile
	for _, obj := range srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == reloadRegenMonsterID {
			m = h
		}
	}
	if m == nil {
		t.Fatal("monster not spawned")
	}
	hp, mp, cp := c.HPRegenRate(), c.MPRegenRate(), c.CPRegenRate()
	npcHP, npcMP := m.HPRegenRate(), m.MPRegenRate()
	if hp <= 0 || mp <= 0 || cp <= 0 || npcHP <= 0 || npcMP <= 0 {
		t.Fatalf("boot rates: player %v/%v/%v, monster %v/%v; want every rate positive", hp, mp, cp, npcHP, npcMP)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)

	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if got := [3]float64{c.HPRegenRate(), c.MPRegenRate(), c.CPRegenRate()}; !near(got[0], hp*2) || !near(got[1], mp*3) || !near(got[2], cp*4) {
		t.Fatalf("player rates after the reload = %v, want %v", got, [3]float64{hp * 2, mp * 3, cp * 4})
	}
	if got := [2]float64{m.HPRegenRate(), m.MPRegenRate()}; !near(got[0], npcHP*2) || !near(got[1], npcMP*3) {
		t.Fatalf("monster rates after the reload = %v, want %v", got, [2]float64{npcHP * 2, npcMP * 3})
	}
}

// TestAdminReloadConfigPartyRangeReachesHateRandomization: attack
// randomization reads the party range in force (AggroList.randomizeAttack
// reads PARTY_RANGE on each call). At a boot range of 500 a monster does
// not turn on a bystander 750 away; once //reload config puts 1500 in
// force, it does.
func TestAdminReloadConfigPartyRangeReachesHateRandomization(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithPartyRange(500),
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			KillRewards: &manager.KillRewardSettings{
				Rates:      item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1},
				PartyRange: 1500, RateXP: 1, RateSP: 1,
			},
		})))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	bystanderID := srv.SeedCharacterFor(t, "bystander", "Bystander", 1, 0).ID
	bc := srv.DialClient(t, "bystander", 1)
	enterWorld(t, bc)
	x, y, z := srv.PlayerPosition(t, gmID)
	onlineCharacter(t, srv, bystanderID).TeleportTo(x+800, y, z, 0)
	monster := srv.SpawnHostileNPCTemplateAt(t, reloadRewardMonster(reloadRegenMonsterID), location.Location{X: x + 50, Y: y, Z: z})
	drain(t, bc)
	drain(t, gm)
	combatant := func(id int32) attackable.Combatant {
		obj, ok := srv.State.Player(id)
		if !ok {
			t.Fatalf("world.Player(%d) missing", id)
		}
		return obj.(attackable.Combatant)
	}
	gmC, bystander := combatant(gmID), combatant(bystanderID)
	onNPCQueue(t, monster, func() {
		monster.AddDamageHate(gmC, 0, 1000)
		monster.AddDamageHate(bystander, 0, 500)
	})
	randomize := func() int32 {
		e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "RandomizeHate", Count: 1, Time: 1})
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		e.Effector, e.Effected = monster, monster
		var most int32
		onNPCQueue(t, monster, func() {
			monster.EffectList().Add(e)
			if top, ok := monster.AI().Threats().MostHated(); ok {
				most = top.Attacker.ObjectID()
			}
		})
		return most
	}

	if got := randomize(); got != gmID {
		t.Fatalf("most hated at range 500 = %d, want still the GM %d", got, gmID)
	}
	// The first effect runs out before the second lands.
	srv.Advance(t, time.Second)
	srv.TickEffects()
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if got := randomize(); got != bystanderID {
		t.Fatalf("most hated at the reloaded range 1500 = %d, want the bystander %d", got, bystanderID)
	}
}
