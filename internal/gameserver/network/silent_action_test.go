package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestGameClientLinkNeverGoesSilentOnActionRequests is the guardrail against
// the bug class behind #828/#829/#873: an accepted client action packet that
// a handler quietly drops, leaving the client's pending action unresolved —
// which presented as a character that walks up to a target and freezes, a
// picked-up item that never leaves the ground, or an item-window click that
// does nothing. Every case here sends a request built to be rejected (a
// nonexistent object id, an unclaimed action id, a command with no target to
// act on) and asserts the exact rejection frames that come back, read up to a
// manor-list barrier so nothing is left in flight for the next case. A new
// rejection reason or message updates the case's want list.
//
// Scope limit: every case here is a *rejected* request, so this guardrail
// cannot catch a flow that answers rejections correctly but leaves the
// client's pending action outstanding when the action succeeds. Flows whose
// success path must also release the click (pickup, interact, follow — see
// docs/agents/action-response-contract.md) assert that release in their own
// success-path tests instead.
//
// RequestAcquireSkillInfo is deliberately absent: the reference returns
// without an alternative packet when no offer matches (#1638), and the
// opcode registers no pending client action, so its silence is documented
// reference parity rather than a silent drop.
//
// RequestSellItem is absent too: the reference answers every refusal with
// nothing (RequestSellItem.java:49-72: no merchant target or out of reach,
// another merchant's list id, a payout past the int32 cap), and the client
// closes its sell window when it sends the request, leaving no click
// pending. tests/npcs asserts that silence.
//
// RequestBuyItem and RequestPreviewItem answer most refusals with nothing
// (an unknown list, an untargeted or unreachable merchant, an item off the
// list, a count over the stock), as the reference does, and RequestItemList
// answers nothing while a shop window keeps the inventory disabled
// (RequestItemList.java:21-22). A shop window's submit and the inventory
// button register no pending client action, so that silence is reference
// parity; tests/npcs/merchant_test.go pins each silent branch. The
// try-on's empty request is answered and probed here.
//
// SendWarehouseDepositList, SendWarehouseWithdrawList and RequestPackageSend
// are absent too: the reference answers most of their refusals with nothing
// (no warehouse opened, no keeper selected or in reach, a karma player, a
// row naming an item not held), and the client closes its warehouse or
// package window when it sends the request, leaving no click pending.
// tests/npcs/warehouse_test.go pins those silences and every refusal that
// does answer.
//
// Logout at character select is absent for the same reason: with no character
// in the world the reference sends nothing and keeps the connection open
// (#2514), and the opcode registers no pending client action there.
// tests/character asserts that silence and the open connection.
//
// RequestRecipeBookDestroy, RequestRecipeItemMakeInfo and
// RequestRecipeItemMakeSelf naming an unknown recipe, and a craft of a recipe
// the book does not hold, are absent too: the reference drops each without an
// answer (an unknown recipe's craft window is a packet with no bytes), and
// none of them registers a pending client action — the recipe windows only
// ask again on the next click. tests/items asserts that silence.
//
// RequestHennaItemInfo, RequestHennaEquip and RequestHennaUnequipInfo naming
// an unknown symbol, and RequestHennaUnequip naming a symbol not worn, are
// absent for the same reason: the reference returns without an answer, and
// the symbol windows hold no pending action, asking again on the next click.
// tests/npcs asserts that silence.
//
// The private store and workshop window requests are absent as well
// (SetPrivateStoreListSell/Buy, RequestPrivateStoreBuy/Sell, the store
// titles, RequestPrivateStoreManage*/Quit*, RequestRecipeShop*): the
// reference answers their refusals with nothing, and each comes from a
// store window the client closes or keeps open itself, so no click waits on
// an answer. tests/trade asserts those refusals. The action-bar commands that
// open a store do answer a refusal, below.
//
// MultiSellChoose refused for its reuse window, its amount, list or entry,
// the NPC or the player's reach, or a non-stackable entry asked for more
// than once, is absent as well: the reference drops the open list without
// an answer, and the multisell window holds no pending action — it stays
// open and sends again only on the next click. tests/npcs asserts that
// silence.
//
// The augmentation window's requests are absent as well. RequestRefine and
// RequestRefineCancel answer every refusal with their extended result packet,
// which this barrier (itself an extended reply) cannot tell apart, so
// tests/items asserts those answers. The three confirmation steps and
// RequestConfirmCancelItem naming an item the player does not hold get
// nothing, as in the reference: the window registers no pending client
// action and only asks again on the next item dropped into it.
//
// Four friend and block branches are absent too, each silent in the
// reference: RequestAnswerFriendInvite with no answerable invitation (the
// dialog closed when the client answered), RequestBlock unblocking a name
// not on the block list or naming a type no command sends, and
// RequestSendL2FriendSay with an empty or over-long message. None registers
// a pending client action; tests/social asserts those silences.
//
// RequestDeleteMacro naming a macro the list lacks, and RequestEvaluate
// naming an online player other than the current selection, are absent as
// well: the reference returns without an answer, and neither the macro
// window nor the recommend command registers a pending client action.
// tests/character asserts both silences.
func TestGameClientLinkNeverGoesSilentOnActionRequests(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	self := chars.soleObjectID(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	const missingObjectID = 999999

	// Select the player itself: a second click on it is a follow of
	// oneself, refused.
	testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeActionOn(self, false))
		c.Send(encodeRequestManorList())
	}, serverpackets.OpcodeExtended)

	cases := []struct {
		name    string
		payload []byte
		want    []byte // reply opcodes, in wire order
	}{
		{"UseItem on an object the player doesn't hold", encodeUseItem(missingObjectID, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestUnEquipItem for an empty body slot", encodeRequestUnEquipItem(0), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestActionUse with an action id no handler claims", encodeRequestActionUse(9999, false, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestActionUse pet command with no active summon", encodeRequestActionUse(16, false, false), []byte{serverpackets.OpcodeActionFailed}},
		{"Action on the selected player itself (a follow of oneself)", encodeActionOn(self, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestBypassToServer for a command family not modeled yet", encodeRequestBypassToServer("bbs_default"), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestRecipeBookOpen on an empty book", encodeRequestRecipeBookOpen(1), []byte{serverpackets.OpcodeRecipeBookItemList}},
		{"RequestPreviewItem trying nothing on", encodeRequestPreviewItem(1), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestEvaluate naming no online player", encodeRequestEvaluate(missingObjectID), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestMakeMacro without a name", encodeRequestMakeMacro(""), []byte{serverpackets.OpcodeSystemMessage}},
		// The view resync answers with UserInfo, then each known object;
		// the player here knows nothing.
		{"RequestRecordInfo with nothing in view", wire.NewPacketWriter(clientpackets.OpcodeRequestRecordInfo).Bytes(), []byte{serverpackets.OpcodeUserInfo}},
		// The sell command puts the player in sell set-up; the buy command
		// that follows is refused without a message of its own.
		{"RequestActionUse private store sell", encodeRequestActionUse(10, false, false), []byte{serverpackets.OpcodePrivateStoreManageListSell}},
		{"RequestActionUse private store buy while setting up a sell store", encodeRequestActionUse(28, false, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestFriendInvite naming nobody online", encodeNamedRequest(clientpackets.OpcodeRequestFriendInvite, "Nobody"), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeFriendAddRequestResult}},
		{"RequestFriendDel naming no friend", encodeNamedRequest(clientpackets.OpcodeRequestFriendDel, "Nobody"), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestBlock naming no character", encodeRequestBlock(clientpackets.BlockAdd, "Nobody"), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestSendL2FriendSay to no friend", encodeRequestSendL2FriendSay("hi", "Nobody"), []byte{serverpackets.OpcodeSystemMessage}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frames := testsupport.SyncBarrierFrames(t, c, func() {
				c.Send(tc.payload)
				c.Send(encodeRequestManorList())
			}, serverpackets.OpcodeExtended)
			if len(frames) == 0 {
				t.Fatalf("%s: no reply at all — the request was silently dropped, leaving the client's action unresolved", tc.name)
			}
			got := make([]byte, len(frames))
			for i, f := range frames {
				got[i] = f[0]
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("%s: reply opcodes = %x, want %x", tc.name, got, tc.want)
			}
		})
	}
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

func encodeActionOn(objectID int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestRecipeBookOpen(typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRecipeBookOpen)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeRequestPreviewItem(listID int32, itemIDs ...int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPreviewItem)
	w.WriteInt32(0)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(itemIDs)))
	for _, id := range itemIDs {
		w.WriteInt32(id)
	}
	return w.Bytes()
}

func encodeNamedRequest(opcode byte, name string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(name)
	return w.Bytes()
}

func encodeRequestBlock(typ int32, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(typ)
	if typ == clientpackets.BlockAdd || typ == clientpackets.BlockRemove {
		w.WriteString(name)
	}
	return w.Bytes()
}

func encodeRequestSendL2FriendSay(message, recipient string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSendL2FriendSay)
	w.WriteString(message)
	w.WriteString(recipient)
	return w.Bytes()
}

func encodeRequestEvaluate(targetID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEvaluate)
	w.WriteInt32(targetID)
	return w.Bytes()
}

func encodeRequestMakeMacro(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMakeMacro)
	w.WriteInt32(0)
	w.WriteString(name)
	w.WriteString("")
	w.WriteString("")
	w.WriteUint8(0)
	w.WriteUint8(0)
	return w.Bytes()
}
