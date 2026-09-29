package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// readCastStartFrames consumes the packets a successful active cast emits up
// to and including MagicSkillLaunched, asserting the ack's identity and
// timing fields along the way.
func readCastStartFrames(t *testing.T, c *scriptedClient, objID, skillID, level, hitTime, reuse, targetID int32) {
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

// assertCostStatus asserts frame is the caster's own StatusUpdate that a
// cast's MP payment sends (PlayerStatus.broadcastStatusUpdate: CUR_HP,
// CUR_MP, CUR_CP, MAX_CP), reporting mp as the MP left.
func assertCostStatus(t *testing.T, frame []byte, objID int32, mp int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "cost StatusUpdate")
	r := wireReader(frame[1:])
	if id := r.ReadInt32(); id != objID {
		t.Fatalf("cost StatusUpdate object = %d, want %d", id, objID)
	}
	if count := r.ReadInt32(); count != 4 {
		t.Fatalf("cost StatusUpdate count = %d, want 4", count)
	}
	for _, want := range []serverpackets.StatusType{serverpackets.StatusCurrentHP, serverpackets.StatusCurrentMP, serverpackets.StatusCurrentCP, serverpackets.StatusMaxCP} {
		typ, value := r.ReadInt32(), r.ReadInt32()
		if typ != int32(want) {
			t.Fatalf("cost StatusUpdate attribute = %d, want %d", typ, want)
		}
		if want == serverpackets.StatusCurrentMP && value != mp {
			t.Fatalf("cost StatusUpdate CUR_MP = %d, want %d", value, mp)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read cost StatusUpdate: %v", err)
	}
}

// castKillSkill casts killSkillDefs' skill 42 at targetID and reads its start
// through MagicSkillLaunched: the initial MP charge's status, then the cast
// start frames.
func castKillSkill(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objID, targetID int32, ctrl bool) {
	t.Helper()
	startMP := srv.PlayerCurrentMP(t, objID)
	c.Send(encodeRequestMagicSkillUse(42, ctrl, false))
	assertCostStatus(t, c.Read(), objID, int32(startMP-2))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, targetID)
}
