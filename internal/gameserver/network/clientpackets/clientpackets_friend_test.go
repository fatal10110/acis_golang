package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// The friend and block requests decode from hand-written payloads: the
// RequestBlock name is present only for a block or an unblock (types 0 and
// 1), and RequestSendL2FriendSay carries the message before the recipient.
func TestDecodeFriendRequests(t *testing.T) {
	bo := []byte{'B', 0x00, 'o', 0x00, 0x00, 0x00}

	invite, err := DecodeRequestFriendInvite(append([]byte{OpcodeRequestFriendInvite}, bo...))
	if err != nil || invite.Name != "Bo" {
		t.Fatalf("DecodeRequestFriendInvite = %+v, %v; want Bo", invite, err)
	}
	answer, err := DecodeRequestAnswerFriendInvite([]byte{OpcodeRequestAnswerFriendInvite, 0x01, 0x00, 0x00, 0x00})
	if err != nil || answer.Response != 1 {
		t.Fatalf("DecodeRequestAnswerFriendInvite = %+v, %v; want response 1", answer, err)
	}
	del, err := DecodeRequestFriendDel(append([]byte{OpcodeRequestFriendDel}, bo...))
	if err != nil || del.Name != "Bo" {
		t.Fatalf("DecodeRequestFriendDel = %+v, %v; want Bo", del, err)
	}
	block, err := DecodeRequestBlock(append([]byte{OpcodeRequestBlock, 0x01, 0x00, 0x00, 0x00}, bo...))
	if err != nil || block.Type != BlockRemove || block.Name != "Bo" {
		t.Fatalf("DecodeRequestBlock unblock = %+v, %v; want type 1 named Bo", block, err)
	}
	list, err := DecodeRequestBlock([]byte{OpcodeRequestBlock, 0x02, 0x00, 0x00, 0x00})
	if err != nil || list.Type != BlockList || list.Name != "" {
		t.Fatalf("DecodeRequestBlock list = %+v, %v; want type 2 with no name", list, err)
	}
	say, err := DecodeRequestSendL2FriendSay([]byte{OpcodeRequestSendL2FriendSay, 'h', 0x00, 'i', 0x00, 0x00, 0x00, 'B', 0x00, 'o', 0x00, 0x00, 0x00})
	if err != nil || say.Message != "hi" || say.Recipient != "Bo" {
		t.Fatalf("DecodeRequestSendL2FriendSay = %+v, %v; want hi to Bo", say, err)
	}
}

// A block without its name, or a friend message without its recipient, is
// a short packet.
func TestDecodeFriendRequestsShort(t *testing.T) {
	if _, err := DecodeRequestBlock([]byte{OpcodeRequestBlock, 0x00, 0x00, 0x00, 0x00}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeRequestBlock without a name: err = %v, want ErrShortPacket", err)
	}
	if _, err := DecodeRequestSendL2FriendSay([]byte{OpcodeRequestSendL2FriendSay, 'h', 0x00, 0x00, 0x00}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeRequestSendL2FriendSay without a recipient: err = %v, want ErrShortPacket", err)
	}
	if _, err := DecodeRequestAnswerFriendInvite([]byte{OpcodeRequestAnswerFriendInvite, 0x01}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeRequestAnswerFriendInvite short: err = %v, want ErrShortPacket", err)
	}
}
