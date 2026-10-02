package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Party-matching packet opcodes.
const (
	OpcodePartyMatchList   = 0x96
	OpcodePartyMatchDetail = 0x97

	OpcodeExPartyRoomMember              uint16 = 0x000e
	OpcodeExClosePartyRoom               uint16 = 0x000f
	OpcodeExManagePartyRoomMember        uint16 = 0x0010
	OpcodeExAskJoinPartyRoom             uint16 = 0x0034
	OpcodeExListPartyMatchingWaitingRoom uint16 = 0x0035
)

// Party-matching system message ids.
const (
	SystemMessagePartyRoomCreated               = 1388
	SystemMessagePartyRoomRevised               = 1389
	SystemMessagePartyRoomExited                = 1391
	SystemMessageS1LeftPartyRoom                = 1392
	SystemMessageOustedFromPartyRoom            = 1393
	SystemMessagePartyRoomDisbanded             = 1395
	SystemMessageCantViewPartyRooms             = 1396
	SystemMessagePartyRoomLeaderChanged         = 1397
	SystemMessageCantEnterPartyRoom             = 1413
	SystemMessageCannotDismissPartyMember       = 1699
	SystemMessagePartyMatchingRequestNoResponse = 1728
	SystemMessageS1EnteredPartyRoom             = 1900
)

// PartyRoom is one row of a party-matching room list.
type PartyRoom struct {
	ID         int32
	Title      string
	Location   int32
	MinLevel   int32
	MaxLevel   int32
	Members    int32
	MaxMembers int32
	LeaderName string
}

// FramePartyMatchList lists the rooms a player may browse.
func FramePartyMatchList(rooms []PartyRoom) wire.Frame {
	w := newFrameWriter(OpcodePartyMatchList)
	w.WriteInt32(boolInt32(len(rooms) > 0))
	w.WriteInt32(int32(len(rooms)))
	for _, r := range rooms {
		w.WriteInt32(r.ID)
		w.WriteString(r.Title)
		w.WriteInt32(r.Location)
		w.WriteInt32(r.MinLevel)
		w.WriteInt32(r.MaxLevel)
		w.WriteInt32(r.Members)
		w.WriteInt32(r.MaxMembers)
		w.WriteString(r.LeaderName)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// PartyRoomTerms are a room's terms, as its window shows them.
type PartyRoomTerms struct {
	ID         int32
	MaxMembers int32
	MinLevel   int32
	MaxLevel   int32
	Loot       int32
	Location   int32
	Title      string
}

// FramePartyMatchDetail shows a room's terms.
func FramePartyMatchDetail(t PartyRoomTerms) wire.Frame {
	w := newFrameWriter(OpcodePartyMatchDetail)
	w.WriteInt32(t.ID)
	w.WriteInt32(t.MaxMembers)
	w.WriteInt32(t.MinLevel)
	w.WriteInt32(t.MaxLevel)
	w.WriteInt32(t.Loot)
	w.WriteInt32(t.Location)
	w.WriteString(t.Title)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// PartyRoomMember is one row of a room's member list. Status is 1 for the
// room's leader, 2 for a member of the leader's party, 0 otherwise.
type PartyRoomMember struct {
	ObjectID int32
	Name     string
	ClassID  int32
	Level    int32
	Location int32
	Status   int32
}

func writePartyRoomMember(w *wire.Writer, m PartyRoomMember) {
	w.WriteInt32(m.ObjectID)
	w.WriteString(m.Name)
	w.WriteInt32(m.ClassID)
	w.WriteInt32(m.Level)
	w.WriteInt32(m.Location)
	w.WriteInt32(m.Status)
}

// FrameExPartyRoomMember shows a room's member list.
func FrameExPartyRoomMember(mode int32, members []PartyRoomMember) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPartyRoomMember)
	w.WriteInt32(mode)
	w.WriteInt32(int32(len(members)))
	for _, m := range members {
		writePartyRoomMember(w, m)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExManagePartyRoomMember adds, updates or removes one row of a
// room's member list.
func FrameExManagePartyRoomMember(mode int32, m PartyRoomMember) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExManagePartyRoomMember)
	w.WriteInt32(mode)
	writePartyRoomMember(w, m)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExClosePartyRoom closes the room window.
func FrameExClosePartyRoom() wire.Frame { return frameExtendedOnly(OpcodeExClosePartyRoom) }

// FrameExAskJoinPartyRoom asks the receiver to join the room of the player
// named requester.
func FrameExAskJoinPartyRoom(requester string) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExAskJoinPartyRoom)
	w.WriteString(requester)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// WaitingPlayer is one row of the party-matching waiting list.
type WaitingPlayer struct {
	Name    string
	ClassID int32
	Level   int32
}

// FrameExListPartyMatchingWaitingRoom lists the waiting players.
func FrameExListPartyMatchingWaitingRoom(mode int32, players []WaitingPlayer) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExListPartyMatchingWaitingRoom)
	w.WriteInt32(mode)
	w.WriteInt32(int32(len(players)))
	for _, p := range players {
		w.WriteString(p.Name)
		w.WriteInt32(p.ClassID)
		w.WriteInt32(p.Level)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
