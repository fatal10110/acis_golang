package olympiad

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// TestOlympiadAbortBeforeFightClosesScoreboard: a competitor leaving during
// the battle countdown stops the match before the fight; the one still in
// the stadium, never put in combat, is still told it left the combat zone
// and its scoreboard closes.
func TestOlympiadAbortBeforeFightClosesScoreboard(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, nil)
	a, b := m.ones[0], m.ones[1]

	// Midway through the battle countdown both are long placed in the
	// stadium.
	m.runUntil(t, "battle countdown at 30", 5*time.Minute, func() bool { return slices.Contains(a.course(), "SM 1495 30") })
	m.drop(t, b)
	m.over(t)

	want := lines(enterCountdown,
		"Teleport 5900 0", "Mode 1",
		battleCountdown,
		"SM 1497 Alpha", "SM 1657 Alpha 3", "SM 1658 Bravo 3", "SM 53 6651 30", "SM 284", "MatchEnd",
		townCountdown,
		"Mode 0", fmt.Sprintf("Teleport %d %d", m.start[0].X, m.start[0].Y),
	)
	if got := a.course(); !slices.Equal(got, want) {
		t.Fatalf("Alpha course:\n got %q\nwant %q", got, want)
	}
}
