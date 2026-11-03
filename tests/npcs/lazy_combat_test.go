package npcs

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	lazyCombatMonsterID = 31951
	lazyCombatSkillID   = 4136
	lazyCombatAdena     = 41
)

// lazyCombatWorld boots the plain fixture character in the world beside
// an NPC population that spawns lazyCombatMonster, casting its skill, with
// the AI task driven by hand, and extra on top.
func lazyCombatWorld(t *testing.T, extra ...gameservertest.Option) (*folkWorld, *npc.Template) {
	t.Helper()
	monster := &npc.Template{
		ID: lazyCombatMonsterID, TemplateID: lazyCombatMonsterID, Name: "Lazy Dummy", Type: "Monster", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Skills: map[int]int{lazyCombatSkillID: 1},
		Drops:  []item.DropCategory{{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: lazyCombatAdena, Max: lazyCombatAdena, Chance: 100}}}},
	}
	buff := modelskill.Definition{
		ID: lazyCombatSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
	opts := append([]gameservertest.Option{
		gameservertest.WithAITask(),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster})),
		gameservertest.WithNpcDropRates(item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{buff})),
	}, extra...)
	w := bootFolkWorld(t, nil, opts...)
	return w, monster
}

// spawnLazy spawns the monster dx along x from the character.
func (w *folkWorld) spawnLazy(t *testing.T, tmpl *npc.Template, dx int) *npc.Hostile {
	t.Helper()
	h, err := w.srv.NpcSpawns.SpawnFixedHostile(tmpl, w.at.X+dx, w.at.Y, w.at.Z, 0)
	if err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	return h
}

// assertNoCombatParts fails unless h holds none of its combat-only parts:
// no threat, hate or desire table, no effect list and no cast controller.
func assertNoCombatParts(t *testing.T, h *npc.Hostile, when string) {
	t.Helper()
	brain := h.AI()
	if brain.ThreatsIfBuilt() != nil || brain.HatesIfBuilt() != nil || brain.DesiresIfBuilt() != nil {
		t.Errorf("%s: threat, hate, desire tables built = %v, %v, %v, want none", when,
			brain.ThreatsIfBuilt() != nil, brain.HatesIfBuilt() != nil, brain.DesiresIfBuilt() != nil)
	}
	if h.EffectListIfBuilt() != nil {
		t.Errorf("%s: effect list built, want none", when)
	}
	if brain.CastControllerIfBuilt() != nil || h.CastControl() != nil {
		t.Errorf("%s: cast controller built, want none", when)
	}
}

// TestIdleSpawnedHostileBuildsNoCombatParts spawns a monster nobody fights
// through the NPC population: its spawn, an idle AI cycle beside a player
// and its region going quiet (which stops its effects, clears its
// attackers and sends it back to peace) build none of its combat-only
// parts. Its first threat then builds the threat table alone.
func TestIdleSpawnedHostileBuildsNoCombatParts(t *testing.T) {
	t.Parallel()
	w, monster := lazyCombatWorld(t)
	h := w.spawnLazy(t, monster, 100)
	w.srv.Settle(t)
	assertNoCombatParts(t, h, "spawned")

	before := h.LifeTime()
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() error: %v", err)
	}
	w.srv.Settle(t)
	if h.LifeTime() == before {
		t.Fatal("test setup: the AI task did not tick the monster")
	}
	assertNoCombatParts(t, h, "after an idle AI cycle")

	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AdvanceUntil(t, "the region going quiet", func() bool {
		_, active := w.srv.State.RegionActivity(h)
		return !active
	})
	w.srv.Settle(t)
	if !h.AISleeping() {
		t.Fatal("test setup: the monster did not take its region going quiet")
	}
	assertNoCombatParts(t, h, "after its region went quiet")

	other := w.spawnLazy(t, monster, 200)
	h.AI().AddDamageHate(other, 1, 1)
	if h.AI().ThreatsIfBuilt() == nil {
		t.Fatal("first threat built no threat table")
	}
	if h.AI().HatesIfBuilt() != nil || h.AI().DesiresIfBuilt() != nil || h.EffectListIfBuilt() != nil || h.AI().CastControllerIfBuilt() != nil {
		t.Fatal("first threat built more than the threat table")
	}
}

// coldHit is what one attacker saw of the cold target's combat parts.
type coldHit struct {
	threats *attackable.ThreatTable
	hates   *attackable.HateTable
	desires *ai.DesireQueue
	effects *effect.List
	cast    ai.CastController
}

