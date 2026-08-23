package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
