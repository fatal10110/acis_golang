package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const folkLongCastSkill = modelskill.ID(9203)

// bootCastingFolk brings a player in beside a civilian NPC and has the NPC
// start a self-cast whose hit time keeps it in flight through the test on
// any clock. Neither timing is static, so the cast itself is timed by the
// NPC's speed while an observer is shown the skill's own values.
func bootCastingFolk(t *testing.T) (*gameservertest.Server, int32, *npc.Folk) {
	t.Helper()
	srv, objID, folk := bootFolkCaster(t, []modelskill.Definition{{
		ID: folkLongCastSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "DUMMY", HitTime: 20_000, ReuseDelay: 45_000,
	}})
	addCastDesire(t, folk, modelskill.Ref{ID: folkLongCastSkill, Level: 1})
	if frames := tickAI(t, srv); len(framesWith(frames, serverpackets.OpcodeMagicSkillUse, folk.ObjectID())) != 1 {
		t.Fatalf("AI tick frames %x, want the Folk's MagicSkillUse", frameOpcodes(frames))
	}
	if !folk.CastingNow() {
		t.Fatal("Folk not casting after its MagicSkillUse")
	}
	return srv, objID, folk
}

// TestEnteringPlayerSeesFolkCastInFlight pins CreatureCast.describeCastTo
// for a civilian NPC: a player coming to know a casting Folk gets its
// NpcInfo, then right after it the cast's MagicSkillUse from the Folk where
// it stands to its target, carrying the skill's own hit time and reuse
// delay, laid out as for a player or monster caster.
func TestEnteringPlayerSeesFolkCastInFlight(t *testing.T) {
	t.Parallel()
	srv, _, folk := bootCastingFolk(t)

	srv.SeedCharacterFor(t, "player2", "Arriving", 5, 0)
	observer := srv.DialClient(t, "player2", 1)
	observer.Send(encodeRequestGameStart(0))
	for frame := observer.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = observer.Read() {
	}
	observer.Send(encodeEnterWorld())
	var frames [][]byte
	for frame := observer.ReadWithTimeout(500 * time.Millisecond); frame != nil; frame = observer.ReadWithTimeout(500 * time.Millisecond) {
		frames = append(frames, frame)
	}

	assertFolkCastDescribed(t, frames, folk)
}

// TestRecordInfoResendsKnownObjectsWithState pins RequestRecordInfo
// (Player.refreshInfos): the player gets its UserInfo first, then every
// object it knows as it was first shown — a casting Folk's NpcInfo followed
// by its MagicSkillUse, a walking monster's NpcInfo followed by its
// MoveToLocation — and nothing else.
func TestRecordInfoResendsKnownObjectsWithState(t *testing.T) {
	t.Parallel()
	srv, objID, folk := bootCastingFolk(t)
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, objID)
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: x - 80, Y: y, Z: z})
	walkTo := location.Location{X: x - 80, Y: y + 1500, Z: z}
	var walkErr error
	onNPCQueue(t, monster, func() { _, walkErr = monster.Move().MoveToLocation(walkTo) })
	if walkErr != nil {
		t.Fatalf("start monster walk: %v", walkErr)
	}
	drainUntilQuiet(t, c)

	frames := requestRecordInfo(t, c)
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("RequestRecordInfo frames %x, want UserInfo first", frameOpcodes(frames))
	}
	if n := len(framesWithOpcode(frames, serverpackets.OpcodeUserInfo)); n != 1 {
		t.Fatalf("RequestRecordInfo frames %x, want one UserInfo, got %d", frameOpcodes(frames), n)
	}
	assertFolkCastDescribed(t, frames, folk)

	info := frameIndexOf(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID())
	if info < 0 || info+1 >= len(frames) {
		t.Fatalf("RequestRecordInfo frames %x, want the monster's NpcInfo then its MoveToLocation", frameOpcodes(frames))
	}
	move := frames[info+1]
	assertFrameOpcode(t, move, serverpackets.OpcodeMoveToLocation, "frame after the monster's NpcInfo")
	r := wireReader(move[1:])
	id := r.ReadInt32()
	dest := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if id != monster.ObjectID() || dest != walkTo {
		t.Fatalf("MoveToLocation object %d to %v, want %d to %v", id, dest, monster.ObjectID(), walkTo)
	}

	// Two NPCs are known, so exactly two info packets and two state frames
	// follow the UserInfo.
	if len(frames) != 5 {
		t.Fatalf("RequestRecordInfo frames %x, want UserInfo then two NpcInfo/state pairs", frameOpcodes(frames))
	}
}

// requestRecordInfo sends RequestRecordInfo and returns every frame it
// answers with.
func requestRecordInfo(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRecordInfo))
	return readUntilQuiet(t, c)
}

// assertFolkCastDescribed fails unless frames hold folk's NpcInfo followed
// at once by the MagicSkillUse of its in-flight self-cast.
func assertFolkCastDescribed(t *testing.T, frames [][]byte, folk *npc.Folk) {
	t.Helper()
	info := frameIndexOf(frames, serverpackets.OpcodeNPCInfo, folk.ObjectID())
	if info < 0 || info+1 >= len(frames) {
		t.Fatalf("frames %x, want the Folk's NpcInfo then its MagicSkillUse", frameOpcodes(frames))
	}
	next := frames[info+1]
	assertFrameOpcode(t, next, serverpackets.OpcodeMagicSkillUse, "frame after the Folk's NpcInfo")
	r := wireReader(next[1:])
	gotCaster, gotTarget, gotSkill, gotLevel := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	gotHit, gotReuse := r.ReadInt32(), r.ReadInt32()
	cx, cy, cz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	crit := r.ReadInt32()
	tx, ty, tz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	folkID := folk.ObjectID()
	if gotCaster != folkID || gotTarget != folkID || gotSkill != int32(folkLongCastSkill) || gotLevel != 1 {
		t.Fatalf("MagicSkillUse ids = caster %d target %d skill %d level %d, want %d/%d/%d/1",
			gotCaster, gotTarget, gotSkill, gotLevel, folkID, folkID, folkLongCastSkill)
	}
	if gotHit != 20_000 || gotReuse != 45_000 {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want the skill's own 20000/45000", gotHit, gotReuse)
	}
	x, y, z := folk.Position()
	if int(cx) != x || int(cy) != y || int(cz) != z || tx != cx || ty != cy || tz != cz {
		t.Fatalf("MagicSkillUse caster at (%d,%d,%d) target at (%d,%d,%d), want both at the Folk's (%d,%d,%d)",
			cx, cy, cz, tx, ty, tz, x, y, z)
	}
	if crit != 0 {
		t.Fatalf("MagicSkillUse critical flag = %d, want 0", crit)
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("MagicSkillUse layout: err %v, %d trailing bytes", err, r.Remaining())
	}
}

// frameIndexOf returns the index of the first frame with opcode whose
// leading int32 is objectID, -1 when there is none.
func frameIndexOf(frames [][]byte, opcode byte, objectID int32) int {
	for i, frame := range frames {
		if frame[0] == opcode && len(frame) >= 5 && wireReader(frame[1:]).ReadInt32() == objectID {
			return i
		}
	}
	return -1
}
