package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeFriendList is the wire opcode for FriendList.
const OpcodeFriendList = 0xfa

// FriendListEntry is one friend row shown in the client's friend list.
type FriendListEntry struct {
	ObjectID int32
	Name     string
	Online   bool
}

// FrameFriendList builds the friend list packet.
func FrameFriendList(friends []FriendListEntry) wire.Frame {
	w := newFrameWriter(OpcodeFriendList)
	w.WriteInt32(int32(len(friends)))
	for _, f := range friends {
		w.WriteInt32(f.ObjectID)
		w.WriteString(f.Name)
		w.WriteInt32(boolInt32(f.Online))
		if f.Online {
			w.WriteInt32(f.ObjectID)
		} else {
			w.WriteInt32(0)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
