package party

import (
	"cmp"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPartyRoomListInHashTableOrder pins the room list and quick join to
// the order of the rooms' hash table, keyed by room id, not to the ids'
// order: with rooms 3 and 17 open in a 16-bucket table, room 17 (bucket
// 1) is listed before room 3 (bucket 3), and quick join enters it.
func TestPartyRoomListInHashTableOrder(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"First", 20}, {"Second", 20}, {"Third", 20}}, spawnRegions())
	first, second, third := g.players[0], g.players[1], g.players[2]
	dismiss := func(p player, id int32) {
		t.Helper()
		p.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestDismissPartyRoom, id, 0))
		g.quiet(t)
	}
	for range 2 {
		dismiss(first, g.openRoom(t, first, "Gone"))
	}
	low := g.openRoom(t, first, "Low")
	for range 13 {
		dismiss(second, g.openRoom(t, second, "Gone"))
	}
	high := g.openRoom(t, second, "High")
	if low != 3 || high != 17 {
		t.Fatalf("rooms %d and %d opened, want 3 and 17", low, high)
	}

	third.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	var listed []int32
	for _, f := range drainFrames(t, third.c) {
		if f[0] == serverpackets.OpcodePartyMatchList {
			for _, rm := range readRoomList(t, f) {
				listed = append(listed, rm.id)
			}
		}
	}
	if want := []int32{17, 3}; !slices.Equal(listed, want) {
		t.Fatalf("room list = %v, want %v", listed, want)
	}

	third.c.Send(encodeJoinPartyRoom(0, anyLocation, allLevels))
	var joined int32
	for _, f := range drainFrames(t, third.c) {
		if f[0] == serverpackets.OpcodePartyMatchDetail {
			joined = readDetail(t, f).id
		}
	}
	if joined != 17 {
		t.Fatalf("quick join entered room %d, want 17", joined)
	}
}

// TestPartyWaitingListInHashTableOrder pins the waiting list to the order
// of the waiting players' hash table, not to the order they joined it: a
// player hashes as 31 plus its object id, and its bucket in the table of
// 16 is that hash's low bits folded with its high half. The players join
// in the reverse of that order.
func TestPartyWaitingListInHashTableOrder(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Asker", 20}, {"Alpha", 20}, {"Beta", 20}, {"Gamma", 20}}, spawnRegions())
	asker := g.players[0]
	bucket := func(p player) uint32 {
		h := uint32(31 + p.id)
		return (h ^ h>>16) & 15
	}
	joins := slices.Clone(g.players[1:])
	slices.SortFunc(joins, func(a, b player) int { return cmp.Compare(bucket(b), bucket(a)) })
	want := slices.Clone(joins)
	slices.SortStableFunc(want, func(a, b player) int { return cmp.Compare(bucket(a), bucket(b)) })
	if slices.Equal(want, joins) {
		t.Fatalf("players %v share one bucket; the join order cannot differ from the table's", joins)
	}
	for _, p := range joins {
		p.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
		g.quiet(t)
	}

	asker.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestListPartyMatchingWaitingRoom, 1, 1, 80, 0))
	r := wire.NewReader(expectRoom(t, asker, "Waiting")[0][3:])
	if mode := r.ReadInt32(); mode != 0 {
		t.Fatalf("waiting list mode %d, want 0", mode)
	}
	var got []string
	for n := r.ReadInt32(); n > 0; n-- {
		got = append(got, r.ReadString())
		r.ReadInt32()
		r.ReadInt32()
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("ExListPartyMatchingWaitingRoom: err %v, %d bytes left", err, r.Remaining())
	}
	var names []string
	for _, p := range want {
		names = append(names, p.name)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("waiting list = %q, want %q", got, names)
	}
}
