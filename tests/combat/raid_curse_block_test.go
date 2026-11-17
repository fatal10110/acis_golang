package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRaidCurseBlocksTheSwingAndAnswersThePlayer pins what the player reads
// when the hit timer of its swing meets the raid curse: the curse animation
// (MagicSkillUse) and an ActionFailed answer, with no damage dealt and no
// further swing started.
func TestRaidCurseBlocksTheSwingAndAnswersThePlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{{
			ID: modelskill.RaidCurse2SkillID, Level: 1,
			Activation: modelskill.ActivationActive, Debuff: true, Offensive: true,
			SkillType: "PARALYZE", EffectRange: 2000,
			Effects: []modelskill.EffectTemplate{{
				Name: "Petrification", Count: 1, Time: 120, Icon: true,
				StackType: "turn_stone", StackOrder: 99, EffectPower: -1,
			}},
		}})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	onHostileQueue(t, hostile, func() { hostile.SetRaidRelated(true) })
	drainUntilQuiet(t, c)
	var hpBefore int
	onHostileQueue(t, hostile, func() { hpBefore = hostile.CurrentHP() })
	hitAt := time.Duration(playerAttackMillis(t, srv, objID)/2) * time.Millisecond

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntilOwnAttack(t, c, objID, "the swing")

	srv.Advance(t, hitAt-time.Millisecond)
	if got := tallySwing(t, srv, c, objID); got != (swingTally{}) {
		t.Fatalf("before the hit: %+v, want nothing", got)
	}
	srv.Advance(t, time.Millisecond)
	srv.Settle(t)
	frames := srv.ReadQueued(t, c)
	curse := indexOf(frames, 0, serverpackets.OpcodeMagicSkillUse, hostile.ObjectID())
	if curse < 0 {
		t.Fatal("the raid curse's MagicSkillUse never reached the player at the hit")
	}
	failed := 0
	for _, frame := range frames {
		switch {
		case attackFrameBy(frame, objID):
			t.Fatal("a new swing started after the curse blocked the attack")
		case frame[0] == serverpackets.OpcodeActionFailed:
			failed++
		case frame[0] == serverpackets.OpcodeSystemMessage &&
			wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageYouDidS1Dmg:
			t.Fatal("damage landed on the swing the curse blocked")
		}
	}
	// The petrification aborts the attack, cast and movement (an
	// ActionFailed each) before the swing's own stop answers; only the
	// presence of the answer is pinned, not the effect's internal count.
	if failed == 0 {
		t.Fatal("the blocked swing sent the player no ActionFailed")
	}
	var hpAfter int
	onHostileQueue(t, hostile, func() { hpAfter = hostile.CurrentHP() })
	if hpAfter != hpBefore {
		t.Fatalf("raid monster hp = %d, want %d: the blocked swing hurt it", hpAfter, hpBefore)
	}

	srv.Advance(t, 2*time.Second)
	if got := tallySwing(t, srv, c, objID); got.swings != 0 || got.damage != 0 {
		t.Fatalf("after the block: %+v, want no swing and no damage", got)
	}
}
