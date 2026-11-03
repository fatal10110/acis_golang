package npc

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// TestDoorIDs pins the door list of each doorman's command: a plain
// doorman's list is split on commas and spaces and skips its first id; a
// castle doorman's starts one character after the command name and is
// split on ", ", dropping empty pieces at its end. A list that is too short,
// or an id that does not parse, is malformed after the ids before it.
func TestDoorIDs(t *testing.T) {
	for _, tc := range []struct {
		scope     DoorScope
		command   string
		open      bool
		ids       []int
		malformed bool
	}{
		{DoorsOfWorld, "open_doors 24170001", true, nil, false},
		{DoorsOfWorld, "open_doors 1 24170001, 24170002", true, []int{24170001, 24170002}, false},
		{DoorsOfWorld, "close_doors", false, nil, true},
		{DoorsOfWorld, "close_doors 1 5, x, 6", false, []int{5}, true},
		{DoorsOfResidence, "open_doors 19210001, 19210002", true, []int{19210001, 19210002}, false},
		{DoorsOfResidence, "close_doors 19210001, ", false, []int{19210001}, false},
		{DoorsOfResidence, "open_doors", true, nil, true},
		{DoorsOfResidence, "open_doors ", true, nil, true},
		{DoorsOfResidence, "open_doors 7,8", true, nil, true},
		{DoorsOfResidence, "open_doors 7, +8", true, []int{7, 8}, false},
	} {
		ids, malformed := BypassReply{DoorScope: tc.scope, OpenDoors: tc.open, doorCommand: tc.command}.DoorIDs()
		if !slices.Equal(ids, tc.ids) || malformed != tc.malformed {
			t.Errorf("%v %q = %v malformed %v, want %v malformed %v", tc.scope, tc.command, ids, malformed, tc.ids, tc.malformed)
		}
	}
}

// TestDoormanTeleportGate pins that a castle or clan hall doorman lets only
// the talker allowed to work its gates take its teleports, while a plain
// doorman lets everyone; a command that does not parse is released either
// way.
func TestDoormanTeleportGate(t *testing.T) {
	stranger := ResidenceTalk{CastleID: 1, OwnerID: 7, ClanID: 8}
	owner := ResidenceTalk{CastleID: 1, OwnerID: 7, ClanID: 7, Privileges: clan.PrivCastleEntryExit}
	for _, tc := range []struct {
		kind    string
		r       ResidenceTalk
		command string
		want    BypassOutcome
	}{
		{"CastleDoorman", stranger, "teleport 0", BypassRefused},
		{"CastleDoorman", stranger, "instant_teleport 0", BypassRefused},
		{"CastleDoorman", stranger, "teleport x", BypassReleased},
		{"CastleDoorman", owner, "teleport 0", BypassTeleport},
		{"ClanHallDoorman", ResidenceTalk{HallID: 22, OwnerID: 7, ClanID: 7}, "instant_teleport 0", BypassInstantTeleport},
		{"Doorman", ResidenceTalk{}, "teleport 0", BypassTeleport},
	} {
		r := tc.r
		f := digitsFolk(t, tc.kind, 35096)
		got := f.Bypass(noPages{}, ChatRules{}, Talker{Residence: func() ResidenceTalk { return r }}, tc.command)
		if got.Outcome != tc.want {
			t.Errorf("%s %q = %v, want %v", tc.kind, tc.command, got.Outcome, tc.want)
		}
	}
}

// TestClanGateAllowed pins the castle magician's gate check: a busy leader
// is never reached; from a Seven Signs dungeon, during seal validation only
// a leader of the winning cabal (a tie's no cabal included), otherwise a
// leader of any cabal.
func TestClanGateAllowed(t *testing.T) {
	for _, tc := range []struct {
		leader ClanGateLeader
		talker ClanGateTalker
		want   bool
	}{
		{ClanGateLeader{Busy: true, Cabal: sevensigns.Dawn}, ClanGateTalker{}, false},
		{ClanGateLeader{}, ClanGateTalker{}, true},
		{ClanGateLeader{}, ClanGateTalker{InDungeon: true}, false},
		{ClanGateLeader{Cabal: sevensigns.Dusk}, ClanGateTalker{InDungeon: true}, true},
		{ClanGateLeader{Cabal: sevensigns.Dusk}, ClanGateTalker{InDungeon: true, SealValidation: true, Winning: sevensigns.Dawn}, false},
		{ClanGateLeader{Cabal: sevensigns.Dawn}, ClanGateTalker{InDungeon: true, SealValidation: true, Winning: sevensigns.Dawn}, true},
		{ClanGateLeader{}, ClanGateTalker{InDungeon: true, SealValidation: true}, true},
	} {
		if got := ClanGateAllowed(tc.leader, tc.talker); got != tc.want {
			t.Errorf("ClanGateAllowed(%+v, %+v) = %v, want %v", tc.leader, tc.talker, got, tc.want)
		}
	}
}
