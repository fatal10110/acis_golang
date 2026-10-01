package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Friend and block list packet opcodes.
const (
	OpcodeFriendAddRequestResult = 0x77
	OpcodeL2FriendStatus         = 0xfc
	OpcodeFriendAddRequest       = 0x7d
	OpcodeL2Friend               = 0xfb
	OpcodeL2FriendSay            = 0xfd
)

// L2Friend actions: the row is added to or removed from the client's friend
// list.
const (
	L2FriendAdd    int32 = 1
	L2FriendRemove int32 = 3
)

// FrameFriendAddRequestResult builds the invitation outcome sent to each
// side: accepted, or failed/refused.
func FrameFriendAddRequestResult(accepted bool) wire.Frame {
	w := newFrameWriter(OpcodeFriendAddRequestResult)
	w.WriteInt32(boolInt32(accepted))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameFriendAddRequest builds the invitation dialog shown to the invited
// player, naming who invites it.
func FrameFriendAddRequest(requesterName string) wire.Frame {
	w := newFrameWriter(OpcodeFriendAddRequest)
	w.WriteString(requesterName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameL2Friend builds one friend list row change. A row for a character
// not in the world carries object id 0 when its id is not known.
func FrameL2Friend(action int32, name string, online bool, objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeL2Friend)
	w.WriteInt32(action)
	w.WriteInt32(0)
	w.WriteString(name)
	w.WriteInt32(boolInt32(online))
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameL2FriendStatus builds the notice that a friend entered or left the
// world.
func FrameL2FriendStatus(online bool, name string, objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeL2FriendStatus)
	w.WriteInt32(boolInt32(online))
	w.WriteString(name)
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameL2FriendSay builds a friend's private message. failureMessage is 0
// for a delivered message, or the system message id explaining why the
// message was not delivered.
func FrameL2FriendSay(failureMessage int32, receiver, sender, message string) wire.Frame {
	w := newFrameWriter(OpcodeL2FriendSay)
	w.WriteInt32(failureMessage)
	w.WriteString(receiver)
	w.WriteString(sender)
	w.WriteString(message)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
