package social

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// namesQuery is how the read of stored character names starts.
const namesQuery = "SELECT obj_Id, char_name"

// TestFriendListNamesHoldNoQueue holds the characters table while Alice
// asks for her friend list. While the names read waits, Alice's queue still
// runs work posted to it; once the lock lifts the list arrives whole, in
// order. Were the read made on the queue, the probe would wait behind it and
// the read would time out, leaving Bobby off the list.
func TestFriendListNamesHoldNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)

	release := p.srv.LockTable(t, "characters")
	p.alice.Send(encodeFriendList())
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.aliceID)
	release()

	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1Online, "Bobby")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)
	assertSilent(t, p.alice, "friend list after the footer")
}

// TestBlockListNamesHoldNoQueue is TestFriendListNamesHoldNoQueue for the
// block list.
func TestBlockListNamesHoldNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)
	p.srv.Relations.Block(p.aliceID, p.bobbyID)

	release := p.srv.LockTable(t, "characters")
	p.alice.Send(encodeBlock(clientpackets.BlockList, ""))
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.aliceID)
	release()

	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageBlockListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1, "1. Bobby")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)
	assertSilent(t, p.alice, "block list after the footer")
}

// TestFriendAcceptNamesLeftRequesterOffQueue holds the characters table
// while Bobby accepts the invitation of a login of Alice's that has left,
// whose name is read from the database. Bobby's queue runs meanwhile, and
// his three notices arrive in order once the lock lifts.
func TestFriendAcceptNamesLeftRequesterOffQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)
	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	p.restart(t, p.alice, p.aliceID)
	drainUntilQuiet(t, p.bobby)

	release := p.srv.LockTable(t, "characters")
	p.bobby.Send(encodeAnswerFriendInvite(1))
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.bobbyID)
	release()

	assertAddResult(t, p.bobby.Read(), true)
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1JoinedAsFriend, "Alice")
	assertL2Friend(t, p.bobby.Read(), serverpackets.L2FriendAdd, "Alice", false, p.aliceID)
	assertSilent(t, p.bobby, "accepting player after the add row")
	if !p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("the accepted invitation made no friends")
	}
}

// TestEnterWorldFriendNamesHoldNoQueue holds the characters table as Alice
// enters the world with Bobby on her friend list. The names read waits off
// every queue, so Bobby's still runs, and the burst then arrives in its
// order with Bobby named and online in the FriendList.
func TestEnterWorldFriendNamesHoldNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	startInWorld(t, p.bobby)
	drainUntilQuiet(t, p.bobby)
	p.alice.Send(clientwire.RequestGameStart(0))
	assertOpcode(t, p.alice.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertOpcode(t, p.alice.Read(), serverpackets.OpcodeCharSelected, "CharSelected")

	release := p.srv.LockTable(t, "characters")
	p.alice.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.bobbyID)
	release()

	burst := readEnterWorldBurst(t, p.alice)
	assertFriendList(t, burst[8], []serverpackets.FriendListEntry{{ObjectID: p.bobbyID, Name: "Bobby", Online: true}})
}