// TestColdHostileFirstHitsFromTwoAttackers has two monsters reach cold
// targets at once, each from its own goroutine as its queue would: every
// combat part a first hit, hate, desire, buff, cast or swing check builds
// is built once, both attackers see the same one, and each attacker's
// threat, hate and desire land in it as on a target reached by one attacker
// after the other.
func TestColdHostileFirstHitsFromTwoAttackers(t *testing.T) {
	t.Parallel()
	w, monster := lazyCombatWorld(t)
	attackers := [2]*npc.Hostile{w.spawnLazy(t, monster, 300), w.spawnLazy(t, monster, 340)}
	reach := func(target, attacker *npc.Hostile) {
		brain := target.AI()
		brain.AddDamageHate(attacker, 10, 10)
		brain.AddHate(attacker, 5)
		brain.AddAttackDesire(attacker, 10)
	}
	// reference is reached by one attacker after the other.
	reference := w.spawnLazy(t, monster, 380)
	for _, attacker := range attackers {
		reach(reference, attacker)
	}
	for i := range 20 {
		target := w.spawnLazy(t, monster, 100+i*5)
		var start, done sync.WaitGroup
		start.Add(1)
		var seen [2]coldHit
		for j, attacker := range attackers {
			done.Go(func() {
				start.Wait()
				reach(target, attacker)
				brain := target.AI()
				seen[j] = coldHit{
					threats: brain.Threats(),
					hates:   brain.Hates(),
					desires: brain.Desires(),
					effects: target.EffectList(),
					cast:    brain.CastController(),
				}
				// The swing gate builds the attack controller; a monster
				// may not attack another, so its answer is no.
				if brain.CanAttack(attacker) {
					t.Errorf("target %d: CanAttack(a monster) = true, want false", i)
				}
			})
		}
		start.Done()
		done.Wait()

		a, b := seen[0], seen[1]
		if a.threats != b.threats || a.hates != b.hates || a.desires != b.desires || a.effects != b.effects || a.cast != b.cast {
			t.Fatalf("target %d: the attackers saw different combat parts: %+v and %+v", i, a, b)
		}
		if a.threats != target.AI().ThreatsIfBuilt() || a.effects != target.EffectListIfBuilt() || a.cast != target.AI().CastControllerIfBuilt() {
			t.Fatalf("target %d: the parts the attackers saw are not the ones published", i)
		}
		if a.cast == nil {
			t.Fatalf("target %d: no cast controller built for a monster with skills", i)
		}
		// Damage, crowd control and the cast observers are shown drive the
		// live cast controller the AI one holds.
		if _, ok := target.CastControl().(*actorcast.Controller); !ok {
			t.Fatalf("target %d: CastControl() = %T, want the live *cast.Controller", i, target.CastControl())
		}
		for _, attacker := range attackers {
			if got, want := target.AI().Threats().Hate(attacker), reference.AI().Threats().Hate(attacker); got != want {
				t.Fatalf("target %d: threat hate of %d = %v, want %v as one attacker after the other leaves", i, attacker.ObjectID(), got, want)
			}
			if got, want := target.AI().Hates().Hate(attacker), reference.AI().Hates().Hate(attacker); got != want {
				t.Fatalf("target %d: skill hate of %d = %v, want %v", i, attacker.ObjectID(), got, want)
			}
		}
		if got, want := target.AI().Desires().Len(), reference.AI().Desires().Len(); got != want {
			t.Fatalf("target %d: %d desires queued, want %d", i, got, want)
		}
	}
}

// TestSpawnedHostileKillPaysItsRewards kills a monster the NPC population
// spawned, whose death rewards it builds only at the death: the kill drops
// the monster's adena, reserved to the killer.
func TestSpawnedHostileKillPaysItsRewards(t *testing.T) {
	t.Parallel()
	w, monster := lazyCombatWorld(t)
	h := w.spawnLazy(t, monster, 50)
	w.srv.Settle(t)
	drainFrames(t, w.c)
	if !h.TakeDamage(1_000_000, w.rider(t)) {
		t.Fatal("the lethal hit did not kill the monster")
	}
	w.srv.Settle(t)
	drops := w.srv.GroundItems.Snapshots(nil)
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != lazyCombatAdena || drops[0].OwnerID != w.player {
		t.Fatalf("ground drops = %+v, want the monster's %d adena reserved to %d", drops, lazyCombatAdena, w.player)
	}
}
