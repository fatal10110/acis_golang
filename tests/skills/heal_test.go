package skills

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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
// self-cast: each MP payment reports its own StatusUpdate, and the hit's
// trailing StatusUpdate reports the restored HP, clamped at the
// stat-computed max.
func TestHealSelfCastRestoresDamagedCaster(t *testing.T) {
	t.Parallel()
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
	assertCasterStatus(t, srv, c.Read(), objID, before, 28)
	readCastStartFrames(t, c, objID, 1218, 1, 500, 60_000, objID)
	// The final MP payment reports itself before the heal lands.
	assertCasterStatus(t, srv, c.Read(), objID, before, 25)

	assertRestoredNumber(t, findSystemMessage(t, c, int32(serverpackets.SystemMessageS1HPRestored)), 8)
	assertStatusAttrs(t, c.Read(), objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: maxHP},
	})
	if hp := srv.PlayerCurrentHP(t, objID); hp != maxHP {
		t.Fatalf("caster HP after heal = %d, want restored to computed max %d (was %d)", hp, maxHP, before)
	}
	drainUntilQuiet(t, c)
}

func TestManaHealSelfCastSendsMPRestoredMessage(t *testing.T) {
	t.Parallel()
	const (
		skillID  = 1219
		headroom = 8
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "MANAHEAL", Power: 50,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	before := srv.PlayerCurrentMP(t, objID)
	if before < headroom {
		t.Fatalf("current MP %d leaves no %d-point restore headroom", before, headroom)
	}
	srv.DrainPlayerMP(t, objID, headroom)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	assertRestoredNumber(t, findSystemMessage(t, c, int32(serverpackets.SystemMessageS1MPRestored)), headroom)
	if mp := srv.PlayerCurrentMP(t, objID); mp != before {
		t.Fatalf("caster MP after mana heal = %d, want %d", mp, before)
	}
	drainUntilQuiet(t, c)
}

func TestCombatPointHealSelfCastSendsCPRestoredMessage(t *testing.T) {
	t.Parallel()
	const (
		damageSkillID = 1227
		healSkillID   = 1228
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{ID: damageSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 500, StaticHitTime: true, SkillType: "CPDAMPERCENT", Power: 50},
			{ID: healSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, HitTime: 500, StaticHitTime: true, SkillType: "COMBATPOINTHEAL", Power: 99_999},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, damageSkillID, 1)
	seedKnownSkill(t, srv, objID, healSkillID, 1)
	startInWorld(t, c)

	maxCP := srv.PlayerMaxCP(t, objID)
	c.Send(encodeRequestMagicSkillUse(damageSkillID, false, false))
	readCastStartFrames(t, c, objID, damageSkillID, 1, 500, 0, objID)
	srv.Advance(t, 700*time.Millisecond)
	drainUntilQuiet(t, c)
	if current := srv.PlayerCurrentCP(t, objID); current >= maxCP {
		t.Fatalf("caster CP after damage = %d, want below %d", current, maxCP)
	}

	want := maxCP - srv.PlayerCurrentCP(t, objID)
	c.Send(encodeRequestMagicSkillUse(healSkillID, false, false))
	readCastStartFrames(t, c, objID, healSkillID, 1, 500, 0, objID)
	assertRestoredNumber(t, findSystemMessage(t, c, int32(serverpackets.SystemMessageS1CPWillBeRestored)), int32(want))
	if cp := srv.PlayerCurrentCP(t, objID); cp != maxCP {
		t.Fatalf("caster CP after heal = %d, want restored to computed max %d", cp, maxCP)
	}
	drainUntilQuiet(t, c)
}

// TestHealOverTimeTicksRestoreDamagedCaster lands a heal-over-time on a
// damaged caster and verifies each production effect sweep visibly restores
// HP until the caster reaches full health, where the ticks fall silent.
func TestHealOverTimeTicksRestoreDamagedCaster(t *testing.T) {
	t.Parallel()
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
	srv.Advance(t, 700*time.Millisecond)
	drainUntilQuiet(t, c)

	before := srv.PlayerCurrentHP(t, objID)

	// The production sweep only advances effects whose period elapsed. The
	// first tick restores the whole remaining gap to the stat-computed max
	// and reports it; every later tick heals nothing and stays
	// silent, matching the reference's bypass on a zero-amount setter.
	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()
	frame := c.ReadWithTimeout(time.Second)
	if frame == nil {
		t.Fatalf("tick 1: no StatusUpdate arrived")
	}
	// PlayerStatus.broadcastStatusUpdate (PlayerStatus.java:408-416): self
	// only, CUR_HP, CUR_MP, CUR_CP, MAX_CP.
	assertStatusAttrs(t, frame, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: maxHP},
		{Type: serverpackets.StatusCurrentMP, Value: srv.PlayerCurrentMP(t, objID)},
		{Type: serverpackets.StatusCurrentCP, Value: srv.PlayerCurrentCP(t, objID)},
		{Type: serverpackets.StatusMaxCP, Value: srv.PlayerMaxCP(t, objID)},
	})
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP {
		t.Fatalf("tick 1: HP = %d, want restored to computed max %d (was %d)", got, maxHP, before)
	}

	srv.Advance(t, 1100*time.Millisecond)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// collectUntilQuiet returns every frame the client receives until the server
