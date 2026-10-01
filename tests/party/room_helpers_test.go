package party

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spawnBBS is the region number of the class template's spawn point
// (10, 20) under spawnRegions: map region (20, 18), as the restart table
// groups positions by 32768-unit tiles from tile (16, 10) at
// (-131072, -262144).
const spawnBBS = 7

func spawnRegions() gameservertest.Option {
	return gameservertest.WithRestartPoints(&restart.Table{Points: []restart.Point{
		{Name: "spawn", BBS: spawnBBS, MapRegions: []location.Point{{X: 20, Y: 18}}},
	}})
}

// Party-matching window values.
const (
	anyLocation int32 = -1
	allLevels   int32 = 1
)

func encodeListPartyWaiting(location, levelMode int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestListPartyWaiting)
	w.WriteInt32(0)
	w.WriteInt32(location)
	w.WriteInt32(levelMode)
	return w.Bytes()
}

func encodeManagePartyRoom(roomID, maxMembers, minLevel, maxLevel, loot int32, title string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestManagePartyRoom)
	w.WriteInt32(roomID)
	w.WriteInt32(maxMembers)
	w.WriteInt32(minLevel)
	w.WriteInt32(maxLevel)
	w.WriteInt32(loot)
	w.WriteString(title)
	return w.Bytes()
}

func encodeJoinPartyRoom(roomID, location, levelMode int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinPartyRoom)
	w.WriteInt32(roomID)
	w.WriteInt32(location)
	w.WriteInt32(levelMode)
	return w.Bytes()
}

func encodeExtendedInts(second uint16, vs ...int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(second)
	for _, v := range vs {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

// roomTokens names c's frames, in order, as the room tests compare them:
// party positions are left out; system messages carry their id, member
// lists their mode and member rows their mode and name.
func roomTokens(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodePartyMemberPosition:
		case serverpackets.OpcodePartyMatchList:
			out = append(out, "List")
		case serverpackets.OpcodePartyMatchDetail:
			out = append(out, "Detail")
		case serverpackets.OpcodeSystemMessage:
			out = append(out, fmt.Sprintf("SM%d", wire.NewReader(f[1:]).ReadInt32()))
		case serverpackets.OpcodeUserInfo:
			out = append(out, "UserInfo")
		case serverpackets.OpcodeCharInfo:
			out = append(out, "CharInfo")
		case serverpackets.OpcodeActionFailed:
			out = append(out, "ActionFailed")
		case serverpackets.OpcodeCreatureSay:
			out = append(out, "Say")
		case serverpackets.OpcodeExtended:
			r := wire.NewReader(f[1:])
			switch sub := r.ReadUint16(); sub {
			case serverpackets.OpcodeExPartyRoomMember:
				out = append(out, fmt.Sprintf("Members%d", r.ReadInt32()))
			case serverpackets.OpcodeExManagePartyRoomMember:
				mode := r.ReadInt32()
				r.ReadInt32()
				out = append(out, fmt.Sprintf("Manage%d:%s", mode, r.ReadString()))
			case serverpackets.OpcodeExClosePartyRoom:
				out = append(out, "Close")
			case serverpackets.OpcodeExAskJoinPartyRoom:
				out = append(out, "Ask:"+r.ReadString())
			case serverpackets.OpcodeExListPartyMatchingWaitingRoom:
				out = append(out, "Waiting")
			default:
				out = append(out, fmt.Sprintf("Ex%#x", sub))
			}
		default:
			out = append(out, fmt.Sprintf("%#x", f[0]))
		}
	}
	return out
}

// expectRoom drains c and compares its frames' tokens with want.
func expectRoom(t *testing.T, p player, want ...string) [][]byte {
	t.Helper()
	frames := drainFrames(t, p.c)
	if got := roomTokens(t, frames); !slices.Equal(got, want) {
		t.Fatalf("%s frames = %q, want %q", p.name, got, want)
	}
	var kept [][]byte
	for _, f := range frames {
		if f[0] != serverpackets.OpcodePartyMemberPosition {
			kept = append(kept, f)
		}
	}
	return kept
}

