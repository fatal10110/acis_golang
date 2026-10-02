package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	triggeringSkill = 1010 // level 1 brings triggeredOne along, level 2 triggeredTwo
	triggeredOne    = 1011
	triggeredTwo    = 1012
	buffSkill       = 1020 // a self buff whose effect is put on the target
	slowCastSkill   = 1021 // a magic self buff whose hit time holds the cast open
)

// bootSkillRemoveAdmin boots a GM and a second character, Learner, at
// level 20 knowing, stored at level 1, triggeringSkill, triggeredOne,
// triggeredTwo, buffSkill and slowCastSkill, with the first three on
// shortcut slots 1 to 3. It returns the server and Learner's client and
// object id, both in the world, the GM's selection on Learner.
func bootSkillRemoveAdmin(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: triggeringSkill, Level: 1, Name: "Triggering", Activation: modelskill.ActivationActive, TriggeredID: triggeredOne, TriggeredLevel: 1},
		{ID: triggeringSkill, Level: 2, Name: "Triggering", Activation: modelskill.ActivationActive, TriggeredID: triggeredTwo, TriggeredLevel: 1},
		{ID: triggeredOne, Level: 1, Name: "TriggeredOne", Activation: modelskill.ActivationActive},
		{ID: triggeredTwo, Level: 1, Name: "TriggeredTwo", Activation: modelskill.ActivationActive},
		{
			ID: buffSkill, Level: 1, Name: "Buff", Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
		{
			ID: slowCastSkill, Level: 1, Name: "SlowCast", Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 5000, StaticReuse: true, Magic: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skills),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_skills.htm")),
	)
	enterWorld(t, srv.Client)
	learner := srv.SeedCharacterFor(t, "player2", "Learner", 20, 0)
	for _, id := range []int{triggeringSkill, triggeredOne, triggeredTwo, buffSkill, slowCastSkill} {
		if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0)", learner.ID, id); err != nil {
			t.Fatalf("seed learner skill %d: %v", id, err)
		}
	}
	for slot, id := range []int{triggeringSkill, triggeredOne, triggeredTwo} {
		if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?, ?, 0, 'SKILL', ?, 1, 0)", learner.ID, slot+1, id); err != nil {
			t.Fatalf("seed learner shortcut %d: %v", id, err)
		}
	}
	learnerClient := srv.DialClient(t, "player2", 1)
	enterWorld(t, learnerClient)
	drain(t, srv.Client)
	exchange(t, srv.Client, encodeAction(learner.ID))
	return srv, learnerClient, learner.ID
}

// onLearnerQueue runs fn on objID's queue and waits for it.
func onLearnerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func(*player.Character)) {
	t.Helper()
	pc := onlineCharacter(t, srv, objID)
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// knownLevels returns objID's live skill levels, read on its queue.
func knownLevels(t *testing.T, srv *gameservertest.Server, objID int32) map[int]int {
	t.Helper()
	var levels map[int]int
	onLearnerQueue(t, srv, objID, func(pc *player.Character) { levels = pc.SkillLevels() })
	return levels
}

// gmReply keeps, of frames, the GM's text reports and pages, dropping the
// target's broadcasts the GM sees as a bystander.
func gmReply(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage || f[0] == serverpackets.OpcodeNpcHtmlMessage {
			out = append(out, f)
		}
	}
	return out
}

