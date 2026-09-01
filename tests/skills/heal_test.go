package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// damageToHealHeadroom drops the caster so its remaining HP sits exactly
// headroom below the stat-computed max — the observable slack a heal flow
// needs. A fresh character starts at its computed max, so the drop is
// measured from full HP down to the target.
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

func assertRestoredNumber(t *testing.T, r *wire.Reader, amount int32) {
	t.Helper()
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("restored message params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
		t.Fatalf("restored message param type = %d, want number", typ)
	}
	if got := r.ReadInt32(); got != amount {
		t.Fatalf("restored amount = %d, want %d", got, amount)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read restored message: %v", err)
	}
}

func assertRestoredBy(t *testing.T, r *wire.Reader, healer string, amount int32) {
	t.Helper()
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("restored-by message params = %d, want 2", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("restored-by message param type = %d, want text", typ)
	}
	if got := r.ReadString(); got != healer {
		t.Fatalf("restored-by healer = %q, want %q", got, healer)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
		t.Fatalf("restored-by message param type = %d, want number", typ)
	}
	if got := r.ReadInt32(); got != amount {
		t.Fatalf("restored-by amount = %d, want %d", got, amount)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read restored-by message: %v", err)
	}
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

// TestHealEffectSelfCastSendsHPRestoredMessage lands a Heal effect on a
// self-cast and checks the number-only restored message. The reported
// amount is the first applied restore, not the doubled HP actually added.
func TestHealEffectSelfCastSendsHPRestoredMessage(t *testing.T) {
	const (
		skillID   = 1221
		headroom  = 8
		healPower = 4
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					SkillType: "BUFF",
					Effects:   []modelskill.EffectTemplate{{Name: "Heal", Value: healPower}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	before, maxHP := damageToHealHeadroom(t, srv, objID, headroom)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	assertRestoredNumber(t, findSystemMessage(t, c, int32(serverpackets.SystemMessageS1HPRestored)), healPower)

	if hp := srv.PlayerCurrentHP(t, objID); hp != maxHP {
		t.Fatalf("caster HP after heal effect = %d, want restored to computed max %d (was %d)", hp, maxHP, before)
	}
	drainUntilQuiet(t, c)
}

// TestManaHealEffectSelfCastSendsMPRestoredMessage lands a ManaHeal effect
// on a self-cast and checks the number-only restored message.
func TestManaHealEffectSelfCastSendsMPRestoredMessage(t *testing.T) {
	const (
		skillID   = 1222
		headroom  = 8
		healPower = 4
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					SkillType: "BUFF",
					Effects:   []modelskill.EffectTemplate{{Name: "ManaHeal", Value: healPower}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	before := srv.PlayerCurrentMP(t, objID)
	if before < headroom {
		t.Fatalf("current MP %d leaves no %d-point restore headroom", before, headroom)
	}
	srv.DrainPlayerMP(t, objID, headroom)
	if got := srv.PlayerCurrentMP(t, objID); got != before-headroom {
		t.Fatalf("drained MP = %d, want %d", got, before-headroom)
	}

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	assertRestoredNumber(t, findSystemMessage(t, c, int32(serverpackets.SystemMessageS1MPRestored)), healPower)

	if mp := srv.PlayerCurrentMP(t, objID); mp != before {
		t.Fatalf("caster MP after mana-heal effect = %d, want restored to %d", mp, before)
	}
	drainUntilQuiet(t, c)
}

// TestHealEffectOtherCastSendsHPRestoredByHealer lands a Heal effect from
// one player onto another and checks the named restored-by message.
func TestHealEffectOtherCastSendsHPRestoredByHealer(t *testing.T) {
	const (
		skillID   = 1223
		headroom  = 8
		healPower = 4
	)
	defs := []modelskill.Definition{
		{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			CastRange: 600, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Heal", Value: healPower}},
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Healer", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	healer := srv.Client
	healerID := srv.SoleObjectID(t)
	patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
	patient := srv.DialClient(t, "player2", 1)
	seedKnownSkill(t, srv, healerID, skillID, 1)

	startInWorld(t, healer)
	startInWorldAmongPlayers(t, patient)
	drainUntilQuiet(t, healer)
	drainUntilQuiet(t, patient)

	before, maxHP := damageToHealHeadroom(t, srv, patientID, headroom)
	x, y, z := srv.PlayerPosition(t, patientID)
	healer.Send(encodeAction(patientID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, healer)
	drainUntilQuiet(t, patient)

	healer.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, healer, healerID, skillID, 1, 500, 60_000, patientID)
	assertRestoredBy(t, findSystemMessage(t, patient, int32(serverpackets.SystemMessageS2HPRestoredByS1)), "Healer", healPower)

	if hp := srv.PlayerCurrentHP(t, patientID); hp != maxHP {
		t.Fatalf("patient HP after heal effect = %d, want restored to computed max %d (was %d)", hp, maxHP, before)
	}
	drainUntilQuiet(t, healer)
	drainUntilQuiet(t, patient)
}
