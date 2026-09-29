package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// magicStrikeHitTime leaves the magic strike's interrupt window open well
// past a melee swing's hit.
const magicStrikeHitTime = 5000

// bootMagicStriker is bootWolfStriker with a magic, five-second strike, and
// the pet's combat rolls, its cast-break roll among them, fixed to roll.
func bootMagicStriker(t *testing.T, roll int) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	strike := wolfStrike()
	strike.Magic, strike.HitTime = true, magicStrikeHitTime
	h, petActor, hostile := bootWolfStrikerWith(t, strike)
	runOnPetQueue(t, petActor, func() {
		petActor.SetRollSource(func(n int) int { return min(roll, n-1) })
	})
	return h, petActor, hostile
}

// petCastOutcome is what the owner's client saw of the pet's strike.
type petCastOutcome struct {
	canceled    bool
	interrupted bool
	launched    bool
}

func readPetCastOutcome(t *testing.T, h *petWorld, petActor *summon.Actor) petCastOutcome {
	t.Helper()
	var out petCastOutcome
	for _, frame := range drainFrames(t, h.client) {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillCanceled:
			if wire.NewReader(frame[1:]).ReadInt32() == petActor.ObjectID() {
				out.canceled = true
			}
		case serverpackets.OpcodeMagicSkillLaunched:
			out.launched = true
		case serverpackets.OpcodeSystemMessage:
			if wire.NewReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageCastingInterrupted {
				out.interrupted = true
			}
		}
	}
	return out
}

// assertStrikeAborted pins an aborted strike: once its hit would have come
// due, the monster still has its full HP and the pet follows its owner
// again (PlayableCast.stop -> tryToIdle).
func assertStrikeAborted(t *testing.T, h *petWorld, petActor *summon.Actor, hostile *npc.Hostile) {
	t.Helper()
	if petActor.CastingNow() {
		t.Fatal("pet CastingNow() = true, want its strike aborted")
	}
	h.srv.Advance(t, magicStrikeHitTime*time.Millisecond)
	drainUntilQuiet(t, h.client)
	if hp, full := hostile.HP(), float64(hostile.MaxHP()); hp != full {
		t.Fatalf("monster HP = %v after the pet's strike was aborted, want untouched %v", hp, full)
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent = %v after the abort, want follow-owner", got)
	}
}

// landOnPet applies the named real effect to the pet on its queue, with the
// owner as its effector.
func landOnPet(t *testing.T, h *petWorld, petActor *summon.Actor, name string) {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := obj.(effect.Actor)
	if !ok {
		t.Fatalf("world player %T is not an effect actor", obj)
	}
	e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: name, Time: 30})
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = owner, petActor
	runOnPetQueue(t, petActor, func() { petActor.EffectList().Add(e) })
}

// TestMonsterHitBreaksPetMagicCast lands a monster's melee hit on a pet
// mid-way through a magic strike and pins Formulas.calcCastBreak for a
// summon target: a roll under the clamped rate breaks the strike (observers
// see MagicSkillCanceled, the owner reads CASTING_INTERRUPTED, no hit lands),
// a roll over it lets the strike run on and land.
func TestMonsterHitBreaksPetMagicCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		roll  int
		broke bool
	}{
		{name: "roll under the rate breaks", roll: 0, broke: true},
		{name: "roll over the rate keeps casting", roll: 99},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, hostile := bootMagicStriker(t, tt.roll)
			x, y, z := petActor.Position()
			attacker := h.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 20, Y: y, Z: z})
			drainUntilQuiet(t, h.client)
			startWolfStrike(t, h)

			attacker.DoAttack(t, petActor)
			got := readPetCastOutcome(t, h, petActor)
			if got.canceled != tt.broke || got.interrupted != tt.broke {
				t.Fatalf("pet MagicSkillCanceled = %v, CASTING_INTERRUPTED = %v, want both %v", got.canceled, got.interrupted, tt.broke)
			}
			if tt.broke {
				assertStrikeAborted(t, h, petActor, hostile)
				return
			}
			if !petActor.CastingNow() {
				t.Fatal("pet CastingNow() = false after a non-breaking hit, want its strike running")
			}
			h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
			drainUntilQuiet(t, h.client)
		})
	}
}

