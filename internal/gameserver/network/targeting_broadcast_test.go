package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestGameClientLinkAttackBroadcastSendsToSelfAndObservers(t *testing.T) {
	state := world.New()
	link := &GameClientLink{world: state}
	attackerFrames := &testsupport.FrameCapture{}
	observerFrames := &testsupport.FrameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	testsupport.ResetCapture(attackerFrames)
	testsupport.ResetCapture(observerFrames)

	link.broadcastAttack(attacker, attack.Snapshot{
		AttackerID: attacker.ObjectID(),
		X:          10,
		Y:          20,
		Z:          30,
		Hits:       []attack.SnapshotHit{{TargetID: observer.ObjectID(), Damage: 7}},
	})

	if len(attackerFrames.Frames()) != 1 || attackerFrames.Frames()[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("attacker frames = %x, want one Attack", attackerFrames.Frames())
	}
	if len(observerFrames.Frames()) != 1 || observerFrames.Frames()[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("observer frames = %x, want one Attack", observerFrames.Frames())
	}
}
