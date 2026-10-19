package party

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const removedText = "You have been removed from the festival arena."

// teleports are where the TeleportToLocation frames among frames send
// objectID.
func teleports(frames [][]byte, objectID int32) []location.Location {
	var out []location.Location
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		out = append(out, location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())})
	}
	return out
}

// count is how many of xs are x.
func count(xs []string, x string) int {
	n := 0
	for _, s := range xs {
		if s == x {
			n++
		}
	}
	return n
}

// origins are where each seat stands.
func (f *festivalRun) origins(t *testing.T) []location.Location {
	t.Helper()
	out := make([]location.Location, len(f.players))
	for i, p := range f.players {
		x, y, z := f.srv.PlayerPosition(t, p.id)
		out[i] = location.Location{X: x, Y: y, Z: z}
	}
	return out
}

// begin fires the first cycle and its signup period, then has the seats
// taken into an arena land there.
func (f *festivalRun) begin(t *testing.T, seats ...int) {
	t.Helper()
	f.timers.fire(t, 2*time.Minute)
	f.timers.fire(t, 19*time.Minute)
	for _, i := range seats {
		f.players[i].c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)
	if !f.srv.Festival.Initialized() {
		t.Fatal("the festival did not begin")
	}
}

// Party edits during the festival, with parties of three and more:
//   - a member withdrawing from a party of four is taken back where it came
//     from, the three left keeping the run;
//   - a member leaving through the guide is taken back, then expelled from
//     its party, whose edit takes it out again, now to the nearest town:
//     twice removed;
//   - the next withdrawal disperses the party of two, below the minimum:
//     the leader, taken out by the dispersal, is taken out again with the
//     whole party as the run drops, and the last member once. Nobody is a
//     participant any more; the festival goes on.
func TestFestivalPartyEditsDuringTheFestival(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Ann", 20}, {"Bob", 20}, {"Cid", 20}}, 1000, 3)
	for i := 1; i < 4; i++ {
		f.invite(t, 0, i, 0)
	}
	f.signUp(t)
	from := f.origins(t)
	f.begin(t, 0, 1, 2, 3)

	bob := f.players[2]
	bob.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	frames := drainFrames(t, bob.c)
	if got := teleports(frames, bob.id); len(got) != 1 || !near(got[0].X, got[0].Y, from[2].X, from[2].Y, 20) {
		t.Fatalf("Bob teleported to %v, want once back near %v", got, from[2])
	}
	if got := texts(frames); count(got, removedText) != 1 {
		t.Fatalf("Bob told %q, want removed once", got)
	}
	if got := f.participants(); !sameBools(got, []bool{true, true, false, true}) {
		t.Fatalf("participants after Bob left = %v", got)
	}
	f.quiet(t)

	ann := f.players[1]
	x, y, z := f.srv.PlayerPosition(t, ann.id)
	inside := f.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("FestivalGuide", dawnGuideID), location.Location{X: x + 30, Y: y, Z: z})
	f.quiet(t)
	frames = f.commandAt(t, 1, inside, "9")
	requireMessage(t, "Ann", frames, serverpackets.SystemMessageHaveBeenExpelledFromParty)
	if got := texts(frames); count(got, removedText) != 2 {
		t.Fatalf("Ann told %q, want removed twice", got)
	}
	if got := teleports(frames, ann.id); len(got) == 0 || !near(got[0].X, got[0].Y, from[1].X, from[1].Y, 20) {
		t.Fatalf("Ann teleported to %v, want first back near %v", got, from[1])
	}
	if got := f.participants(); !sameBools(got, []bool{true, false, false, true}) {
		t.Fatalf("participants after Ann left = %v", got)
	}
	f.quiet(t)

	cid := f.players[3]
	cid.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	leaderFrames, cidFrames := drainFrames(t, f.players[0].c), drainFrames(t, cid.c)
	if got := texts(leaderFrames); count(got, removedText) != 2 {
		t.Fatalf("leader told %q, want removed twice", got)
	}
	if got := teleports(leaderFrames, f.players[0].id); len(got) == 0 || !near(got[0].X, got[0].Y, from[0].X, from[0].Y, 20) {
		t.Fatalf("leader teleported to %v, want first back near %v", got, from[0])
	}
	if got := texts(cidFrames); count(got, removedText) != 1 {
		t.Fatalf("Cid told %q, want removed once", got)
	}
	if got := teleports(cidFrames, cid.id); len(got) != 1 || !near(got[0].X, got[0].Y, from[3].X, from[3].Y, 20) {
		t.Fatalf("Cid teleported to %v, want once back near %v", got, from[3])
	}
	if got := f.participants(); !sameBools(got, []bool{false, false, false, false}) {
		t.Fatalf("participants after the dispersal = %v, want none", got)
	}
	if !f.srv.Festival.Initialized() {
		t.Fatal("the festival stopped with the dropped run")
	}
}

// A participant whose connection drops during the festival leaves its
// party as it leaves the world, which takes it out of the arena: it is
// saved back where it came from, and the two left keep the run.
func TestFestivalParticipantDroppedIsSavedAtItsOrigin(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Ann", 20}, {"Bob", 20}}, 1000, 2)
	f.invite(t, 0, 1, 0)
	f.invite(t, 0, 2, 0)
	f.signUp(t)
	from := f.origins(t)
	f.begin(t, 0, 1, 2)

	bob := f.players[2]
	if err := bob.c.Close(); err != nil {
		t.Fatal(err)
	}
	f.srv.AdvanceUntil(t, "Bob out of the world", func() bool {
		_, ok := f.srv.State.Player(bob.id)
		return !ok
	})
	f.srv.FlushPersistence(t)
	var at location.Location
	if err := f.srv.DB.QueryRowContext(context.Background(), "SELECT x, y, z FROM characters WHERE obj_Id = ?", bob.id).Scan(&at.X, &at.Y, &at.Z); err != nil {
		t.Fatal(err)
	}
	if !near(at.X, at.Y, from[2].X, from[2].Y, 20) {
		t.Fatalf("Bob saved at %v, want near %v", at, from[2])
	}
	if !f.srv.Festival.IsParticipant(f.players[0].id) || !f.srv.Festival.IsParticipant(f.players[1].id) {
		t.Fatal("the two left lost the run")
	}
}