// stays quiet for a full read timeout.
func collectUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

// hpRestoredAmount returns the amount of the single S1_HP_RESTORED message
// among frames.
func hpRestoredAmount(t *testing.T, frames [][]byte) int32 {
	t.Helper()
	found, amount := 0, int32(0)
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != int32(serverpackets.SystemMessageS1HPRestored) {
			continue
		}
		if params, typ := r.ReadInt32(), r.ReadInt32(); params != 1 || typ != serverpackets.SystemMessageParamNumber {
			t.Fatalf("S1_HP_RESTORED params = %d type %d, want one number", params, typ)
		}
		amount = r.ReadInt32()
		found++
	}
	if found != 1 {
		t.Fatalf("S1_HP_RESTORED messages = %d, want 1", found)
	}
	return amount
}

// hasAbnormalIcon reports whether any AbnormalStatusUpdate among frames
// carries skillID's icon.
func hasAbnormalIcon(t *testing.T, frames [][]byte, skillID int32) bool {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
			continue
		}
		for _, e := range readAbnormalStatusUpdateEntriesFromFrame(t, frame) {
			if e.SkillID == skillID {
				return true
			}
		}
	}
	return false
}

// TestHealCastLandsItsHealOverTime casts a Greater Heal-shaped HEAL: the
// instant heal lands and the skill's heal-over-time effect lands with it,
// showing its icon to the caster.
func TestHealCastLandsItsHealOverTime(t *testing.T) {
	t.Parallel()
	const (
		skillID = 1217
		power   = 5
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "HEAL", Power: power,
			Effects: []modelskill.EffectTemplate{{
				Name: "HealOverTime", Value: 15, Count: 15, Time: 1, Icon: true,
				StackType: "life_force_others", StackOrder: 2,
			}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	maxHP := srv.PlayerMaxHP(t, objID)
	srv.DamagePlayerHP(t, objID, srv.PlayerCurrentHP(t, objID)-1)
	before := srv.PlayerCurrentHP(t, objID)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	frames := collectUntilQuiet(t, c)

	if !hasAbnormalIcon(t, frames, skillID) {
		t.Fatalf("no AbnormalStatusUpdate carried the heal-over-time icon of skill %d", skillID)
	}
	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("caster missing from the world")
	}
	caster, ok := live.(interface{ MAtk() float64 })
	if !ok {
		t.Fatalf("caster %T exposes no M.Atk", live)
	}
	want := int32(power + math.Sqrt(float64(int(caster.MAtk()))))
	if int(want) >= maxHP-before {
		t.Fatalf("heal %d leaves no headroom under max HP %d", want, maxHP)
	}
	healed := hpRestoredAmount(t, frames)
	if healed != want {
		t.Fatalf("instant heal = %d, want %d", healed, want)
	}
	if got := srv.PlayerCurrentHP(t, objID); got != before+int(healed) {
		t.Fatalf("HP after heal = %d, want %d + %d", got, before, healed)
	}
}

// TestStaticHealCastBuffsBeforeHealing casts a Battle Roar-shaped
// HEAL_STATIC whose buff raises max HP by the heal's own power. The buff
// lands first, so the heal fills the raised ceiling instead of clamping at
// the old one.
func TestStaticHealCastBuffsBeforeHealing(t *testing.T) {
	t.Parallel()
	const (
		skillID  = 3125
		power    = 30
		headroom = 10
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "HEAL_STATIC", Power: power,
			Effects: []modelskill.EffectTemplate{{
				Name: "Buff", Time: 120, Icon: true, StackType: "abnormal_item", StackOrder: 1,
				Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: power}},
			}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	before, maxHP := damageToHealHeadroom(t, srv, objID, headroom)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	frames := collectUntilQuiet(t, c)

	if !hasAbnormalIcon(t, frames, skillID) {
		t.Fatalf("no AbnormalStatusUpdate carried the buff icon of skill %d", skillID)
	}
	if got := hpRestoredAmount(t, frames); got != power {
		t.Fatalf("static heal = %d, want the full %d under the raised max HP", got, power)
	}
	if got := srv.PlayerMaxHP(t, objID); got != maxHP+power {
		t.Fatalf("max HP after the buff = %d, want %d", got, maxHP+power)
	}
	if got := srv.PlayerCurrentHP(t, objID); got != before+power {
		t.Fatalf("HP after heal = %d, want %d", got, before+power)
	}
}