// monsterPetStrikeSkill is a casting monster's quick physical skill strike.
const monsterPetStrikeSkill = modelskill.ID(9104)

// TestMonsterSkillHitBreaksPetMagicCast lands a monster's physical skill
// (PDAM) on a pet mid-way through a magic strike and pins the skill-damage
// cast break for a summon target (Pdam.java: calcCastBreak before
// reduceCurrentHp): a roll under the clamped rate breaks the strike
// (observers see MagicSkillCanceled, the owner reads CASTING_INTERRUPTED, no
// hit lands, the pet follows its owner again), a roll over it lets the
// strike run on and land.
func TestMonsterSkillHitBreaksPetMagicCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		roll  int
		broke bool
	}{
		{name: "roll under the rate breaks", roll: 0, broke: true},
		{name: "roll over the rate keeps casting", roll: 99},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, hostile := bootMagicStriker(t, tt.roll)
			caster, aiCtl := h.srv.SpawnCastingHostileNPC(t, &npc.Template{
				ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000, PAtk: 10,
				AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			}, modelskill.NewTable([]modelskill.Definition{{
				ID: monsterPetStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				Offensive: true, CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
				SkillType: "PDAM", Power: 100,
			}}))
			// The owner stays unflagged: the reference resolves an NPC's
			// ONE target list with no conditions, so the strike reaches
			// the pet regardless.
			drainUntilQuiet(t, h.client)
			startWolfStrike(t, h)

			petHP := petActor.HP()
			if !caster.Queue().Post(func() { aiCtl.Cast(petActor, modelskill.Ref{ID: monsterPetStrikeSkill, Level: 1}) }) {
				t.Fatal("post monster cast: queue closed")
			}
			h.srv.AdvanceUntil(t, "monster skill landing on the pet", func() bool { return petActor.HP() < petHP })

			got := readPetCastOutcome(t, h, petActor)
			if got.canceled != tt.broke || got.interrupted != tt.broke {
				t.Fatalf("pet MagicSkillCanceled = %v, CASTING_INTERRUPTED = %v, want both %v", got.canceled, got.interrupted, tt.broke)
			}
			if tt.broke {
				assertStrikeAborted(t, h, petActor, hostile)
				return
			}
			if !petActor.CastingNow() {
				t.Fatal("pet CastingNow() = false after a non-breaking skill hit, want its strike running")
			}
			h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
			drainUntilQuiet(t, h.client)
		})
	}
}

// TestAbortCastInterruptsPetCast lands AbortCast on a pet mid-strike: the
// strike is interrupted (EffectAbortCast.onStart -> CreatureCast.interrupt),
// observers see MagicSkillCanceled, the owner reads CASTING_INTERRUPTED, and
// no hit lands.
func TestAbortCastInterruptsPetCast(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootMagicStriker(t, 99)
	startWolfStrike(t, h)
	landOnPet(t, h, petActor, "AbortCast")

	got := readPetCastOutcome(t, h, petActor)
	if !got.canceled || !got.interrupted || got.launched {
		t.Fatalf("after AbortCast: MagicSkillCanceled = %v, CASTING_INTERRUPTED = %v, MagicSkillLaunched = %v; want true, true, false",
			got.canceled, got.interrupted, got.launched)
	}
	assertStrikeAborted(t, h, petActor, hostile)
}

// TestRemoveTargetStopsPetCast lands RemoveTarget on a pet mid-strike: the
// strike stops unconditionally (EffectRemoveTarget.onStart ->
// CreatureCast.stop), so observers see MagicSkillCanceled, the owner reads
// no CASTING_INTERRUPTED, and no hit lands.
func TestRemoveTargetStopsPetCast(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootMagicStriker(t, 99)
	startWolfStrike(t, h)
	landOnPet(t, h, petActor, "RemoveTarget")

	got := readPetCastOutcome(t, h, petActor)
	if !got.canceled || got.interrupted || got.launched {
		t.Fatalf("after RemoveTarget: MagicSkillCanceled = %v, CASTING_INTERRUPTED = %v, MagicSkillLaunched = %v; want true, false, false",
			got.canceled, got.interrupted, got.launched)
	}
	assertStrikeAborted(t, h, petActor, hostile)
}
