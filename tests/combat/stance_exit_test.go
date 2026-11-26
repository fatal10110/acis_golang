package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestHitAfterExitLeavesNoAttackStance pins that a hit reaching a player
// whose exit already took it out of the world does not put it back in the
// stance tracker (the ATTACKED event is dropped for a creature no longer
// visible, AbstractAI.notifyEvent). Tracked again, the entry could never
// expire, its owner's queue being closed, and would refuse the next
// session's Logout as fighting.
func TestHitAfterExitLeavesNoAttackStance(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
		// The relog selects the character again inside CharacterSelectTime.
		gameservertest.WithReuseDelays(0, 0),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	departed, ok := livePlayer(t, srv, objID).(interface {
		NotifyAttacked(attackable.Combatant)
	})
	if !ok {
		t.Fatal("player cannot be notified of a hit")
	}
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(clientwire.SingleOpcode(clientpackets.OpcodeRequestRestart))
	if reply := mustRead(t, c, "RestartResponse"); reply[0] != serverpackets.OpcodeRestartResponse || reply[1] != 1 {
		t.Fatalf("restart reply = %x, want an accepting RestartResponse", reply)
	}
	departed.NotifyAttacked(attacker.Hostile)
	srv.Settle(t)
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("hit after the exit put the departed player back in the stance tracker")
	}

	drainUntilQuiet(t, c)
	c.Send(clientwire.RequestGameStart(0))
	if reply := mustRead(t, c, "SSQInfo"); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("relog select answered %#x, want SSQInfo", reply[0])
	}
	c.Send(clientwire.EnterWorld())
	drainUntilQuiet(t, c)
	c.Send(encodeLogout())
	if reply := mustRead(t, c, "Logout reply"); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("Logout of a relogged player that did not fight answered %#x, want LeaveWorld", reply[0])
	}
}
