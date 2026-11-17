package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"
)

// Reference: FlagWar.onMyDying (the final's branch), endSiege.

// msgSiegeDraw is SIEGE_S1_DRAW.
const msgSiegeDraw = 856

// flagWarFinal plays the Bandit Stronghold's flag war that Holders owns up
// to its final: Besiegers (with the Attacker) is registered with the
// warrior 35428, Holders owns the warrior 35430. The warriors of no clan
// fall in the tryouts, and the final has begun when it returns.
func flagWarFinal(t *testing.T) *flagWarWorld {
	t.Helper()
	besiegers, holders := strconv.Itoa(int(besiegersClanID)), strconv.Itoa(int(holdersClanID))
	w := bootFlagWar(t, flagWarSetup{owned: true, seed: []string{
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35423, " + strconv.Itoa(ally1ID) + ", " + besiegers + ")",
		"INSERT INTO clanhall_flagwar_members SELECT 35, " + besiegers + ", obj_Id FROM characters WHERE char_name = 'Attacker'",
		"INSERT INTO clanhall_flagwar_owner_npcs VALUES (35, 35430, " + holders + ")",
	}})
	if !w.siege.Instant() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	w.srv.Advance(t, 300*time.Second)
	for _, h := range w.warriors(ally2ID) {
		w.kill(t, h)
	}
	w.srv.Advance(t, 300*time.Second)
	w.wantStanding(t, "the final", map[int]int{royalFlagID: 1, redFlagID: 1, ally1ID: 1, ally3ID: 1}, flagWarIDs...)
	if !w.siege.InProgress() || !w.field.Active() {
		t.Fatalf("the final did not begin: in progress %v, battlefield %v", w.siege.InProgress(), w.field.Active())
	}
	drainFrames(t, w.c)
	return w
}

// TestBanditStrongholdFinalTimesOutLeavesNoOwner: the final of the hall
// Holders owns runs out its 40 minutes with both warriors standing. The final
// ends drawn: everyone is told the siege was drawn and is over, and the
// hall, freed as the siege ended, is nobody's: Holders loses it, though its
// stored warrior stays. The flag war rows are cleared and the
// registrations open again.
func TestBanditStrongholdFinalTimesOutLeavesNoOwner(t *testing.T) {
	t.Parallel()
	w := flagWarFinal(t)

	// The final's 40 minutes run from the tryouts' end, the 5 minutes before
	// the warriors stand included.
	w.srv.Advance(t, 34*time.Minute)
	if !w.siege.InProgress() {
		t.Fatal("the final ended before its 40 minutes")
	}
	w.srv.Advance(t, 2*time.Minute)
	got := flat(sysMsgParams(t, drainFrames(t, w.c)))
	draw, over := slices.Index(got, strconv.Itoa(msgSiegeDraw)+" 35"), slices.Index(got, strconv.Itoa(msgSiegeFinished)+" 35")
	if draw < 0 || over < draw {
		t.Fatalf("the final's time-out notices = %v, want %d then %d", got, msgSiegeDraw, msgSiegeFinished)
	}
	if slices.Contains(got, strconv.Itoa(msgMatchDraw)+" 35") {
		t.Fatalf("the final's time-out told the first phase's draw: %v", got)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall owner = %#x, want nobody: the owner is left without the hall", owner)
	}
	if !w.siege.Registering() || w.field.Active() {
		t.Fatalf("after the time-out: registering %v, battlefield %v", w.siege.Registering(), w.field.Active())
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the time-out: clans %v, members %v", c, m)
	}
	if got, want := w.ownerNPCs(t), []string{row(35, ally3ID, int64(holdersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("owner NPC rows = %v, want %v: the stored warrior stays", got, want)
	}
	w.wantStanding(t, "the time-out", map[int]int{}, append(flagWarIDs, teleporterIDs...)...)
}

// TestBanditStrongholdChallengerFallsInTheFinal: Besiegers' warrior falls
// in the final of the hall Holders owns. Holders, the clan left, keeps the
// hall: everyone is told Holders' clan won, the hall stays Holders' and its
// stored warrior row is not written again.
func TestBanditStrongholdChallengerFallsInTheFinal(t *testing.T) {
	t.Parallel()
	w := flagWarFinal(t)
	before := w.ownerNPCs(t)
	if want := []string{row(35, ally3ID, int64(holdersClanID))}; !slices.Equal(before, want) {
		t.Fatalf("owner NPC rows before the end = %v, want %v", before, want)
	}

	w.kill(t, w.warriors(ally1ID)[0])
	got := sysMsgParams(t, drainFrames(t, w.c))
	if !slices.ContainsFunc(got, func(m []string) bool {
		return slices.Equal(m, []string{strconv.Itoa(msgClanDefeated), "Holders", "35"})
	}) ||
		!slices.ContainsFunc(got, func(m []string) bool { return slices.Equal(m, []string{strconv.Itoa(msgSiegeFinished), "35"}) }) {
		t.Fatalf("the end's notices = %v, want Holders' victory and the siege's end", got)
	}
	if owner := w.hallOwner(t); owner != holdersClanID {
		t.Fatalf("hall owner = %#x, want Holders", owner)
	}
	if after := w.ownerNPCs(t); !slices.Equal(after, before) {
		t.Fatalf("owner NPC rows after the end = %v, want %v untouched", after, before)
	}
	if !w.siege.Registering() || w.field.Active() {
		t.Fatalf("after the end: registering %v, battlefield %v", w.siege.Registering(), w.field.Active())
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the end: clans %v, members %v", c, m)
	}
	w.wantStanding(t, "the end", map[int]int{}, append(flagWarIDs, teleporterIDs...)...)
}
