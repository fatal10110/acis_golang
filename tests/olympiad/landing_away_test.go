package olympiad

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// runHolding lets the clock run a second at a time, reading every frame
// but held's own teleports, until done holds; it gives up after limit.
func (m *match) runHolding(t *testing.T, held *competitor, what string, limit time.Duration, done func() bool) {
	t.Helper()
	for passed := time.Duration(0); !done(); passed += time.Second {
		if passed > limit {
			t.Fatalf("%s not reached within %v", what, limit)
		}
		m.srv.Advance(t, time.Second)
		m.readHolding(t, held)
	}
}

// hasLinePrefix reports whether one of lines starts with prefix.
func hasLinePrefix(lines []string, prefix string) bool {
	return slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, prefix) })
}

// TestOlympiadAwayCompetitorDefects pins the owner-approved defection of a
// competitor whose held stadium entry never lands before the fight
// (#5121): recalled by a GM as the match is drawn, Alpha holds the
// recall's landing back past the battle countdown. The fight does not
// start with Alpha outside the stadium; Alpha defects, losing its points,
// Bravo is told its opponent does not meet the requirements and the match
// is no tie. Alpha leaves the match, and once it lands it is sent back to
// where it entered from instead of into the stadium.
func TestOlympiadAwayCompetitorDefects(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, withAdmin(t))
	gm := m.addGM(t)
	a, b := m.ones[0], m.ones[1]
	c := m.online(t, a)
	s := m.stadium(t)

	m.drawn(t)
	m.command(t, gm, "recall Alpha")
	m.readHolding(t, a)
	if !c.Teleporting() {
		t.Fatal("the recall is not in flight")
	}
	recalled := c.CurrentLocation()
	m.runHolding(t, a, "match over", 15*time.Minute, func() bool { return !s.Running() })
	m.srv.Advance(t, time.Second)
	m.readHolding(t, a)

	course := b.course()
	if !slices.Contains(course, fmt.Sprintf("SM %d", serverpackets.SystemMessageOlympiadOpponentUnfit)) {
		t.Fatalf("Bravo's course = %q, want told its opponent is unfit", course)
	}
	for _, banned := range []int32{serverpackets.SystemMessageOlympiadStartsTheGame, serverpackets.SystemMessageOlympiadGameEndedInATie} {
		if hasLinePrefix(course, fmt.Sprintf("SM %d", banned)) {
			t.Fatalf("Bravo's course = %q, want no SM %d: the fight never starts and is no tie", course, banned)
		}
	}
	if !hasLinePrefix(course, fmt.Sprintf("SM %d Alpha ", serverpackets.SystemMessageS1HasLostS2OlympiadPoints)) {
		t.Fatalf("Bravo's course = %q, want Alpha losing points", course)
	}
	if c.OlympiadMode() || c.OlympiadGameID() != -1 || !c.Teleporting() || c.CurrentLocation() != recalled {
		t.Fatalf("after the match: mode=%v game=%d teleporting=%v at %+v, want out of the match still landing at %+v",
			c.OlympiadMode(), c.OlympiadGameID(), c.Teleporting(), c.CurrentLocation(), recalled)
	}

	got := m.land(t, a)
	if slices.Contains(got, fmt.Sprintf("Teleport %d %d", stadiumSide1.X, stadiumSide1.Y)) {
		t.Fatalf("Alpha's landing = %q, want no entry into the stadium", got)
	}
	if at := c.CurrentLocation(); c.OlympiadMode() || at.X != recalled.X || at.Y != recalled.Y {
		t.Fatalf("after the landing: mode=%v at %+v, want sent back to %+v", c.OlympiadMode(), at, recalled)
	}
}

