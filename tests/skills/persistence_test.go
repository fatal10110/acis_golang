package skills

import (
	"context"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestLearnedSkillSurvivesRelogin learns a skill, logs out, and walks the
// second login: the character_skills row survives the session, the restored
// SkillList reports it, and no save row lingers.
func TestLearnedSkillSurvivesRelogin(t *testing.T) {
	srv, c, objID := bootLearner(t, generalLearnOpts(t, 50)...)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 0)
	c.Read() // SP-decreased SystemMessage
	c.Read() // LearnedSkill SystemMessage
	c.Read() // SkillList
	c.Read() // empty AcquireSkillList
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})

	logout(t, c)

	relogin := srv.DialClient(t, "player1", 1)
	frames := startInWorld(t, relogin)
	assertSkillList(t, frames[7], skillListEntry{passive: 0, level: 1, id: 3})
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
}

// TestLiveBuffAndReusePersistAtLogoutAndRestoreAtLogin casts a buff for
// real through the client protocol, logs out, verifies character_skills_save
// holds exactly the live-cast row, then logs back in: the restored effect
// replays during EnterWorld (its icon lands inside the entry burst) and the
// reuse timer comes back through SkillCoolTime.
func TestLiveBuffAndReusePersistAtLogoutAndRestoreAtLogin(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 1204, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
				Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1204, 2)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(1204, false, false))
	readCastStartFrames(t, c, objID, 1204, 2, 500, 45_000, objID)
	readStatusUpdateSkippingAbnormal(t, c, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	drainUntilQuiet(t, c)

	logout(t, c)

	var rows int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_skills_save WHERE char_obj_id = ? AND skill_id = 1204 AND skill_level = 2", objID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("character_skills_save rows for the live-cast buff = %d, want 1", rows)
	}

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	relogin.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, relogin)
	coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
	if len(coolTimes) != 1 || coolTimes[0].SkillID != 1204 || coolTimes[0].Level != 2 ||
		coolTimes[0].RemainingSeconds <= 0 || coolTimes[0].RemainingSeconds > 45 {
		t.Fatalf("restored SkillCoolTime = %+v, want one skill 1204 level 2 row with a positive remainder", coolTimes)
	}

	// The restored buff is live again: its replay during EnterWorld landed
	// the AbnormalStatusUpdate burst frame (index 3), and a second logout
	// saves it right back — an expired effect would persist nothing.
	if frame := frames[3]; frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("restored-buff frame opcode = %#x, want AbnormalStatusUpdate", frame[0])
	}
	drainUntilQuiet(t, relogin)

	logout(t, relogin)
	rows2 := 0
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_skills_save WHERE char_obj_id = ? AND skill_id = 1204 AND skill_level = 2", objID).Scan(&rows2); err != nil {
		t.Fatal(err)
	}
	if rows2 != 1 {
		t.Fatalf("character_skills_save rows after second logout = %d, want 1 (restored effect is live)", rows2)
	}
}

// TestSelfOnlyEffectDoesNotRestoreOnRelogin casts a skill whose templates
// are self-targeted only. Logout still persists the live effect (and its
// reuse), but relogin restores reuse through SkillCoolTime without replaying
// the effect: EnterWorld has no AbnormalStatusUpdate, and a second logout
// writes a reuse-only row.
func TestSelfOnlyEffectDoesNotRestoreOnRelogin(t *testing.T) {
	const skillID, level = 40, 1
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: skillID, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
				SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30, Self: true, Icon: true}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, level)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, level, 500, 45_000, objID)
	icons := readStatusUpdateSkippingAbnormal(t, c, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	found := false
	for _, e := range icons {
		if e.SkillID == skillID && int32(e.Level) == level {
			found = true
		}
	}
	if !found {
		t.Fatalf("AbnormalStatusUpdate icons after self-effect cast = %+v, want skill %d", icons, skillID)
	}
	drainUntilQuiet(t, c)

	logout(t, c)
	count, restoreType := skillSaveRow(t, srv, objID, skillID, level)
	if count != 1 || restoreType != 0 {
		t.Fatalf("character_skills_save after logout = count %d restore_type %d, want 1 effect row", count, restoreType)
	}

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	relogin.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, relogin)
	coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
	if len(coolTimes) != 1 || coolTimes[0].SkillID != skillID || coolTimes[0].Level != level ||
		coolTimes[0].RemainingSeconds <= 0 || coolTimes[0].RemainingSeconds > 45 {
		t.Fatalf("restored SkillCoolTime = %+v, want one skill %d level %d row with a positive remainder", coolTimes, skillID, level)
	}
	drainUntilQuiet(t, relogin)

	logout(t, relogin)
	count, restoreType = skillSaveRow(t, srv, objID, skillID, level)
	if count != 1 || restoreType != 1 {
		t.Fatalf("character_skills_save after second logout = count %d restore_type %d, want 1 reuse-only row", count, restoreType)
	}
}

func skillSaveRow(t *testing.T, srv *gameservertest.Server, objID, skillID, level int32) (count, restoreType int) {
	t.Helper()
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT COUNT(*), COALESCE(MAX(restore_type), 0) FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ? AND skill_level = ?`,
		objID, skillID, level).Scan(&count, &restoreType); err != nil {
		t.Fatal(err)
	}
	return count, restoreType
}

// logout sends the logout request and consumes its reply sequence: the
// LeaveWorld ack, the detach's unconditional cast-stop ActionFailed, and a
// closed connection.
func logout(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeLogout())
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout reply opcode = %#x, want LeaveWorld", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("post-logout opcode = %#x, want ActionFailed from detach's unconditional cast-stop ack", reply[0])
	}
	c.ExpectClosed()
}