// The guide's refusals, the festival taking parties of two:
//   - a party whose highest member is above the festival's level is
//     refused (2d), on the fee page and on a crafted payment alike;
//   - an unknown stone, or a stone the leader holds too few of, is not
//     enough items: no signup, no bonus;
//   - a signed-up leader asking again signs its party up again (2f);
//   - during the festival nobody signs up, and while it is under way
//     nobody registers a score; raising the challenge of an arena that is
//     not open answers nothing at all;
//   - after it, a member of the party cannot register (3b), and the leader
//     beating no record (Dusk's 100 is ahead of Dawn's 0) gets 3d, its
//     contribution still said to rise, the record and Dawn's festival score
//     unchanged.
func TestFestivalGuideRefusals(t *testing.T) {
	t.Parallel()
	f := bootFestivalWith(t, festivalSetup{
		seats: []seat{{"Leader", 20}, {"Member", 20}, {"Cid", 20}, {"Dan", 40}}, blueStones: 1000, minPlayers: 2,
		scores: []festival.Score{{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1, Date: 1, Score: 100, Members: "Old"}},
	})
	f.invite(t, 2, 3, 0)
	for _, cmd := range []string{"1", "2 6360"} {
		if got := festivalPage(t, f.command(t, 2, cmd)); got != "page 2d" {
			t.Fatalf("Festival %s for a level-40 party = %q, want page 2d", cmd, got)
		}
	}
	f.quiet(t)
	f.invite(t, 0, 1, 0)
	for _, cmd := range []string{"2 57", "2 6361"} {
		answer := f.command(t, 0, cmd)
		requireMessage(t, "Festival "+cmd, answer, serverpackets.SystemMessageNotEnoughItems)
		if _, ok := firstFrame(answer, serverpackets.OpcodeNpcHtmlMessage); ok {
			t.Fatalf("Festival %s opened a page", cmd)
		}
	}
	if f.srv.Festival.IsParticipant(f.players[0].id) {
		t.Fatal("an unpaid signup went through")
	}
	if err := f.srv.Festival.SaveStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	var bonus int
	if err := f.srv.DB.QueryRow(`SELECT accumulated_bonus0 FROM seven_signs_status WHERE id = 0`).Scan(&bonus); err != nil || bonus != 0 {
		t.Fatalf("bonus after unpaid signups = %d (%v), want 0", bonus, err)
	}
	f.signUp(t)
	if got := festivalPage(t, f.command(t, 0, "1")); got != "page 2f" {
		t.Fatalf("signed-up leader's signup = %q, want page 2f", got)
	}

	f.begin(t, 0, 1)
	if got := texts(f.command(t, 2, "1")); !equalStrings(got, []string{"You cannot sign up while a festival is in progress."}) {
		t.Fatalf("signup during the festival told %q", got)
	}
	f.timers.fire(t, 2*time.Minute)
	f.quiet(t)
	if got := texts(f.command(t, 2, "3")); !equalStrings(got, []string{"You cannot register a score while a festival is in progress."}) {
		t.Fatalf("registration during the festival told %q", got)
	}
	x, y, z := f.srv.PlayerPosition(t, f.players[2].id)
	closed := f.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("FestivalGuide", dawnGuideID+1), location.Location{X: x - 30, Y: y, Z: z})
	f.quiet(t)
	if frames := f.commandAt(t, 2, closed, "8"); len(frames) != 0 {
		t.Fatalf("challenge of an arena not open = %x, want nothing", opcodes(frames))
	}

	var monster *npc.Hostile
	for _, h := range festivalMonsters(f) {
		monster = h
		break
	}
	if monster == nil || !monster.TakeDamage(1000, f.combatant(t, 1)) {
		t.Fatal("no festival monster killed")
	}
	for _, d := range []time.Duration{3 * time.Minute, 4 * time.Minute, 3 * time.Minute, 3 * time.Minute, 3 * time.Minute} {
		f.timers.fire(t, d)
	}
	for _, p := range f.players[:2] {
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)

	if got := festivalPage(t, f.command(t, 1, "3")); got != "page 3b" {
		t.Fatalf("member's registration = %q, want page 3b", got)
	}
	answer := f.command(t, 0, "3")
	requireMessage(t, "leader", answer, serverpackets.SystemMessageContribScoreIncreasedS1, int32(5))
	if got := festivalPage(t, answer); got != "page 3d" {
		t.Fatalf("leader's registration = %q, want page 3d", got)
	}
	var score, dawnScore int
	if err := f.srv.DB.QueryRow(`SELECT score FROM seven_signs_festival WHERE festivalId = 0 AND cabal = 'DAWN' AND cycle = 1`).Scan(&score); err != nil || score != 0 {
		t.Fatalf("Dawn's record = %d (%v), want it left at 0", score, err)
	}
	if err := f.srv.DB.QueryRow(`SELECT dawn_festival_score FROM seven_signs_status WHERE id = 0`).Scan(&dawnScore); err != nil || dawnScore != 0 {
		t.Fatalf("dawn festival score = %d (%v), want 0", dawnScore, err)
	}
}
