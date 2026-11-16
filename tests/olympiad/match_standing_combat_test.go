package olympiad

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// combatant is a live player's pvp zone state and position.
type combatant interface {
	InPvPZone() bool
	CurrentLocation() location.Location
}

// TestOlympiadFightSwitchesStandingCompetitors: the fight starting puts both
// competitors, standing where the stadium placed them, in a pvp zone, and
// the fight ending takes them out of it while they still stand in the
// stadium, before they are sent back.
func TestOlympiadFightSwitchesStandingCompetitors(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) {
		cfg.BattleMillis = 10_000
		cfg.AnnounceGames = false
	})
	fighters := func() []combatant {
		var out []combatant
		for _, p := range m.ones {
			obj, ok := m.srv.State.Player(p.id)
			if !ok {
				t.Fatalf("%s not in world", p.name)
			}
			out = append(out, obj.(combatant))
		}
		return out
	}
	inPvP := func(want bool) func() bool {
		return func() bool {
			for _, c := range fighters() {
				if c.InPvPZone() != want {
					return false
				}
			}
			return true
		}
	}

	m.drawn(t)
	if !inPvP(false)() {
		t.Fatal("competitors in a pvp zone before the fight")
	}
	m.runUntil(t, "both competitors in combat", 5*time.Minute, inPvP(true))
	m.runUntil(t, "both competitors out of combat", 5*time.Minute, inPvP(false))
	for i, c := range fighters() {
		if at := c.CurrentLocation(); at.X < 5000 || at.X > 7000 {
			t.Fatalf("%s left combat only once sent back to %+v", m.ones[i].name, at)
		}
	}
}
