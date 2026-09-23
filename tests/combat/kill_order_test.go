package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLethalHitOrdersStatusRewardDie pins what the killer's client sees when
// its skill kills a monster, in the reference's doDie order: the monster's
// zero-HP StatusUpdate (setHp(0)), then the killer's exp/SP reward
// (calculateRewards), then the monster's Die (AI DEAD). The death runs
// synchronously on the killer's queue, so the order holds on the worker pool
// as well as inline.
func TestLethalHitOrdersStatusRewardDie(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(levelTableFor(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := spawnRewardedNPC(t, srv, 5000, 25)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())

	var order []string
	for len(order) < 3 {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			t.Fatalf("kill sequence stopped after %v", order)
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate:
			if r.ReadInt32() == hostile.ObjectID() && len(order) == 0 {
				order = append(order, "status")
			}
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() == int32(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP) {
				order = append(order, "reward")
			}
		case serverpackets.OpcodeDie:
			if r.ReadInt32() == hostile.ObjectID() {
				order = append(order, "die")
			}
		}
	}
	if order[0] != "status" || order[1] != "reward" || order[2] != "die" {
		t.Fatalf("kill sequence = %v, want [status reward die]", order)
	}
}