func sm(id int) string { return fmt.Sprintf("SM%d", id) }

// roomRow is one row of a room's member list.
type roomRow struct {
	id       int32
	name     string
	class    int32
	level    int32
	location int32
	status   int32
}

func readRoomRow(r *wire.Reader) roomRow {
	return roomRow{id: r.ReadInt32(), name: r.ReadString(), class: r.ReadInt32(), level: r.ReadInt32(), location: r.ReadInt32(), status: r.ReadInt32()}
}

// readRoomMembers decodes an ExPartyRoomMember.
func readRoomMembers(t *testing.T, frame []byte) (mode int32, rows []roomRow) {
	t.Helper()
	r := wire.NewReader(frame[3:])
	mode = r.ReadInt32()
	n := r.ReadInt32()
	for range n {
		rows = append(rows, readRoomRow(r))
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("ExPartyRoomMember: err %v, %d bytes left", err, r.Remaining())
	}
	return mode, rows
}

// readManageRow decodes an ExManagePartyRoomMember.
func readManageRow(t *testing.T, frame []byte) (int32, roomRow) {
	t.Helper()
	r := wire.NewReader(frame[3:])
	mode := r.ReadInt32()
	row := readRoomRow(r)
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("ExManagePartyRoomMember: err %v, %d bytes left", err, r.Remaining())
	}
	return mode, row
}

// roomTerms is a decoded PartyMatchDetail.
type roomTerms struct {
	id, maxMembers, minLevel, maxLevel, loot, location int32
	title                                              string
}

func readDetail(t *testing.T, frame []byte) roomTerms {
	t.Helper()
	r := wire.NewReader(frame[1:])
	d := roomTerms{id: r.ReadInt32(), maxMembers: r.ReadInt32(), minLevel: r.ReadInt32(), maxLevel: r.ReadInt32(), loot: r.ReadInt32(), location: r.ReadInt32(), title: r.ReadString()}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("PartyMatchDetail: err %v, %d bytes left", err, r.Remaining())
	}
	return d
}

// listedRoom is one row of a PartyMatchList.
type listedRoom struct {
	id                                               int32
	title                                            string
	location, minLevel, maxLevel, members, maxMember int32
	leader                                           string
}

func readRoomList(t *testing.T, frame []byte) []listedRoom {
	t.Helper()
	r := wire.NewReader(frame[1:])
	some, n := r.ReadInt32(), r.ReadInt32()
	if (some == 1) != (n > 0) || some > 1 {
		t.Fatalf("PartyMatchList flag %d with %d rooms", some, n)
	}
	var out []listedRoom
	for range n {
		out = append(out, listedRoom{
			id: r.ReadInt32(), title: r.ReadString(), location: r.ReadInt32(), minLevel: r.ReadInt32(),
			maxLevel: r.ReadInt32(), members: r.ReadInt32(), maxMember: r.ReadInt32(), leader: r.ReadString(),
		})
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("PartyMatchList: err %v, %d bytes left", err, r.Remaining())
	}
	return out
}

// openRoom has p open the window, then a room for every level, and drains
// every client. It returns the room's id.
func (g *group) openRoom(t *testing.T, p player, title string) int32 {
	t.Helper()
	p.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	p.c.Send(encodeManagePartyRoom(0, 12, 1, 80, 0, title))
	var id int32
	for _, f := range drainFrames(t, p.c) {
		if f[0] == serverpackets.OpcodePartyMatchDetail {
			id = readDetail(t, f).id
		}
	}
	if id == 0 {
		t.Fatalf("%s opened no room", p.name)
	}
	g.quiet(t)
	return id
}

// enterRoom has p open the window and enter room id, draining every
// client.
func (g *group) enterRoom(t *testing.T, p player, id int32) {
	t.Helper()
	p.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	p.c.Send(encodeJoinPartyRoom(id, anyLocation, allLevels))
	g.quiet(t)
}
