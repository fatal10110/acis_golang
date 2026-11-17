package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanSkillWithoutLifeCrystalNeeded boots with LifeCrystalNeeded off: the
// skill info names no item requirement, and a leader carrying no item learns
// the skill for its reputation alone, with no item taken and no item message.
func TestClanSkillWithoutLifeCrystalNeeded(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.LifeCrystalNeeded = false
	opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: 1000}), gameservertest.WithClanConfig(cfg))
	w := bootClanWorldCarrying(t, 40, 0, nil, opts...)
	w.talkToMaster(t)

	w.leader.Send(encodeRequestAcquireSkillInfo(vitalityID, 1, acquireSkillReq))
	info := drainFrames(t, w.leader)
	if len(info) != 1 || info[0][0] != serverpackets.OpcodeAcquireSkillInfo {
		t.Fatalf("skill info = %x, want one AcquireSkillInfo", opcodes(info))
	}
	r := wire.NewReader(info[0][1:])
	got := []int32{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
	if want := []int32{vitalityID, 1, vitalityCost, acquireSkillReq, 0}; !slices.Equal(got, want) || r.Err() != nil {
		t.Fatalf("AcquireSkillInfo = %v (%v), want %v with no requirement", got, r.Err(), want)
	}

	w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
	want := []string{
		"PledgeShowInfoUpdate",
		"sm" + itoa(serverpackets.SystemMessageS1DeductedFromClanRep),
		"SkillList", "PledgeSkillListAdd",
		"sm" + itoa(serverpackets.SystemMessageClanSkillS1Added),
		"AcquireSkillList", "ActionFailed",
	}
	if view := clanSkillView(t, drainFrames(t, w.leader)); !slices.Equal(view, want) {
		t.Fatalf("leader's learn without the item = %v, want %v", view, want)
	}
	w.srv.FlushPersistence(t)
	if level := queryInt(t, w, `SELECT skill_level FROM clan_skills WHERE clan_id = ? AND skill_id = ?`, seededClanID, vitalityID); level != 1 {
		t.Fatalf("stored clan skill level = %d, want 1", level)
	}
	if _, rep := storedClan(t, w); rep != 500 {
		t.Fatalf("stored reputation = %d, want 500", rep)
	}
}
