package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: the battlefield teleport at login applies only on a
// battlefield under siege (EnterWorld.java:266-269 checks
// isInsideZone(ZoneId.SIEGE), which a siege zone sets only while its siege
// runs); a member leaving its clan, by withdrawal or expulsion, loses its
// siege state (Clan.removeClanMember, Clan.java:714).

// TestNoBattlefieldTeleportOutsideARunningSiege brings Third, in Knights
// registered to attack Gludio, and Loner, in no clan, in on the battlefield
// before the siege starts: neither is sent to town, and neither takes a
// siege state.
func TestNoBattlefieldTeleportOutsideARunningSiege(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, []castMember{loner, third}, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(thirdID),
		standOnField(lonerID), standOnField(thirdID),
	})
	for _, m := range []castMember{third, loner} {
		frames := startInWorld(t, w.srv.DialClient(t, m.account, 1))
		if i := firstIndexOf(frames, serverpackets.OpcodeTeleportToLocation); i >= 0 {
			t.Fatalf("%s logging in on the battlefield before the siege was sent to town: %x", m.name, opcodes(frames))
		}
		if got := w.srv.PlayerSiegeState(t, m.id); got != 0 {
			t.Fatalf("%s's siege state before the siege = %d, want 0", m.name, got)
		}
	}
}

// TestLeavingTheClanClearsTheSiegeState runs Gludio's siege with Knights
// attacking; Third and Squire, members of Knights, take the attacker state
// at login. Third withdraws and the founder expels Squire: each loses the
// siege state.
func TestLeavingTheClanClearsTheSiegeState(t *testing.T) {
	t.Parallel()
	inKnights := func(id int32) string {
		return `UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(id)
	}
	w := bootSiegeClanWorld(t, []castMember{third, allySquire}, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		inKnights(thirdID), inKnights(allySquireID),
	})
	startGludio(t, w)
	thirdClient := w.bringIn(t, third)
	squire := w.bringIn(t, allySquire)
	drainFrames(t, thirdClient)
	for _, m := range []castMember{third, allySquire} {
		if got := w.srv.PlayerSiegeState(t, m.id); got != 1 {
			t.Fatalf("%s's siege state at login = %d, want 1", m.name, got)
		}
	}

	thirdClient.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	expectMessages(t, "withdrawal during the siege", drainFrames(t, thirdClient),
		serverpackets.SystemMessageYouHaveWithdrawnFromClan, serverpackets.SystemMessageMustWaitBeforeJoiningAnotherClan)
	if got := w.srv.PlayerSiegeState(t, thirdID); got != 0 {
		t.Fatalf("siege state after withdrawing = %d, want 0", got)
	}

	drainFrames(t, w.leader)
	drainFrames(t, squire)
	w.leader.Send(encodeRequestOustPledgeMember(allySquire.name))
	drainFrames(t, w.leader)
	if id, _ := sysMsg(t, lastFrame(t, drainFrames(t, squire))); id != serverpackets.SystemMessageClanMembershipTerminated {
		t.Fatalf("expelled member's last message = %d, want CLAN_MEMBERSHIP_TERMINATED", id)
	}
	if got := w.srv.PlayerSiegeState(t, allySquireID); got != 0 {
		t.Fatalf("siege state after the expulsion = %d, want 0", got)
	}
}

// lastFrame is the last of frames, which must hold one.
func lastFrame(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	if len(frames) == 0 {
		t.Fatal("no frames")
	}
	return frames[len(frames)-1]
}
