package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// removalSteps names, in order, the frames of objectID's answer that tell
// its arena removal from its party notices: "teleport" for its own
// TeleportToLocation, "removed" for the removal text, "left", "expelled"
// and "dispersed" for those party messages, "windows" for
// PartySmallWindowDeleteAll.
func removalSteps(frames [][]byte, objectID int32) []string {
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeTeleportToLocation:
			if len(teleports([][]byte{f}, objectID)) == 1 {
				out = append(out, "teleport")
			}
		case serverpackets.OpcodePartySmallWindowDeleteAll:
			out = append(out, "windows")
		case serverpackets.OpcodeSystemMessage:
			m := systemMessages([][]byte{f})[0]
			switch {
			case m.id == serverpackets.SystemMessageS1 && len(m.params) == 1 && m.params[0] == removedText:
				out = append(out, "removed")
			case m.id == serverpackets.SystemMessageYouLeftParty:
				out = append(out, "left")
			case m.id == serverpackets.SystemMessageHaveBeenExpelledFromParty:
				out = append(out, "expelled")
			case m.id == serverpackets.SystemMessagePartyDispersed:
				out = append(out, "dispersed")
			}
		}
	}
	return out
}

// A participant whose own request takes it out of its party is taken out
// of the arena inside that request, before the party notices that follow:
//   - withdrawing from a party of four: teleport and removal, then "you
//     left the party" and the cleared window;
//   - leaving through the guide ("Festival 9"): taken back by the guide,
//     then told again by the expulsion (no town here to send it to), both
//     before "you have been expelled";
//   - withdrawing from a party of two, below the minimum: the dispersal
//     takes the whole run out as the leader's departure is followed, so
//     the leaver is out before its own window clears and the party is
//     said to disperse.
func TestFestivalLeaverIsRemovedBeforeThePartyNotices(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Ann", 20}, {"Bob", 20}, {"Cid", 20}}, 1000, 3)
	for i := 1; i < 4; i++ {
		f.invite(t, 0, i, 0)
	}
	f.signUp(t)
	f.begin(t, 0, 1, 2, 3)

	bob := f.players[2]
	bob.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	if got, want := removalSteps(drainFrames(t, bob.c), bob.id), []string{"teleport", "removed", "left", "windows"}; !slices.Equal(got, want) {
		t.Fatalf("Bob withdrawing = %v, want %v", got, want)
	}
	f.quiet(t)

	ann := f.players[1]
	x, y, z := f.srv.PlayerPosition(t, ann.id)
	inside := f.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("FestivalGuide", dawnGuideID), location.Location{X: x + 30, Y: y, Z: z})
	f.quiet(t)
	if got, want := removalSteps(f.commandAt(t, 1, inside, "9"), ann.id), []string{"teleport", "removed", "removed", "expelled", "windows"}; !slices.Equal(got, want) {
		t.Fatalf("Ann leaving through the guide = %v, want %v", got, want)
	}
	f.quiet(t)

	cid := f.players[3]
	cid.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	if got, want := removalSteps(drainFrames(t, cid.c), cid.id), []string{"teleport", "removed", "windows", "dispersed"}; !slices.Equal(got, want) {
		t.Fatalf("Cid withdrawing from a party of two = %v, want %v", got, want)
	}
}

// A leader expelling the last other member of its party disperses it: the
// leader, its own request's player, is taken out of the arena as its
// window clears, before it hears the party disperse.
func TestFestivalLeaderDispersingByExpulsionIsRemovedFirst(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}}, 1000, 2)
	f.invite(t, 0, 1, 0)
	f.signUp(t)
	f.begin(t, 0, 1)

	leader := f.players[0]
	leader.c.Send(encodeName(clientpackets.OpcodeRequestOustPartyMember, "Member"))
	if got, want := removalSteps(drainFrames(t, leader.c), leader.id), []string{"windows", "teleport", "removed", "dispersed"}; !slices.Equal(got, want) {
		t.Fatalf("leader expelling = %v, want %v", got, want)
	}
}
