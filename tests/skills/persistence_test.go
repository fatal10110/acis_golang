package skills

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestLearnedSkillSurvivesRelogin learns a skill, logs out, and walks the
// second login: the character_skills row survives the session, the restored
// SkillList reports it, and no save row lingers.
func TestLearnedSkillSurvivesRelogin(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithCapturedLog())...)
	startInWorld(t, c)
	trainer := selectTrainer(t, srv, c, objID, 0)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 0)
	c.Read() // SP-decreased SystemMessage
	c.Read() // LearnedSkill SystemMessage
	c.Read() // SkillList
	c.Read() // NoMoreSkillsToLearn SystemMessage
	c.Read() // AcquireSkillDone
	c.Read() // ActionFailed
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
	// The trainer leaves, so the relogin burst holds no NpcInfo.
	srv.State.Despawn(trainer)
	drainUntilQuiet(t, c)

	logout(t, srv, c)

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
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
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
	assertCasterMPStatus(t, srv, c.Read(), objID, 28)
	readCastStartFrames(t, c, objID, 1204, 2, 500, 45_000, objID)
	readHitStatusThenIcons(t, srv, c, objID, 25)
	drainUntilQuiet(t, c)

	logout(t, srv, c)

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

	logout(t, srv, relogin)
	rows2 := 0
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_skills_save WHERE char_obj_id = ? AND skill_id = 1204 AND skill_level = 2", objID).Scan(&rows2); err != nil {
		t.Fatal(err)
	}
	if rows2 != 1 {
		t.Fatalf("character_skills_save rows after second logout = %d, want 1 (restored effect is live)", rows2)
	}
}

func TestStoreSkillCooltimeDisabledSkipsSaveAndRestore(t *testing.T) {
	t.Parallel()
	const skillID, level = 1204, 2
	defs := []modelskill.Definition{{
		ID: skillID, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
		MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
	}, {
		ID: 1205, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
	}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithStoreSkillCooltime(false),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, level)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, 1205, 1, 2, 0, 45_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertCasterMPStatus(t, srv, c.Read(), objID, 28)
	readCastStartFrames(t, c, objID, skillID, level, 500, 45_000, objID)
	readHitStatusThenIcons(t, srv, c, objID, 25)
	drainUntilQuiet(t, c)
	srv.TickAutosave(t)
	if count, _ := skillSaveRow(t, srv, objID, skillID, level); count != 0 {
		t.Fatalf("character_skills_save rows after disabled autosave = %d, want 0", count)
	}
	logout(t, srv, c)
	if count, _ := skillSaveRow(t, srv, objID, skillID, level); count != 0 {
		t.Fatalf("character_skills_save rows after disabled logout = %d, want 0", count)
	}
	if count, _ := skillSaveRow(t, srv, objID, 1205, 1); count != 1 {
		t.Fatalf("pre-seeded character_skills_save row after disabled save/restore = %d, want 1", count)
	}
}

// TestAutosaveRewritesSkillSaveAfterRestoreAndSurvivesCrashRelog consumes a
// restored character_skills_save row on login, then fires the production
// autosave sweep while the session is still attached. Restore already
// deleted the row, so the rewrite must come from the live effect list and
// reuse timers. Closing the stack without Logout simulates a process crash
// before detach; a fresh Boot must restore the buff from that autosave row.
func TestAutosaveRewritesSkillSaveAfterRestoreAndSurvivesCrashRelog(t *testing.T) {
	t.Parallel()
	defs := []modelskill.Definition{
		{
			ID: 1204, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1204, 2)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(1204, false, false))
	assertCasterMPStatus(t, srv, c.Read(), objID, 28)
	readCastStartFrames(t, c, objID, 1204, 2, 500, 45_000, objID)
	readHitStatusThenIcons(t, srv, c, objID, 25)
	drainUntilQuiet(t, c)

	logout(t, srv, c)

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
	if frame := frames[3]; frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("restored-buff frame opcode = %#x, want AbnormalStatusUpdate", frame[0])
	}
	drainUntilQuiet(t, relogin)

	count, _ := skillSaveRow(t, srv, objID, 1204, 2)
	if count != 0 {
		t.Fatalf("character_skills_save rows after restore = %d, want 0 (Restore consumed them)", count)
	}

	srv.TickAutosave(t)
	count, restoreType := skillSaveRow(t, srv, objID, 1204, 2)
	if count != 1 || restoreType != 0 {
		t.Fatalf("character_skills_save after autosave = count %d restore_type %d, want 1 effect row", count, restoreType)
	}

	srv.Close()

	srv2 := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	if got := srv2.SoleObjectID(t); got != objID {
		t.Fatalf("second boot character id = %d, want %d", got, objID)
	}
	crashRelog := srv2.Client
	crashRelog.Send(encodeRequestGameStart(0))
	if reply := crashRelog.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := crashRelog.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	crashRelog.Send(encodeEnterWorld())
	restored := readEnterWorldBurstWithRestoredBuff(t, crashRelog)
	if frame := restored[3]; frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("crash-relog restored-buff frame opcode = %#x, want AbnormalStatusUpdate", frame[0])
	}
	coolTimes := readSkillCoolTimeEntriesFromFrame(t, restored[len(restored)-2])
	if len(coolTimes) != 1 || coolTimes[0].SkillID != 1204 || coolTimes[0].Level != 2 ||
		coolTimes[0].RemainingSeconds <= 0 || coolTimes[0].RemainingSeconds > 45 {
		t.Fatalf("crash-relog SkillCoolTime = %+v, want one skill 1204 level 2 row with a positive remainder", coolTimes)
	}
}

