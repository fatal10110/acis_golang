package script

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// characterFake is a player handle's world object holding a character.
type characterFake struct {
	timerPlayerFake
	c *player.Character
}

func (f *characterFake) PlayerCharacter() *player.Character { return f.c }

// Class ids of one fighter line, by tier.
const (
	humanFighter = 0  // tier 0
	warrior      = 1  // tier 1
	gladiator    = 2  // tier 2
	duelist      = 88 // tier 3
)

func playerAt(classID, level int) *Player {
	c := &player.Character{CharLevel: level}
	c.SetClassID(classID)
	return &Player{self: &characterFake{timerPlayerFake: timerPlayerFake{combatant: combatant{objectID: 1}}, c: c}}
}

// TestPlayerClassLevelAndIsNewbie pins the newbie test at its
// boundaries: at most the first class change, level 25 or lower, and with
// the low-level check level 6 or higher.
func TestPlayerClassLevelAndIsNewbie(t *testing.T) {
	for _, tc := range []struct {
		class, level int
		tier         int32
		newbie, low  bool
	}{
		{humanFighter, 1, 0, true, false},
		{humanFighter, 5, 0, true, false},
		{humanFighter, 6, 0, true, true},
		{humanFighter, 25, 0, true, true},
		{humanFighter, 26, 0, false, false},
		{warrior, 20, 1, true, true},
		{warrior, 25, 1, true, true},
		{warrior, 26, 1, false, false},
		{gladiator, 20, 2, false, false},
		{duelist, 20, 3, false, false},
	} {
		p := playerAt(tc.class, tc.level)
		if got := p.ClassLevel(); got != tc.tier {
			t.Errorf("class %d: ClassLevel = %d, want %d", tc.class, got, tc.tier)
		}
		if got := p.IsNewbie(false); got != tc.newbie {
			t.Errorf("class %d level %d: IsNewbie(false) = %v, want %v", tc.class, tc.level, got, tc.newbie)
		}
		if got := p.IsNewbie(true); got != tc.low {
			t.Errorf("class %d level %d: IsNewbie(true) = %v, want %v", tc.class, tc.level, got, tc.low)
		}
	}
}

// TestPlayerAfterDropsOnDetach: work scheduled with After runs on the
// player's queue once its delay passes; what is still pending when the
// player detaches (its queue closes) never runs, and After on a detaching
// player schedules nothing.
func TestPlayerAfterDropsOnDetach(t *testing.T) {
	clock := sim.NewInline(time.UnixMilli(0))
	fake := &timerPlayerFake{combatant: combatant{objectID: 1}, queue: clock.NewQueue("p")}
	p := &Player{self: fake}
	var ran []int
	for i := range 3 {
		p.After(time.Duration(i)*time.Second, func() { ran = append(ran, i) })
	}
	clock.Advance(0)
	if len(ran) != 1 || ran[0] != 0 {
		t.Fatalf("ran %v at once, want [0]", ran)
	}
	fake.detaching.Store(true)
	fake.queue.Close()
	p.After(0, func() { ran = append(ran, 9) })
	clock.Advance(5 * time.Second)
	if len(ran) != 1 {
		t.Fatalf("ran %v after detach, want only [0]", ran)
	}
}
