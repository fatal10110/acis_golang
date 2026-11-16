package skills

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CreatureCast.callSkill (CreatureCast.java:595-603). Once a
// player's (or its summon's) skill lands, every target of a CORPSE_MOB or
// AREA_CORPSE_MOB skill other than a HARVEST is force-decayed:
// Creature.forceDecay (Creature.java:1915) cancels the corpse's decay task
// and, when it was pending, runs onDecay at once. The corpse leaves the
// world (DeleteObject) and its respawn is scheduled. Leaving the world
// clears the player's selection (Creature.removeKnownObject), so a second
// corpse skill has no target: PlayableAI.tryToCast answers ActionFailed.

const corpseDrainSkillID = 1151

func corpseDrainSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: corpseDrainSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpseMob, SkillType: "DRAIN", Offensive: true, Magic: true,
		CastRange: 400, EffectRange: 900, HitTime: 500, ReuseDelay: 500,
		StaticHitTime: true, StaticReuse: true, Power: 100,
	}
}

// corpseRemovals is a decay effect that removes a due corpse from the world
// and records the respawn each removal arms.
type corpseRemovals struct {
	mu       sync.Mutex
	state    *world.State
	respawns []int32
}

func (d *corpseRemovals) Decay(actor task.DecayActor) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	obj, ok := state.Object(actor.ObjectID())
	if !ok {
		return
	}
	corpse, ok := obj.(interface {
		DecayWithRespawn(*world.State, func(id int32) func()) bool
	})
	if !ok {
		return
	}
	corpse.DecayWithRespawn(state, func(id int32) func() {
		return func() {
			d.mu.Lock()
			d.respawns = append(d.respawns, id)
			d.mu.Unlock()
		}
	})
}

func (d *corpseRemovals) armed() []int32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.respawns)
}

// corpseConsumeScene boots a player knowing def beside a fresh monster
// whose corpse stays a minute, lets prepare mark it, has the player kill it
// with the monster selected, and returns once the corpse awaits its decay.
func corpseConsumeScene(t *testing.T, def modelskill.Definition, prepare func(mob *npc.Hostile, casterID int32), opts ...gameservertest.Option) (*gameservertest.Server, int32, *npc.Hostile, *task.Decay, *corpseRemovals) {
	t.Helper()
	removals := &corpseRemovals{}
	decay, err := task.NewDecay(removals, time.Now)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Caster", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithDecay(decay),
	}, opts...)...)
	removals.mu.Lock()
	removals.state = srv.State
	removals.mu.Unlock()
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), 1)
	startInWorld(t, c)

	tmpl := spoiledMonsterTemplate()
	tmpl.Level, tmpl.CorpseTime, tmpl.Seedable = 20, 60, true
	mob := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, mob.ObjectID())
	drainUntilQuiet(t, c)
	if prepare != nil {
		prepare(mob, objID)
	}

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !mob.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	srv.AdvanceUntil(t, "the corpse awaits its decay", func() bool { return decay.Tracked(mob) })
	drainUntilQuiet(t, c)
	return srv, objID, mob, decay, removals
}

// TestCorpseSkillConsumesTheCorpse casts a corpse drain and a sweep on a
// fresh corpse: once the skill lands the corpse leaves the world at once,
// its respawn armed, and a second cast of the skill has no corpse to target.
func TestCorpseSkillConsumesTheCorpse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		def     modelskill.Definition
		prepare func(mob *npc.Hostile, casterID int32)
	}{
		{"corpse drain", corpseDrainSkill(), nil},
		{"sweep", sweeperSkill(), func(mob *npc.Hostile, casterID int32) {
			if !mob.SpoilPool().Mark(casterID) {
				t.Fatal("fresh monster already spoiled")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, _, mob, decay, removals := corpseConsumeScene(t, tc.def, tc.prepare)
			c := srv.Client

			c.Send(encodeRequestMagicSkillUse(int32(tc.def.ID), false, false))
			srv.AdvanceUntil(t, "the corpse skill consumes the corpse", mob.Decayed)
			srv.Settle(t)
			frames := collectUntilQuiet(t, c)
			if !slices.ContainsFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeDeleteObject && wireReader(f[1:]).ReadInt32() == mob.ObjectID()
			}) {
				t.Fatalf("no DeleteObject for the consumed corpse among opcodes %x", opcodesOf(frames))
			}
			if _, ok := srv.State.Object(mob.ObjectID()); ok {
				t.Fatal("the consumed corpse is still in the world")
			}
			if decay.Tracked(mob) {
				t.Fatal("the consumed corpse still awaits its decay")
			}
			if got, want := removals.armed(), []int32{mob.ObjectID()}; !slices.Equal(got, want) {
				t.Fatalf("respawns armed = %v, want %v", got, want)
			}

			// Past the reuse delay, the same skill has no corpse left to
			// target: the removal cleared the selection, so the request is
			// refused with a bare ActionFailed.
			srv.Advance(t, time.Second)
			drainUntilQuiet(t, c)
			c.Send(encodeRequestMagicSkillUse(int32(tc.def.ID), false, false))
			srv.Settle(t)
			if got, want := opcodesOf(collectUntilQuiet(t, c)), []byte{serverpackets.OpcodeActionFailed}; !slices.Equal(got, want) {
				t.Fatalf("second corpse skill sent opcodes %x, want %x", got, want)
			}
		})
	}
}

// TestHarvestLeavesTheCorpse harvests a corpse nobody sowed: the harvest
// lands (THE_HARVEST_FAILED_BECAUSE_THE_SEED_WAS_NOT_SOWN) and, being a
// harvest, leaves the corpse in the world awaiting its decay.
func TestHarvestLeavesTheCorpse(t *testing.T) {
	t.Parallel()
	srv, _, mob, decay, removals := corpseConsumeScene(t, harvestSkill(), nil,
		gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: harvestRate}))
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(harvestSkillID, false, false))
	srv.Advance(t, time.Second)
	srv.Settle(t)
	frames := collectUntilQuiet(t, c)
	if ids := systemMessageIDs(frames); !slices.Contains(ids, int32(serverpackets.SystemMessageHarvestFailedSeedNotSown)) {
		t.Fatalf("harvest system messages = %v, want THE_HARVEST_FAILED_BECAUSE_THE_SEED_WAS_NOT_SOWN", ids)
	}
	if slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeDeleteObject }) {
		t.Fatal("the harvest removed the corpse")
	}
	if mob.Decayed() || !mob.HasCorpse() {
		t.Fatalf("after the harvest decayed = %v, corpse = %v; want the corpse kept", mob.Decayed(), mob.HasCorpse())
	}
	if _, ok := srv.State.Object(mob.ObjectID()); !ok {
		t.Fatal("the harvested corpse left the world")
	}
	if !decay.Tracked(mob) {
		t.Fatal("the harvested corpse no longer awaits its decay")
	}
	if got := removals.armed(); len(got) != 0 {
		t.Fatalf("respawns armed = %v, want none", got)
	}
}
