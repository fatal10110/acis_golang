package skills

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seedKnownSkill persists a known skill level for objID before the client
// selects the character, mirroring a previously learned skill restored from
// character_skills.
func seedKnownSkill(t *testing.T, srv *gameservertest.Server, objID int32, skillID, level int) {
	t.Helper()
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, skillID, level); err != nil {
		t.Fatalf("seed known skill %d: %v", skillID, err)
	}
}

// readCastStartFrames consumes the packets a successful active cast emits up
// to and including MagicSkillLaunched, asserting the ack's identity and
// timing fields along the way.
func readCastStartFrames(t *testing.T, c clientReader, objID, skillID, level, hitTime, reuse, targetID int32) {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wireReader(reply[1:])
	caster, gotTarget, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != objID || gotTarget != targetID || sid != skillID || lvl != level {
		t.Fatalf("MagicSkillUse ids = caster %d target %d skill %d level %d, want %d/%d/%d/%d",
			caster, gotTarget, sid, lvl, objID, targetID, skillID, level)
	}
	gotHit, gotReuse := r.ReadInt32(), r.ReadInt32()
	if gotHit != hitTime || gotReuse != reuse {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want %d/%d", gotHit, gotReuse, hitTime, reuse)
	}

	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, skillID, level)

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeSetupGauge, "SetupGauge")
	r = wireReader(reply[1:])
	color, current, maxTime := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	wantCurrent, wantMax := int32(0), int32(0)
	if hitTime > 0 {
		wantCurrent, wantMax = hitTime, hitTime
	}
	if color != int32(serverpackets.GaugeBlue) || current != wantCurrent || maxTime != wantMax {
		t.Fatalf("SetupGauge = color %d current %d max %d, want blue/%d/%d", color, current, maxTime, wantCurrent, wantMax)
	}

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	r = wireReader(reply[1:])
	launchedCaster, launchedSkill, launchedLevel, count, launchedTarget := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if launchedCaster != objID || launchedSkill != skillID || launchedLevel != level || count != 1 || launchedTarget != targetID {
		t.Fatalf("MagicSkillLaunched = caster %d skill %d level %d count %d target %d, want %d/%d/%d/1/%d",
			launchedCaster, launchedSkill, launchedLevel, count, launchedTarget, objID, skillID, level, targetID)
	}
}

// clientReader narrows the scripted-client surface to frame reads.
type clientReader interface {
	Read() []byte
}

// TestCastActiveSkillChargesMPAndStartsReuse walks a full self-cast: the
// MagicSkillUse ack with the definition's timing, the use message, the cast
// bar, the launch report, and the MP charge in a StatusUpdate. A second
// request while the skill is disabled is rejected with the
// prepared-for-reuse message and an ActionFailed — the reuse gate itself,
// since the reference never answers a SkillCoolTime request: the client
// learns remaining reuse from unsolicited pushes, not queries.
func TestCastActiveSkillChargesMPAndStartsReuse(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "DUMMY",
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 3, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(3, false, false))
	readCastStartFrames(t, c, objID, 3, 1, 500, 60_000, objID)
	assertStatusAttrs(t, c.Read(), objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	drainUntilQuiet(t, c)

	// The reference registers an empty case for RequestSkillCoolTime:
	// sending one must stay silent.
	c.Send(encodeRequestSkillCoolTime())
	c.ExpectNoFrame()

	c.Send(encodeRequestMagicSkillUse(3, false, false))
	reply := c.Read()
	assertSystemMessageSkillFrame(t, reply, serverpackets.SystemMessageS1PreparedForReuse, 3, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "recast rejection")
	drainUntilQuiet(t, c)
}

func TestCastSkillMasteryCooldownBypass(t *testing.T) {
	for _, tt := range []struct {
		name      string
		skillType string
		power     float64
		roll      float64
		mastery   bool
	}{
		{name: "mastery", skillType: "DUMMY", power: 1000, roll: 99.5, mastery: true},
		{name: "no mastery", skillType: "DUMMY", power: 1, roll: 99.5},
		{name: "fishing", skillType: "FISHING", power: 100, roll: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := modelskill.Definition{
				ID: 1855, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: tt.skillType,
			}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
			startInWorld(t, c)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatalf("world player %d missing", objID)
			}
			ch, ok := obj.(interface {
				AddStatFuncs([]effect.Mod)
				SetFloatRollSource(func(float64) float64)
				HasSkillReuse(int32) bool
				SkillDisabled(int32) bool
			})
			if !ok {
				t.Fatalf("world player %d = %T, want mastery-capable player", objID, obj)
			}
			ch.AddStatFuncs([]effect.Mod{{Stat: stat.SkillMastery, Op: effect.OpSet, Value: tt.power}})
			ch.SetFloatRollSource(func(n float64) float64 {
				if n != 100 {
					t.Fatalf("mastery roll bound = %g, want 100", n)
				}
				return tt.roll
			})

			c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			readCastStartFrames(t, c, objID, int32(def.ID), int32(def.Level), 500, 60_000, objID)

			key := cast.ReuseKey(def)
			if got := ch.HasSkillReuse(key); got != !tt.mastery {
				t.Fatalf("HasSkillReuse(%d) = %v, want %v", key, got, !tt.mastery)
			}
			if got := ch.SkillDisabled(key); got != !tt.mastery {
				t.Fatalf("SkillDisabled(%d) = %v, want %v", key, got, !tt.mastery)
			}
			drainUntilQuiet(t, c)
		})
	}
}

