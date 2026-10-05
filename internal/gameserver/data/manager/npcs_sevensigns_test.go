package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

type fakeSevenSigns struct {
	period sevensigns.Period
	won    sevensigns.Cabal
	owners [3]sevensigns.Cabal
}

func (f fakeSevenSigns) CurrentPeriod() sevensigns.Period { return f.period }
func (f fakeSevenSigns) WinningCabal() sevensigns.Cabal   { return f.won }
func (f fakeSevenSigns) SealOwners() [3]sevensigns.Cabal  { return f.owners }

// TestSevenSignsGroupsFollowThePeriod checks, for every period, winner and
// pair of seal owners, the spawn condition of each Seven Signs group
// against the reference's formulas (SpawnMaker.checkHasSpawnCondition),
// and that the groups a period change starts (notifySevenSignsChange) are
// exactly the ones whose condition does not hold.
func TestSevenSignsGroupsFollowThePeriod(t *testing.T) {
	cabals := []sevensigns.Cabal{sevensigns.NoCabal, sevensigns.Dusk, sevensigns.Dawn}
	for period := sevensigns.Recruiting; period <= sevensigns.SealValidation; period++ {
		for _, won := range cabals {
			for _, avarice := range cabals {
				for _, gnosis := range cabals {
					ss := fakeSevenSigns{period: period, won: won, owners: [3]sevensigns.Cabal{avarice, gnosis, sevensigns.NoCabal}}
					contest := period == sevensigns.Recruiting || period == sevensigns.Competition
					want := map[string]bool{"ssq_event": !contest}
					for prefix, owner := range map[string]sevensigns.Cabal{"ssq_seal1": avarice, "ssq_seal2": gnosis} {
						want[prefix+"_none"] = contest || !(owner == sevensigns.NoCabal || owner != won)
						want[prefix+"_dawn"] = contest || !(owner == sevensigns.Dawn && owner == won)
						want[prefix+"_twilight"] = contest || !(owner == sevensigns.Dusk && owner == won)
					}
					started := map[string]bool{}
					if contest {
						started["ssq_event"] = true
					} else {
						started[sealGroup("ssq_seal1", avarice, won)] = true
						started[sealGroup("ssq_seal2", gnosis, won)] = true
					}
					for _, event := range sevenSignsGroups {
						held, ok := sevenSignsHeld(ss, event)
						if !ok || held != want[event] {
							t.Errorf("%v won=%v owners=%v/%v: %s held = %v (ok %v), want %v", period, won, avarice, gnosis, event, held, ok, want[event])
						}
						if started[event] == held {
							t.Errorf("%v won=%v owners=%v/%v: %s started = %v while held = %v", period, won, avarice, gnosis, event, started[event], held)
						}
					}
				}
			}
		}
	}
	if _, ok := sevenSignsHeld(nil, "christmas"); ok {
		t.Error("christmas counted as a Seven Signs group")
	}
	if held, _ := sevenSignsHeld(nil, "ssq_event"); !held {
		t.Error("a Seven Signs group spawns before the state is known")
	}
}
