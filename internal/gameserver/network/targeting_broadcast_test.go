package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestGameClientLinkAttackBroadcastSendsToSelfAndObservers(t *testing.T) {
	state := world.New()
	link := &GameClientLink{world: state}
	attackerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	attackerFrames.frames = nil
	observerFrames.frames = nil

	link.broadcastAttack(attacker, attack.Snapshot{
		AttackerID: attacker.ObjectID(),
		X:          10,
		Y:          20,
		Z:          30,
		Hits:       []attack.SnapshotHit{{TargetID: observer.ObjectID(), Damage: 7}},
	})

	if len(attackerFrames.frames) != 1 || attackerFrames.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("attacker frames = %x, want one Attack", attackerFrames.frames)
	}
	if len(observerFrames.frames) != 1 || observerFrames.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("observer frames = %x, want one Attack", observerFrames.frames)
	}
}