// TestCastRejectedInsufficientMP verifies a caster without the MP pays
// nothing: the not-enough-MP message and no cast starts.
func TestCastRejectedInsufficientMP(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					MPConsume: 100, SkillType: "DUMMY",
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 3, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(3, false, false))
	reply := c.Read()
	assertStaticSystemMessage(t, reply, serverpackets.SystemMessageNotEnoughMP)
	drainUntilQuiet(t, c)
}

func TestSelfCubicCastRejectsWhenListIsFull(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: 4, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 4, 1)
	startInWorld(t, c)

	actor, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("caster state missing")
	}
	cubics, ok := actor.(interface {
		AddOrRefreshCubic(cubic.ID, bool) (bool, bool)
	})
	if !ok {
		t.Fatalf("caster state %T does not expose AddOrRefreshCubic", actor)
	}
	cubics.AddOrRefreshCubic(cubic.Storm, false)

	c.Send(encodeRequestMagicSkillUse(4, false, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCubicSummoningFailed)
	drainUntilQuiet(t, c)
}

func TestCastItemConsumeShortageNamesSkill(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			ItemConsumeID: 57, ItemConsumeCount: 1,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(5, false, false))
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, 5, 1)
	drainUntilQuiet(t, c)
}

// TestGroundTargetCastRecordsTargetAndAppliesBuff covers the ground-cast
// branch: the cast walks through ValidateLocation, lands the buff on the
// caster (icon visible in AbnormalStatusUpdate), and charges MP.
func TestGroundTargetCastRecordsTargetAndAppliesBuff(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
					CastRange: 3000, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 5, false, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeValidateLocation, "ground cast ValidateLocation")
	readCastStartFrames(t, c, objID, 5, 1, 500, 60_000, objID)
	icons := readStatusUpdateSkippingAbnormal(t, c, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	found := false
	for _, e := range icons {
		if e.SkillID == 5 && int32(e.Level) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ground-cast AbnormalStatusUpdate icons = %+v, want skill 5", icons)
	}
	drainUntilQuiet(t, c)
}

// TestGroundCastShiftOutOfRangeRejected verifies the shift-click variant
// refuses a ground point beyond cast range with the target-too-far message.
func TestGroundCastShiftOutOfRangeRejected(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
					CastRange: 900, HitTime: 500, StaticHitTime: true, SkillType: "SIGNET",
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestExMagicSkillUseGround(5000, 0, 0, 5, false, true))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageTargetTooFar)
	drainUntilQuiet(t, c)
}

// TestGroundCastIgnoresNonGroundAndUnknownSkill verifies the ground-cast
// packet stays silent for a non-ground skill and for an unlearned skill id,
// matching the reference's silent drops.
func TestGroundCastIgnoresNonGroundAndUnknownSkill(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 5, false, false))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("non-ground ground-cast reply = opcode %#x, want no reply", reply[0])
	}

	c.Send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 9999, false, false))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("unlearned ground-cast reply = opcode %#x, want no reply", reply[0])
	}
}

// TestToggleActivatesThenDeactivates reproduces recasting a toggle skill:
// the first request pays MP and applies the buff; the second finds the
// instance active and turns it off at no cost instead of refreshing it. Both
// directions answer with only the instantaneous MagicSkillUse ack plus the
// icon refresh — no message, cast bar, or launch report.
func TestToggleActivatesThenDeactivates(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
					MPConsume: 12, SkillType: "BUFF",
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 288, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(288, false, false))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillUse, "toggle activation")
	r := wireReader(reply[1:])
	caster, target, skillID, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != objID || target != objID || skillID != 288 || level != 1 {
		t.Fatalf("toggle MagicSkillUse ids = %d/%d/%d/%d, want %d/%d/288/1", caster, target, skillID, level, objID, objID)
	}
	if hitTime, reuse := r.ReadInt32(), r.ReadInt32(); hitTime != 0 || reuse != 0 {
		t.Fatalf("toggle MagicSkillUse timing = hit %d reuse %d, want 0/0", hitTime, reuse)
	}
	assertAbnormalStatusUpdate(t, c, 288, 1, 0)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(288, false, false))
	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillUse, "toggle deactivation")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasBeenAborted, 288, 1)
	if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 0 {
		t.Fatalf("AbnormalStatusUpdate entries after deactivation = %+v, want none", entries)
	}
	drainUntilQuiet(t, c)
}

// TestToggleCostFailureBroadcastsCastAbort verifies a toggle the caster
// cannot afford still broadcasts its instant ack, then the cost-failure
// message, the cast-cancel broadcast, and the pending-action release.
func TestToggleCostFailureBroadcastsCastAbort(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{ID: 289, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf, MPConsume: 100000},
			{ID: 290, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf, HPConsume: 100000},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 289, 1)
	seedKnownSkill(t, srv, objID, 290, 1)
	startInWorld(t, c)

	for _, test := range []struct {
		skillID int32
		message int32
	}{
		{289, serverpackets.SystemMessageNotEnoughMP},
		{290, serverpackets.SystemMessageNotEnoughHP},
	} {
		t.Run(skillCase(test.skillID), func(t *testing.T) {
			c.Send(encodeRequestMagicSkillUse(test.skillID, false, false))
			frame := c.Read()
			assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "toggle ack")
			frame = c.Read()
			assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "cost failure")
			if got := wireReader(frame[1:]).ReadInt32(); got != test.message {
				t.Fatalf("SystemMessage id = %d, want %d", got, test.message)
			}
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillCanceled, "cast cancel")
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "action release")
			drainUntilQuiet(t, c)
		})
	}
}

func skillCase(skillID int32) string {
	return fmt.Sprintf("skill-%d", skillID)
}
