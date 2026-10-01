package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMortalFolkDeathCancelsItsCast pins Creature.doDie's abortAll and
// Npc.doDie's AI removal for a civilian NPC: one that dies in the middle of
// a cast shows the cast canceled (MagicSkillCanceled) before it falls
// (Die), stops casting, and its AI casts nothing afterwards.
func TestMortalFolkDeathCancelsItsCast(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := gameservertest.FolkTemplate("Folk", 35588)
	tmpl.Undying = false
	folk := srv.SpawnCastingFolkNPCAt(t, tmpl, location.Location{X: x + 40, Y: y, Z: z}, modelskill.NewTable([]modelskill.Definition{{
		ID: folkLongBuffSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "BUFF", HitTime: 2000,
		StaticHitTime: true, StaticReuse: true,
	}}))
	settleAI(t, srv)
	drainUntilQuiet(t, c)

	addCastDesire(t, folk, modelskill.Ref{ID: folkLongBuffSkill, Level: 1})
	if frames := tickAI(t, srv); len(framesWith(frames, serverpackets.OpcodeMagicSkillUse, folk.ObjectID())) != 1 || !folk.CastingNow() {
		t.Fatalf("AI tick frames %x, casting %v; want the Folk casting", frameOpcodes(frames), folk.CastingNow())
	}

	if !folk.TakeDamage(1_000_000, nil) {
		t.Fatal("a lethal hit did not kill the mortal Folk")
	}
	frames := readUntilQuiet(t, c)
	canceled := frameIndex(frames, serverpackets.OpcodeMagicSkillCanceled, folk.ObjectID())
	die := frameIndex(frames, serverpackets.OpcodeDie, folk.ObjectID())
	if canceled < 0 || die < canceled {
		t.Fatalf("death frames %x: MagicSkillCanceled at %d, Die at %d; want the cancel then the death", frameOpcodes(frames), canceled, die)
	}
	if folk.CastingNow() {
		t.Fatal("dead Folk still casting")
	}
	if again := tickAI(t, srv); len(framesWith(again, serverpackets.OpcodeMagicSkillUse, folk.ObjectID())) != 0 {
		t.Fatalf("AI tick after the death frames %x, want no cast", frameOpcodes(again))
	}
}

// frameIndex returns the index of the first frame with opcode about id, -1
// when none is.
func frameIndex(frames [][]byte, opcode byte, id int32) int {
	for i, frame := range frames {
		if frame[0] == opcode && wireReader(frame[1:]).ReadInt32() == id {
			return i
		}
	}
	return -1
}
