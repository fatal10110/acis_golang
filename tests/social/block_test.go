package social

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBlockAndUnblock pins /block and /unblock by name. A block answers the
// blocker with S1_WAS_ADDED_TO_YOUR_IGNORE_LIST naming the stored name and,
// when the blocked player is online, tells it S1_HAS_ADDED_YOU_TO_IGNORE_LIST
// naming the blocker; blocking again says so again. /blocklist lists the
// blocked names numbered between BLOCK_LIST_HEADER and the friend list
// footer. An unblock answers S1_WAS_REMOVED_FROM_YOUR_IGNORE_LIST, and an
// unblock of a name not blocked answers nothing. The block is saved with
// the relations, flagged by which side of the pair blocks.
func TestBlockAndUnblock(t *testing.T) {
	p := bootPair(t)
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	p.enterAll(t)

	for range 2 {
		p.alice.Send(encodeBlock(clientpackets.BlockAdd, "bobby"))
		assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, "Bobby")
		assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Alice")
	}
	p.alice.Send(encodeBlock(clientpackets.BlockAdd, "CAROL"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, "Carol")
	if !p.srv.Relations.IsBlocked(p.aliceID, p.bobbyID) || p.srv.Relations.IsBlocked(p.bobbyID, p.aliceID) {
		t.Fatal("the block is not Alice's alone")
	}

	p.alice.Send(encodeBlock(clientpackets.BlockList, ""))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageBlockListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1, "1. Bobby")
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1, "2. Carol")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)

	p.srv.SaveRelations(t)
	wantFlag := 2 // the lower id blocks the higher
	if p.aliceID > p.bobbyID {
		wantFlag = 4
	}
	if got := relationRow(t, p, p.aliceID, p.bobbyID); got != wantFlag {
		t.Fatalf("character_relations flags = %d, want %d", got, wantFlag)
	}

	p.alice.Send(encodeBlock(clientpackets.BlockRemove, "Bobby"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1RemovedFromYourIgnoreList, "Bobby")
	assertSilent(t, p.bobby, "unblocked player")
	p.alice.Send(encodeBlock(clientpackets.BlockRemove, "Bobby"))
	assertSilent(t, p.alice, "unblocking a name not blocked")

	p.srv.SaveRelations(t)
	if got := relationRow(t, p, p.aliceID, p.bobbyID); got != -1 {
		t.Fatalf("character_relations row after the unblock = %d, want none", got)
	}
	if got := relationRow(t, p, p.aliceID, carolID); got == -1 {
		t.Fatal("Carol's block was not saved")
	}
}

// TestBlockRefusals pins the block and unblock refusals: no character of
// that name, or the player's own, answers
// FAILED_TO_REGISTER_TO_IGNORE_LIST; a character with an access level
// answers YOU_MAY_NOT_IMPOSE_A_BLOCK_ON_GM. A block type no command sends
// answers nothing.
func TestBlockRefusals(t *testing.T) {
	p := bootPair(t)
	gmID := p.srv.SeedCharacterFor(t, "player3", "Gamemaster", 1, 0).ID
	p.setAccessLevel(t, gmID, 1)
	p.enterAll(t)

	for _, tc := range []struct {
		typ  int32
		name string
		want int
	}{
		{clientpackets.BlockAdd, "Nobody", serverpackets.SystemMessageFailedToRegisterToIgnoreList},
		{clientpackets.BlockAdd, "ALICE", serverpackets.SystemMessageFailedToRegisterToIgnoreList},
		{clientpackets.BlockRemove, "Nobody", serverpackets.SystemMessageFailedToRegisterToIgnoreList},
		{clientpackets.BlockAdd, "gamemaster", serverpackets.SystemMessageYouMayNotImposeBlockOnGM},
		{clientpackets.BlockRemove, "Gamemaster", serverpackets.SystemMessageYouMayNotImposeBlockOnGM},
	} {
		p.alice.Send(encodeBlock(tc.typ, tc.name))
		assertStaticSystemMessage(t, p.alice.Read(), tc.want)
		assertSilent(t, p.alice, "after a refused block of "+tc.name)
	}

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(9)
	p.alice.Send(w.Bytes())
	assertSilent(t, p.alice, "unknown block type")
}

