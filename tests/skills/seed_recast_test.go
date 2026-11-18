package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	seedOfFireID    = 1285 // the id the seed-power cast conditions read
	seedNeedsTwoID  = 3401
	seedNeedsFiveID = 3402
)

// seedGatedSkill is a free self-cast whose <cond> needs fire seed power.
func seedGatedSkill(id modelskill.ID, power string) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "BUFF",
		Conditions: []modelskill.ConditionClause{{
			Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"seed_fire": power}},
			MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
		}},
	}
}

// TestSeedRecastGrowsPowerSeenByCastConditions casts a seed skill on its
// caster three times. Each recast grows the one live seed effect's power
// in place (it never stacks a second effect), and a skill whose <cond>
// needs fire seed power only casts once the seed has grown that far: a
// power-2 gate refuses after one cast and passes after two, a power-5 gate
// still refuses after three.
func TestSeedRecastGrowsPowerSeenByCastConditions(t *testing.T) {
	t.Parallel()
	seed := modelskill.Definition{
		ID: seedOfFireID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "SEED",
		Effects: []modelskill.EffectTemplate{{Name: "Seed", Time: 60, Icon: true}},
	}
	defs := []modelskill.Definition{seed, seedGatedSkill(seedNeedsTwoID, "2"), seedGatedSkill(seedNeedsFiveID, "5")}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, d := range defs {
		seedKnownSkill(t, srv, objID, int(d.ID), 1)
	}
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	// A started cast keeps the caster busy until its hit time passes, and a
	// request sent meanwhile is refused with ActionFailed, so each cast is
	// let finish before the next one goes out.
	castOpens := func(id int32) bool {
		t.Helper()
		started := castStarts(t, c, id)
		if started {
			srv.AdvanceUntil(t, "cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
			drainUntilQuiet(t, c)
		}
		return started
	}
	power := func() (p, effects int) {
		onPlayerQueue(t, srv, objID, func(pc *player.Character) {
			p, effects = pc.SeedPower(seedOfFireID), len(pc.EffectList().All())
		})
		return p, effects
	}

	if p, _ := power(); p != 0 {
		t.Fatalf("seed power before any cast = %d, want 0", p)
	}
	for recast, want := range []struct {
		power    int
		needsTwo bool
	}{{1, false}, {2, true}, {3, true}} {
		if !castOpens(seedOfFireID) {
			t.Fatalf("seed cast %d was refused", recast+1)
		}
		if p, n := power(); p != want.power || n != 1 {
			t.Fatalf("after seed cast %d: power %d with %d effects, want power %d with one effect", recast+1, p, n, want.power)
		}
		if got := castOpens(seedNeedsTwoID); got != want.needsTwo {
			t.Fatalf("after seed cast %d: power-2 gate opened = %v, want %v", recast+1, got, want.needsTwo)
		}
		if castOpens(seedNeedsFiveID) {
			t.Fatalf("after seed cast %d: power-5 gate opened at power %d", recast+1, want.power)
		}
	}
}

// castStarts sends a cast of id and reports whether it started (its
// MagicSkillUse came first) rather than being refused with a system message,
// then drains what the cast left in flight.
func castStarts(t *testing.T, c *testsupport.ScriptedClient, id int32) bool {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(id, false, false))
	first := c.Read()
	drainUntilQuiet(t, c)
	switch first[0] {
	case serverpackets.OpcodeMagicSkillUse:
		return true
	case serverpackets.OpcodeSystemMessage:
		return false
	}
	t.Fatalf("cast of %d answered with opcode %#x, want MagicSkillUse or SystemMessage", id, first[0])
	return false
}
