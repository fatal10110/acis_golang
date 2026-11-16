package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// replayBuffID is a ten-minute stacked self buff with an icon, saved on the
// base class before the scenario logs in.
const replayBuffID = 1204

// TestSubclassSwitchAnnouncesTheReplayedEffects pins #4913: a class change
// restores the class's saved effects on a player in the world through the
// normal add, so each icon effect that takes its stack group is announced
// with "You feel the effect of S1" ahead of the icon refresh. The login
// replay of the same effect stays silent.
func TestSubclassSwitchAnnouncesTheReplayedEffects(t *testing.T) {
	db := sqltest.SharedDB(t)
	defs := modelskill.NewTable([]modelskill.Definition{
		{ID: laterSkill, Level: 3},
		{ID: learnedSkill, Level: 1},
		{
			ID: replayBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 600, Icon: true, StackType: "SpeedUp", StackOrder: 1}},
		},
	})
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			objID, replayBuffID, 1, 1, 0, 0, 0, 0, 0, 1)
	},
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithReuseDelays(0, 0))

	// The subclass has no saved effect: the base class's buff leaves.
	if n := countSystemMessage(w.addSpellhowler(t), serverpackets.SystemMessageYouFeelS1Effect); n != 0 {
		t.Fatalf("felt messages on adding the subclass = %d, want 0", n)
	}

	// Back on the base class its buff comes back, announced.
	frames := w.changeTo(t, 0)
	felt := -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage || systemMessageID(f) != serverpackets.SystemMessageYouFeelS1Effect {
			continue
		}
		if felt >= 0 {
			t.Fatalf("change answer = %x, want one felt message", opcodes(frames))
		}
		felt = i
		r := wire.NewReader(f[1:])
		r.ReadInt32() // message id
		r.ReadInt32() // parameter count
		if kind, skill := r.ReadInt32(), r.ReadInt32(); kind != 4 || skill != replayBuffID {
			t.Fatalf("felt message parameter = type %d skill %d, want skill %d", kind, skill, replayBuffID)
		}
	}
	if felt < 0 {
		t.Fatalf("change answer = %x, want the felt message for the restored buff", opcodes(frames))
	}
	if icons := lastIndex(frames, serverpackets.OpcodeAbnormalStatusUpdate); icons < felt {
		t.Fatalf("change answer = %x, want an icon refresh after the felt message", opcodes(frames))
	}

	// The login replay of the same buff sends no felt message.
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	drainFrames(t, w.c)
	w.c.Send(encodeRequestGameStart(0))
	if reply := w.c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := w.c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	burst := enterWorld(t, w.srv, w.c)
	if firstIndex(burst, serverpackets.OpcodeAbnormalStatusUpdate) < 0 {
		t.Fatalf("EnterWorld = %x, want the restored buff's icons", opcodes(burst))
	}
	if n := countSystemMessage(burst, serverpackets.SystemMessageYouFeelS1Effect); n != 0 {
		t.Fatalf("felt messages at login = %d, want 0", n)
	}
}

func countSystemMessage(frames [][]byte, id int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(f) == id {
			n++
		}
	}
	return n
}
