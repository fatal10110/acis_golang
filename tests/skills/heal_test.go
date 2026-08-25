package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// damageToHealHeadroom drops the caster so its remaining HP sits exactly
// headroom below the stat-computed max — the observable slack a heal flow
// needs. Fresh fixture characters currently sit above their computed max
// (#1896), so the drop is measured from the current value down to the target,
// not from full HP.
func damageToHealHeadroom(t *testing.T, srv *gameservertest.Server, objID, headroom int32) (before, maxHP int) {
	t.Helper()
	maxHP = srv.PlayerMaxHP(t, objID)
	if int(headroom) >= maxHP {
		t.Fatalf("computed max HP %d leaves no %d-point heal headroom", maxHP, headroom)
	}
	target := maxHP - int(headroom)
	srv.DamagePlayerHP(t, objID, srv.PlayerCurrentHP(t, objID)-target)
	before = srv.PlayerCurrentHP(t, objID)
	if before != target {
		t.Fatalf("damaged HP = %d, want %d", before, target)
	}
	return before, maxHP
}

// TestHealSelfCastRestoresDamagedCaster heals a damaged caster through a real
// self-cast: the hit-time StatusUpdate reports both the restored HP — clamped
// at the stat-computed max — and the charged MP.
func TestHealSelfCastRestoresDamagedCaster(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 1218, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "HEAL", Power: 50,
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1218, 1)
	startInWorld(t, c)

	before, maxHP := damageToHealHeadroom(t, srv, objID, 8)

	c.Send(encodeRequestMagicSkillUse(1218, false, false))
	readCastStartFrames(t, c, objID, 1218, 1, 500, 60_000, objID)

	assertStatusAttrs(t, c.Read(), objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: maxHP},
		{Type: serverpackets.StatusCurrentMP, Value: srv.PlayerCurrentMP(t, objID)},
	})
	if hp := srv.PlayerCurrentHP(t, objID); hp != maxHP {
		t.Fatalf("caster HP after heal = %d, want restored to computed max %d (was %d)", hp, maxHP, before)
	}
	drainUntilQuiet(t, c)
}

// TestHealOverTimeTicksRestoreDamagedCaster lands a heal-over-time on a
// damaged caster and verifies each production effect sweep visibly restores
// HP until the caster reaches full health, where the ticks fall silent.
func TestHealOverTimeTicksRestoreDamagedCaster(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 1220, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, StaticHitTime: true, SkillType: "HOT",
					Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Value: 100, Count: 3, Time: 1}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1220, 1)
	startInWorld(t, c)

	_, maxHP := damageToHealHeadroom(t, srv, objID, 10)

	c.Send(encodeRequestMagicSkillUse(1220, false, false))
	readCastStartFrames(t, c, objID, 1220, 1, 500, 0, objID)
	// Let the 500ms hit fire before draining, so the effect-start frames
	// land inside this drain instead of the first tick's read.
	time.Sleep(700 * time.Millisecond)
	drainUntilQuiet(t, c)

	before := srv.PlayerCurrentHP(t, objID)

	// The production sweep only advances effects whose period elapsed. The
	// first tick restores the whole remaining gap to the stat-computed max
	// and reports both bounds; every later tick heals nothing and stays
	// silent, matching the reference's bypass on a zero-amount setter.
	time.Sleep(1100 * time.Millisecond)
	srv.TickEffects()
	frame := c.ReadWithTimeout(time.Second)
	if frame == nil {
		t.Fatalf("tick 1: no StatusUpdate arrived")
	}
	assertStatusAttrs(t, frame, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusMaxHP, Value: maxHP},
		{Type: serverpackets.StatusCurrentHP, Value: maxHP},
	})
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP {
		t.Fatalf("tick 1: HP = %d, want restored to computed max %d (was %d)", got, maxHP, before)
	}

	time.Sleep(1100 * time.Millisecond)
	srv.TickEffects()
	if frame := c.ReadWithTimeout(time.Second); frame != nil {
		t.Fatalf("full-health tick frame opcode %#x, want silence", frame[0])
	}
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP {
		t.Fatalf("HP after full-health tick = %d, want unchanged %d", got, maxHP)
	}
	drainUntilQuiet(t, c)
}
