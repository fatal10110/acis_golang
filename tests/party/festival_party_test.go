package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// signUp has the leader of f, at seat 0, pay its party's signup.
func (f *festivalRun) signUp(t *testing.T) {
	t.Helper()
	if got := festivalPage(t, f.command(t, 0, "2 6360")); got != "page 2e" {
		t.Fatalf("payment = %q, want page 2e", got)
	}
	f.quiet(t)
}

// participants reports, by seat, who the festival holds.
func (f *festivalRun) participants() []bool {
	out := make([]bool, len(f.players))
	for i, p := range f.players {
		out[i] = f.srv.Festival.IsParticipant(p.id)
	}
	return out
}

func sameBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A party's edits follow its signup, the festival taking parties of three:
//   - a member leaving a party of three leaves the festival alone: the two
//     who stay keep the run, even below the minimum;
//   - the next leave disperses the party, and the dispersal, with fewer
//     members than the minimum, drops the run: nobody is a participant;
//   - the dropped run still takes the festival: when the signup period
//     ends the festival begins, with no arena, nobody taken anywhere, and
//     the guides' call reaches the oracle.
func TestFestivalPartyEditsUpdateTheRun(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Ann", 20}, {"Bob", 20}}, 1000, 3)
	f.invite(t, 0, 1, 0)
	f.invite(t, 0, 2, 0)
	f.signUp(t)
	if got := f.participants(); !sameBools(got, []bool{true, true, true}) {
		t.Fatalf("participants after the signup = %v", got)
	}

	f.players[2].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	f.quiet(t)
	if got := f.participants(); !sameBools(got, []bool{true, true, false}) {
		t.Fatalf("participants after Bob left = %v, want Leader and Ann", got)
	}
	f.players[1].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	f.quiet(t)
	if got := f.participants(); !sameBools(got, []bool{false, false, false}) {
		t.Fatalf("participants after the party dispersed = %v, want none", got)
	}

	f.timers.fire(t, 2*time.Minute)
	f.timers.fire(t, 19*time.Minute)
	if !f.srv.Festival.Initialized() {
		t.Fatal("the festival did not begin for the dropped run")
	}
	frames := drainFrames(t, f.players[0].c)
	if _, ok := firstFrame(frames, serverpackets.OpcodeTeleportToLocation); ok {
		t.Fatal("the leader was taken into an arena")
	}
	if got := sayings(frames); !equalStrings(got, []string{"Festival Guide: The main event is now starting."}) {
		t.Fatalf("leader heard %q, want the guides' call", got)
	}
}

// A party leader leaving the festival through the guide takes the whole
// party out of it, answered only by the release every dialog command ends
// with; the party stays together.
func TestFestivalLeaderLeavesWithTheParty(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}}, 1000, 2)
	f.invite(t, 0, 1, 0)
	f.signUp(t)
	assertOpcodes(t, f.command(t, 0, "9"), []byte{serverpackets.OpcodeActionFailed}, "leave answer")
	if got := f.participants(); !sameBools(got, []bool{false, false}) {
		t.Fatalf("participants after the leader left = %v, want none", got)
	}
	f.quiet(t)
	if got := festivalPage(t, f.command(t, 0, "1")); got != "page 1 900/540/270" {
		t.Fatalf("signup after leaving = %q, want the fee page: the party holds", got)
	}
}

// Before the festival a challenge gets only the dialog's release. While it is under way, any
// party leader at a guide raises the
// challenge of the guide's arena, once (8b, then 8c), and a member who does
// not lead its party is refused (8a): the first wave comes again without
// its archers, each kill worth two blood offerings to the killer's party
// leader. A festival monster is out of reach of a player who is no
// participant: selecting one only releases the client.
func TestFestivalChallengeAndMonsterReach(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}, {"Cid", 20}, {"Dan", 20}}, 1000, 2)
	f.invite(t, 0, 1, 0)
	f.signUp(t)
	f.invite(t, 2, 3, 0)

	x, y, z := f.srv.PlayerPosition(t, f.players[2].id)
	stray := f.srv.SpawnHostileNPCKindAt(t, "FestivalMonster", location.Location{X: x + 40, Y: y, Z: z})
	f.quiet(t)
	f.players[2].c.Send(encodeAction(stray.ObjectID(), x, y, z))
	assertOpcodes(t, drainFrames(t, f.players[2].c), []byte{serverpackets.OpcodeActionFailed}, "non-participant selecting a festival monster")
	f.players[0].c.Send(encodeAction(stray.ObjectID(), x, y, z))
	if _, ok := firstFrame(drainFrames(t, f.players[0].c), serverpackets.OpcodeMyTargetSelected); !ok {
		t.Fatal("a participant could not select a festival monster")
	}
	f.quiet(t)

	assertOpcodes(t, f.command(t, 2, "8"), []byte{serverpackets.OpcodeActionFailed}, "challenge before the festival")
	f.timers.fire(t, 2*time.Minute)
	f.timers.fire(t, 19*time.Minute)
	for _, p := range f.players[:2] {
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.timers.fire(t, 2*time.Minute)
	f.quiet(t)
	first := festivalMonsters(f)

	if got := festivalPage(t, f.command(t, 3, "8")); got != "page 8a" {
		t.Fatalf("member's challenge = %q, want page 8a", got)
	}
	if got := festivalPage(t, f.command(t, 2, "8")); got != "page 8b" {
		t.Fatalf("leader's challenge = %q, want page 8b", got)
	}
	if got := festivalPage(t, f.command(t, 2, "8")); got != "page 8c" {
		t.Fatalf("second challenge = %q, want page 8c", got)
	}
	var doubled []*npc.Hostile
	for _, h := range festivalMonsters(f) {
		if h.OfferingBonus() == 2 {
			doubled = append(doubled, h)
		}
	}
	if len(doubled) == 0 || len(doubled) >= len(first) {
		t.Fatalf("challenge added %d monsters worth two over a first wave of %d, want the wave without its archers", len(doubled), len(first))
	}
	for _, h := range doubled {
		if id := h.NpcID(); id%10 == 4 || id%10 == 9 {
			t.Fatalf("challenge brought archer %d", id)
		}
	}
	f.quiet(t)
	if !doubled[0].TakeDamage(1000, f.combatant(t, 1)) {
		t.Fatal("the member's hit did not kill the monster")
	}
	requireMessage(t, "leader", drainFrames(t, f.players[0].c), serverpackets.SystemMessageYouPickedUpS2S1, int32(festival.BloodOfferingID), int32(2))
}

// festivalMonsters are the festival monsters in the world but the stray
// placed by hand (npc id 100).
func festivalMonsters(f *festivalRun) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range f.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.FestivalMonster() && h.NpcID() != 100 {
			out = append(out, h)
		}
	}
	return out
}
