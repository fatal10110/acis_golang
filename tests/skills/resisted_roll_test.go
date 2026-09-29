package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func assertLevelOneResisted(t *testing.T, c *testsupport.ScriptedClient, skillID int32, targetName string) bool {
	t.Helper()
	return assertResistedAt(t, c, skillID, 1, targetName)
}

// assertResistedAt reads c up to S1_RESISTED_YOUR_S2 and asserts it names
// targetName and skill skillID at level. It reports false when the server
// goes quiet first.
func assertResistedAt(t *testing.T, c *testsupport.ScriptedClient, skillID, level int32, targetName string) bool {
	t.Helper()
	for range 50 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			return false
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != int32(serverpackets.SystemMessageS1ResistedYourS2) {
			continue
		}
		if r.ReadInt32() != 2 || r.ReadInt32() != serverpackets.SystemMessageParamText || r.ReadString() != targetName ||
			r.ReadInt32() != serverpackets.SystemMessageParamSkillName || r.ReadInt32() != skillID || r.ReadInt32() != level || r.Err() != nil {
			t.Fatalf("resisted message does not name %q and skill %d level %d", targetName, skillID, level)
		}
		return true
	}
	t.Fatal("too many frames before resisted message")
	return false
}

func TestStunFailedRollSendsLevelOneResistedOnWire(t *testing.T) {
	t.Parallel()
	const skillID = 44
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 7, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "STUN", Offensive: true, IgnoreResists: true, BaseLandRate: 0,
			Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 7)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 7, 500, 60_000, hostile.ObjectID())
	if !assertLevelOneResisted(t, c, skillID, hostile.CharacterName()) {
		t.Fatal("STUN failed roll sent no resisted message")
	}
}

func TestSpoilFailedRollSendsLevelOneResistedOnWire(t *testing.T) {
	t.Parallel()
	const skillID = 254
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Spoiler", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 7, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			SkillType: "SPOIL", Offensive: true, MagicLevel: 1, LevelDepend: -100,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 7)
	startInWorld(t, c)
	// The reference caps magic failure at 99%; use a fresh monster for each
	// attempt so a rare successful spoil cannot make later rolls ineligible.
	for range 5 {
		hostile := srv.SpawnHostileNPC(t)
		drainUntilQuiet(t, c)
		targetHostile(t, c, hostile.ObjectID())
		drainUntilQuiet(t, c)
		c.Send(encodeRequestMagicSkillUse(skillID, false, false))
		readCastStartFrames(t, c, objID, skillID, 7, 500, 0, hostile.ObjectID())
		if assertLevelOneResisted(t, c, skillID, hostile.CharacterName()) {
			return
		}
	}
	t.Fatal("SPOIL never failed in five casts")
}

// TestBetrayFailedRollSendsCastLevelResistedOnWire: a player's BETRAY whose
// landing roll fails tells it the monster resisted the skill, named at the
// cast level (issue #2728).
func TestBetrayFailedRollSendsCastLevelResistedOnWire(t *testing.T) {
	t.Parallel()
	const skillID = 1380
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 7, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "BETRAY", Offensive: true, IgnoreResists: true, BaseLandRate: 0,
			Effects: []modelskill.EffectTemplate{{Name: "Betray", Time: 10}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 7)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 7, 500, 60_000, hostile.ObjectID())
	if !assertResistedAt(t, c, skillID, 7, hostile.CharacterName()) {
		t.Fatal("BETRAY failed roll sent no resisted message")
	}
}

// TestChanceProcForgetFailedRollSendsResistedOnWire: an AGGREDUCE_CHAR
// (Forget) fired as a player's chance proc and failing its landing roll
// tells the player the monster resisted it, at the triggered level, through
// the chance proc's message delivery (issues #2728, #2777).
func TestChanceProcForgetFailedRollSendsResistedOnWire(t *testing.T) {
	t.Parallel()
	const (
		passive   = 3207
		triggered = 1156
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: passive, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
				SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: -1,
				TriggeredID: triggered, TriggeredLevel: 4,
			},
			{
				ID: triggered, Level: 4, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				SkillType: "AGGREDUCE_CHAR", Offensive: true, IgnoreResists: true, BaseLandRate: 0, ReuseDelay: 60_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, passive, 1)
	startInWorld(t, c)
	setPlayerRoll(t, srv, objID, 0)
	px, py, pz := srv.PlayerPosition(t, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, c)

	startMelee(t, srv, objID, hostile.ObjectID())
	hp := hostile.CurrentHP()
	srv.AdvanceUntil(t, "a landed melee hit", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
	if !assertResistedAt(t, c, triggered, 4, hostile.CharacterName()) {
		t.Fatal("chance-proc AGGREDUCE_CHAR failed roll sent no resisted message")
	}
}