// TestSelfOnlyEffectDoesNotRestoreOnRelogin casts a skill whose templates
// are self-targeted only. Logout still persists the live effect (and its
// reuse), but relogin restores reuse through SkillCoolTime without replaying
// the effect: EnterWorld has no AbnormalStatusUpdate, and a second logout
// writes a reuse-only row.
func TestSelfOnlyEffectDoesNotRestoreOnRelogin(t *testing.T) {
	t.Parallel()
	const skillID, level = 40, 1
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
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
	assertCasterMPStatus(t, srv, c.Read(), objID, 28)
	readCastStartFrames(t, c, objID, skillID, level, 500, 45_000, objID)
	icons := readHitStatusThenIcons(t, srv, c, objID, 25)
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

	logout(t, srv, c)
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

	logout(t, srv, relogin)
	count, restoreType = skillSaveRow(t, srv, objID, skillID, level)
	if count != 1 || restoreType != 1 {
		t.Fatalf("character_skills_save after second logout = count %d restore_type %d, want 1 reuse-only row", count, restoreType)
	}
}

// TestReloginWaitsForQueuedSkillSave drops a buffed character's connection
// while its persistence lane is backed up, so the detach's
// character_skills_save write is still queued, then selects the character
// again on a new connection. Selection must wait for that write before the
// character loads: the relogged character gets its buff and reuse back
// instead of restoring from rows the write has not landed yet.
func TestReloginWaitsForQueuedSkillSave(t *testing.T) {
	t.Parallel()
	const skillID, level = 1204, 2
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: skillID, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
				Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, level)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertCasterMPStatus(t, srv, c.Read(), objID, 28)
	readCastStartFrames(t, c, objID, skillID, level, 500, 45_000, objID)
	readHitStatusThenIcons(t, srv, c, objID, 25)
	drainUntilQuiet(t, c)

	release := srv.HoldPersistenceLane(t, objID)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	if count, _ := skillSaveRow(t, srv, objID, skillID, level); count != 0 {
		t.Fatalf("character_skills_save rows while the lane is held = %d, want 0 (the detach save is still queued)", count)
	}

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	// Selection is parked on the held lane.
	relogin.ExpectNoFrame()
	release()
	assertFrameOpcode(t, relogin.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, relogin.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	relogin.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, relogin)
	if frame := frames[3]; frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("restored-buff frame opcode = %#x, want AbnormalStatusUpdate", frame[0])
	}
	coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
	if len(coolTimes) != 1 || coolTimes[0].SkillID != skillID || coolTimes[0].Level != level ||
		coolTimes[0].RemainingSeconds <= 0 || coolTimes[0].RemainingSeconds > 45 {
		t.Fatalf("restored SkillCoolTime = %+v, want one skill %d level %d row with a positive remainder", coolTimes, skillID, level)
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

// logout sends the logout request, consumes its reply sequence (the
// LeaveWorld ack, then a closed connection with nothing in between) and waits
// until every save the detach queued has run, so a row read after it sees
// what logout wrote. srv must boot WithCapturedLog: a detach save the server
// logged as failed fails here with the store's error, instead of surfacing
// later as a row that is simply missing.
func logout(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeLogout())
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout reply opcode = %#x, want LeaveWorld", reply[0])
	}
	c.ExpectClosed()
	srv.FlushPersistence(t)
	for _, line := range strings.Split(srv.LogText(), "\n") {
		if strings.Contains(line, `"message":"save player `) {
			t.Fatalf("logout save failed: %s", line)
		}
	}
}

// TestStackedOutBuffPersistsAndRestoresBehindStronger pins that logout saves
// every held effect, not only the active one: with cancel-lesser off, the
// weaker same-stack buff stays held behind the stronger one, is written to
// character_skills_save ahead of it (buff_index follows the held-list
// order), and after relog is held again behind the restored stronger buff,
// with no stack-change system message sent for the replay.
func TestStackedOutBuffPersistsAndRestoresBehindStronger(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithCancelLesserEffect(false),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			stackedBuffDef(201, 1, 30), stackedBuffDef(202, 2, 30),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 202, 1)
	startInWorld(t, c)

	castSlotBuff(t, c, objID, 201)
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 202)); !slices.Equal(icons, []int32{202}) {
		t.Fatalf("icons after stronger stacked buff = %v, want only active [202]", icons)
	}

	logout(t, srv, c)

	rows, err := srv.DB.QueryContext(context.Background(),
		`SELECT skill_id, restore_type, buff_index FROM character_skills_save WHERE char_obj_id = ? ORDER BY buff_index`, objID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type saveRow struct{ skillID, restoreType, buffIndex int }
	var got []saveRow
	for rows.Next() {
		var r saveRow
		if err := rows.Scan(&r.skillID, &r.restoreType, &r.buffIndex); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []saveRow{{201, 0, 1}, {202, 0, 2}}
	if !slices.Equal(got, want) {
		t.Fatalf("character_skills_save rows = %+v, want held lesser saved first %+v", got, want)
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
	// The reference restores effects before the player has a client, so the
	// restore's stack messages (felt, disappeared) never reach it: only the
	// icon refresh does.
	feedback := drainEffectFeedback(t, relogin)
	for _, entry := range feedback {
		if strings.HasPrefix(entry, "sm:") {
			t.Fatalf("relog effect feedback = %v, want no skill system messages", feedback)
		}
	}
	if len(feedback) == 0 || feedback[len(feedback)-1] != "icons:[202]" {
		t.Fatalf("relog effect feedback = %v, want last icons:[202]", feedback)
	}
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{201, 202}) {
		t.Fatalf("held effects after relog = %v, want lesser held again [201 202]", ids)
	}
}