// TestAdminSkillDropsTriggeredSkill pins the skill a level brings along
// (Player.addSkill and Player.removeSkill, "if (oldSkill.triggerAnotherSkill())
// removeSkill(oldSkill.getTriggeredId(), false)"): //skill set replacing a
// level drops the old level's triggered skill, and //skill remove drops the
// removed level's one, both from the live player only — the triggered
// skill's row and its shortcut stay, as store is false for it.
func TestAdminSkillDropsTriggeredSkill(t *testing.T) {
	t.Parallel()
	srv, learnerClient, learnerID := bootSkillRemoveAdmin(t)
	gm := srv.Client

	frames := exchange(t, gm, encodeBuildCmd("skill set 1010 2"))
	assertReportAndPage(t, "skill set 1010 2", frames, "You gave Triggering skill to Learner.", skillRow(triggeringSkill, "Triggering", 2))
	if page := htmlBody(t, frames[1]); strings.Contains(page, skillRow(triggeredOne, "TriggeredOne", 1)) || !strings.Contains(page, skillRow(triggeredTwo, "TriggeredTwo", 1)) {
		t.Fatalf("skill page after set = %q, want %d gone and %d kept", page, triggeredOne, triggeredTwo)
	}
	assertShortcutFrames(t, "set", settle(t, learnerClient), "R1:2", "L")
	if live := knownLevels(t, srv, learnerID); live[triggeredOne] != 0 || live[triggeringSkill] != 2 || live[triggeredTwo] != 1 {
		t.Fatalf("live skills after set = %v, want %d gone, %d at 2, %d kept", live, triggeredOne, triggeringSkill, triggeredTwo)
	}
	if stored := storedSkills(t, srv, learnerID); stored[triggeredOne] != 1 || stored[triggeringSkill] != 2 {
		t.Fatalf("stored skills after set = %v, want %d's row kept and %d at 2", stored, triggeredOne, triggeringSkill)
	}

	frames = exchange(t, gm, encodeBuildCmd("skill remove 1010"))
	if len(frames) != 2 {
		t.Fatalf("//skill remove 1010 frames = %x, want the report and the skill page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "You removed 1010 skillId from Learner.")
	if page := htmlBody(t, frames[1]); strings.Contains(page, "admin_skill remove 101") {
		t.Fatalf("skill page after remove = %q, want neither %d nor its triggered skills", page, triggeringSkill)
	}
	assertShortcutFrames(t, "remove", settle(t, learnerClient), "D1")
	if live := knownLevels(t, srv, learnerID); live[triggeringSkill] != 0 || live[triggeredTwo] != 0 {
		t.Fatalf("live skills after remove = %v, want %d and %d gone", live, triggeringSkill, triggeredTwo)
	}
	stored := storedSkills(t, srv, learnerID)
	if _, ok := stored[triggeringSkill]; ok || stored[triggeredOne] != 1 || stored[triggeredTwo] != 1 {
		t.Fatalf("stored skills after remove = %v, want %d's row deleted, the triggered skills' rows kept", stored, triggeringSkill)
	}
	if sc := storedShortcuts(t, srv, learnerID); len(sc) != 2 || sc[2] != 1 || sc[3] != 1 {
		t.Fatalf("stored shortcuts = %v, want the triggered skills' slots 2 and 3 kept", sc)
	}
}

// TestAdminSkillRemoveStopsEffect pins Player.removeSkill(id, true)
// stopping the removed skill's effects ("stopSkillEffects(skillId)"): the
// buff it put on the player ends and the player's icon list empties.
func TestAdminSkillRemoveStopsEffect(t *testing.T) {
	t.Parallel()
	srv, learnerClient, learnerID := bootSkillRemoveAdmin(t)
	gm := srv.Client

	onLearnerQueue(t, srv, learnerID, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: buffSkill, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "Buff", Time: 60, Icon: true})
		if err != nil {
			t.Errorf("effect.New(Buff): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	settle(t, learnerClient)
	drain(t, gm)

	assertReportAndPage(t, "skill remove 1020", gmReply(exchange(t, gm, encodeBuildCmd("skill remove 1020"))),
		"You removed 1020 skillId from Learner.", skillRow(triggeringSkill, "Triggering", 1))
	icons := -1
	for _, f := range settle(t, learnerClient) {
		if f[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			icons = int(wire.NewReader(f[1:]).ReadUint16())
		}
	}
	if icons != 0 {
		t.Fatalf("AbnormalStatusUpdate icons after remove = %d (-1: none sent), want an empty list", icons)
	}
	onLearnerQueue(t, srv, learnerID, func(pc *player.Character) {
		for _, e := range pc.EffectList().All() {
			if e.Skill.ID == buffSkill {
				t.Errorf("effect of %d still on the player after remove", buffSkill)
			}
		}
	})
}

// TestAdminSkillRemoveStopsCast pins Player.removeSkill stopping a cast
// of the removed skill in flight ("if (getCast().getCurrentSkill() != null
// && skillId == getCast().getCurrentSkill().getId()) getCast().stop()"):
// the cast is canceled at once and never lands.
func TestAdminSkillRemoveStopsCast(t *testing.T) {
	t.Parallel()
	srv, learnerClient, learnerID := bootSkillRemoveAdmin(t)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	gm := srv.Client

	learnerClient.Send(encodeMagicSkillUse(slowCastSkill))
	cast := settle(t, learnerClient)
	started := false
	for _, f := range cast {
		started = started || f[0] == serverpackets.OpcodeMagicSkillUse
	}
	if !started || !srv.PlayerCastingNow(t, learnerID) {
		t.Fatalf("cast frames %x, want a MagicSkillUse and a cast in flight", testsupport.FrameOpcodes(cast))
	}
	drain(t, gm)

	assertReportAndPage(t, "skill remove 1021", gmReply(exchange(t, gm, encodeBuildCmd("skill remove 1021"))),
		"You removed 1021 skillId from Learner.", skillRow(triggeringSkill, "Triggering", 1))
	canceled := false
	for _, f := range settle(t, learnerClient) {
		if f[0] == serverpackets.OpcodeMagicSkillCanceled {
			canceled = true
		}
	}
	if !canceled {
		t.Fatal("no MagicSkillCanceled after removing the skill being cast")
	}
	if srv.PlayerCastingNow(t, learnerID) {
		t.Fatal("a cast is in flight after the remove")
	}
	srv.Advance(t, 10*time.Second)
	for _, f := range settle(t, learnerClient) {
		if f[0] == serverpackets.OpcodeMagicSkillLaunched {
			t.Fatal("the removed skill's cast landed after the remove")
		}
	}
}

// encodeMagicSkillUse is a RequestMagicSkillUse of skillID, no ctrl or
// shift.
func encodeMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}