// TestBlockEverything pins /allblock and /allunblock: each answers its
// system message, then an EtcStatusUpdate whose third field carries the
// mode. While it is on, friend invitations to the player are refused.
func TestBlockEverything(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
	assertStaticSystemMessage(t, p.bobby.Read(), serverpackets.SystemMessageBlockingAll)
	assertEtcBlocked(t, p.bobby.Read(), true)
	assertSilent(t, p.bobby, "after /allblock")

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1BlockedEverything, "Bobby")
	assertAddResult(t, p.alice.Read(), false)

	p.bobby.Send(encodeBlock(clientpackets.BlockAllRelease, ""))
	assertStaticSystemMessage(t, p.bobby.Read(), serverpackets.SystemMessageNotBlockingAll)
	assertEtcBlocked(t, p.bobby.Read(), false)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest after /allunblock")
}

// TestFriendMessage pins RequestSendL2FriendSay. A message to an online
// friend reaches it as L2FriendSay(0) naming the recipient as typed and the
// sender; the sender hears nothing. A friend that ignores the sender sends
// the message back to the sender as undelivered, L2FriendSay carrying
// S1_HAS_ADDED_YOU_TO_IGNORE_LIST2. A recipient who is offline or no friend
// answers TARGET_IS_NOT_FOUND_IN_THE_GAME. An empty message, or one past
// 300 characters, answers nothing.
func TestFriendMessage(t *testing.T) {
	p := bootPair(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	p.alice.Send(encodeFriendSay("hello", "bobby"))
	assertFriendSay(t, p.bobby.Read(), 0, "bobby", "Alice", "hello")
	assertSilent(t, p.alice, "sender after a delivered message")

	long := strings.Repeat("a", 300)
	p.alice.Send(encodeFriendSay(long, "Bobby"))
	assertFriendSay(t, p.bobby.Read(), 0, "Bobby", "Alice", long)

	for _, msg := range []string{"", long + "a"} {
		p.alice.Send(encodeFriendSay(msg, "Bobby"))
		assertSilent(t, p.alice, "sender of a dropped message")
		assertSilent(t, p.bobby, "recipient of a dropped message")
	}

	for _, name := range []string{"Carol", "Nobody"} {
		p.alice.Send(encodeFriendSay("hello", name))
		assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
	}
	assertSilent(t, carol, "a non-friend named as recipient")

	p.srv.Relations.Block(p.bobbyID, p.aliceID)
	p.alice.Send(encodeFriendSay("hello", "bobby"))
	assertFriendSay(t, p.alice.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList2, "Alice", "bobby", "hello")
	assertSilent(t, p.bobby, "ignoring friend")
}

func assertEtcBlocked(t *testing.T, frame []byte, blocked bool) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeEtcStatusUpdate, "EtcStatusUpdate")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // charges
	r.ReadInt32() // weight penalty
	want := int32(0)
	if blocked {
		want = 1
	}
	if got := r.ReadInt32(); got != want {
		t.Fatalf("EtcStatusUpdate blocked = %d, want %d", got, want)
	}
}

func assertFriendSay(t *testing.T, frame []byte, failure int32, receiver, sender, message string) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeL2FriendSay, "L2FriendSay")
	r := wire.NewReader(frame[1:])
	gotFailure, gotReceiver, gotSender, gotMessage := r.ReadInt32(), r.ReadString(), r.ReadString(), r.ReadString()
	if gotFailure != failure || gotReceiver != receiver || gotSender != sender || gotMessage != message {
		t.Fatalf("L2FriendSay = (%d, %q, %q, %q), want (%d, %q, %q, %q)", gotFailure, gotReceiver, gotSender, gotMessage, failure, receiver, sender, message)
	}
}