// TestOlympiadSendBackWaitsForTheLanding pins the owner-approved retry of
// the return from the stadium refused while the competitor's own teleport
// lands (#5121): recalled by a GM once the fight is over, Alpha holds the
// recall's landing back past the cleanup, so its return is held; once it
// lands it is sent back to where it entered from, out of the Olympiad.
func TestOlympiadSendBackWaitsForTheLanding(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, withAdmin(t))
	gm := m.addGM(t)
	a := m.ones[0]
	c := m.online(t, a)
	s := m.stadium(t)

	m.fighting(t)
	entered := m.start[0]
	toTown := fmt.Sprintf("SM %d ", serverpackets.SystemMessageMovedToTownInS1Seconds)
	m.runUntil(t, "town countdown", 2*time.Minute, func() bool { return hasLinePrefix(a.course(), toTown) })
	m.command(t, gm, "recall Alpha")
	m.readHolding(t, a)
	if !c.Teleporting() {
		t.Fatal("the recall is not in flight")
	}
	recalled := c.CurrentLocation()
	m.runHolding(t, a, "match over", 2*time.Minute, func() bool { return !s.Running() })
	m.srv.Advance(t, time.Second)
	m.readHolding(t, a)
	if c.OlympiadMode() || c.CurrentLocation() != recalled {
		t.Fatalf("after the cleanup: mode=%v at %+v, want out of the match still landing at %+v", c.OlympiadMode(), c.CurrentLocation(), recalled)
	}

	if got, want := m.land(t, a), fmt.Sprintf("Teleport %d %d", entered.X, entered.Y); !slices.Contains(got, want) {
		t.Fatalf("Alpha's landing = %q, want the held %q", got, want)
	}
	if at := c.CurrentLocation(); c.OlympiadMode() || at.X != entered.X || at.Y != entered.Y {
		t.Fatalf("after the landing: mode=%v at %+v, want sent back to %+v", c.OlympiadMode(), at, entered)
	}
}

// strayQuest is the script whose teleport is held behind a stadium entry in
// TestOlympiadEntryOutlastsAHeldScriptMove.
const strayQuest = "Q954_StrayTeleport"

// TestOlympiadEntryOutlastsAHeldScriptMove pins that a script's teleport
// held for a landing yields to a stadium entry held for the same landing
// (owner-approved, #5121): with Alpha's recall in flight, its entry jump
// is held, then a script asks Alpha elsewhere. The landing takes Alpha
// into the stadium, and the script's move, refused there once more, is
// dropped once Alpha lands as a competitor, so Alpha stays on its spawn.
func TestOlympiadEntryOutlastsAHeldScriptMove(t *testing.T) {
	t.Parallel()
	catalog := script.Catalog{"quest." + strayQuest: func() script.Script {
		return script.Script{Title: "Stray Teleport", QuestID: 954}
	}}
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, withAdmin(t),
		gameservertest.WithNPCScripts(nil, []script.Listing{{Path: "quest." + strayQuest}}, catalog))
	gm := m.addGM(t)
	a := m.ones[0]
	c := m.online(t, a)

	m.drawn(t)
	m.command(t, gm, "recall Alpha")
	m.readHolding(t, a)
	recalled := c.CurrentLocation()
	m.runHolding(t, a, "stadium entry", 2*time.Minute, c.OlympiadMode)
	away := script.Loc{X: recalled.X + 300, Y: recalled.Y, Z: recalled.Z}
	m.srv.RunScript(t, a.id, strayQuest, func(_ *script.Script, p *script.Player) { p.TeleportTo(away, 0) })
	m.readHolding(t, a)

	if got, want := m.land(t, a), fmt.Sprintf("Teleport %d %d", stadiumSide1.X, stadiumSide1.Y); !slices.Contains(got, want) {
		t.Fatalf("Alpha's landing = %q, want the held %q", got, want)
	}
	from := len(a.frames)
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)
	if got := spectatorLines(a.id, a.frames[from:]); slices.Contains(got, fmt.Sprintf("Teleport %d %d", away.X, away.Y)) {
		t.Fatalf("Alpha's stadium landing = %q, want the script's move dropped", got)
	}
	if at := c.CurrentLocation(); at.X != stadiumSide1.X || at.Y != stadiumSide1.Y || !c.OlympiadMode() {
		t.Fatalf("after the stadium landing: at %+v mode=%v, want competing on %+v", at, c.OlympiadMode(), stadiumSide1)
	}
}
