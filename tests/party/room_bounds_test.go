package party

import (
	"strings"
	"testing"

	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// levelsTo80 is a level table whose highest attainable level is 80.
func levelsTo80(t *testing.T) gameservertest.Option {
	t.Helper()
	rows := make(map[int]playermodel.Level, 81)
	for lvl := 1; lvl <= 81; lvl++ {
		rows[lvl] = playermodel.Level{RequiredExpToLevelUp: 1000}
	}
	levels, err := playermodel.NewLevelTable(rows)
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return gameservertest.WithLevels(levels)
}

// TestPartyRoomTermsBounded pins the bounds on RequestManagePartyRoom
// terms, an owner-approved divergence (#5101): a title over 64 UTF-16
// units, a size outside 2..12, or a level range outside 1 to the highest
// attainable level (80 here) or upside down is refused with ActionFailed at Open and at Revise, and changes
// nothing; the refused leader stays on the waiting list and may open a
// room with legal terms. Rooms with the longest legal titles still reach
// every viewer in one room list.
func TestPartyRoomTermsBounded(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Other", 30}, {"Viewer", 40}}, spawnRegions(), levelsTo80(t))
	leader, other, viewer := g.players[0], g.players[1], g.players[2]

	longest := strings.Repeat("a", 64)
	// 62 units plus one character outside the Basic Multilingual Plane:
	// 64 UTF-16 units.
	longestWide := strings.Repeat("b", 62) + "\U0001F600"

	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	expectRoom(t, leader, "List")
	refused := []struct {
		name                         string
		maxMembers, minLevel, maxLvl int32
		title                        string
	}{
		{"title of 65 units", 12, 1, 80, longest + "a"},
		{"wide title of 65 units", 12, 1, 80, "a" + longestWide},
		{"size 0", 0, 1, 80, "Go"},
		{"size 1", 1, 1, 80, "Go"},
		{"size 13", 13, 1, 80, "Go"},
		{"huge size", 1 << 30, 1, 80, "Go"},
		{"negative size", -12, 1, 80, "Go"},
		{"min level 0", 12, 0, 80, "Go"},
		{"negative min level", 12, -5, 80, "Go"},
		{"max level 81", 12, 1, 81, "Go"},
		{"huge max level", 12, 1, 1 << 30, "Go"},
		{"min above max", 12, 40, 30, "Go"},
	}
	for _, c := range refused {
		t.Log("open with", c.name)
		leader.c.Send(encodeManagePartyRoom(0, c.maxMembers, c.minLevel, c.maxLvl, 0, c.title))
		expectRoom(t, leader, "ActionFailed")
		viewer.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
		if rooms := readRoomList(t, expectRoom(t, viewer, "List")[0]); len(rooms) != 0 {
			t.Fatalf("after %s: listed rooms = %+v", c.name, rooms)
		}
	}

	// The refused leader is still waiting and opens a room with legal terms.
	first := g.openRoom(t, leader, longest)
	for _, c := range refused {
		t.Log("revise with", c.name)
		leader.c.Send(encodeManagePartyRoom(first, c.maxMembers, c.minLevel, c.maxLvl, 0, c.title))
		expectRoom(t, leader, "ActionFailed")
	}
	second := g.openRoom(t, other, longestWide)

	viewer.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	rooms := readRoomList(t, expectRoom(t, viewer, "List")[0])
	want := []listedRoom{
		{id: first, title: longest, location: spawnBBS, minLevel: 1, maxLevel: 80, members: 1, maxMember: 12, leader: "Leader"},
		{id: second, title: longestWide, location: spawnBBS, minLevel: 1, maxLevel: 80, members: 1, maxMember: 12, leader: "Other"},
	}
	if len(rooms) != len(want) {
		t.Fatalf("listed rooms = %+v, want %+v", rooms, want)
	}
	for i := range want {
		if rooms[i] != want[i] {
			t.Fatalf("listed room %d = %+v, want %+v", i, rooms[i], want[i])
		}
	}
}
