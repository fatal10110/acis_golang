package player

import (
	"math"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// refreshRounds is how many times each test races a state change and its
// refresh against a second refresh. A refresh whose read and store are not
// ordered against the other's leaves a stale value in a few rounds per
// thousand.
const refreshRounds = 20_000

// raceRefresh runs change and refresh on two goroutines released together.
func raceRefresh(change, refresh func()) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		change()
	}()
	go func() {
		defer wg.Done()
		<-start
		refresh()
	}()
	close(start)
	wg.Wait()
}

// TestMoveSpeedRefreshNeverStoresAnOlderReading: a move-speed change and
// its refresh race a refresh from another queue; once both are done, the
// movement simulation runs at the speed the current state gives.
func TestMoveSpeedRefreshNeverStoresAnOlderReading(t *testing.T) {
	c, _ := riderAt(t, 70)
	attachIdleLive(t, c)
	c.refreshMoveSpeed()
	for round := range refreshRounds {
		bonus := 0
		if round%2 == 0 {
			bonus = 50
		}
		raceRefresh(func() { c.SetSwampMoveBonus(bonus) }, c.refreshMoveSpeed)
		if got, want := c.Move().Speed(), c.MoveSpeed(); got != want {
			t.Fatalf("round %d: movement speed = %v, want %v for swamp bonus %d", round, got, want, bonus)
		}
	}
}

// TestExpertisePenaltyRefreshNeverStoresAnOlderReading: an Expertise change
// and its penalty refresh race a refresh from another queue; once both are
// done, the weapon penalty and the penalty skill follow the current
// Expertise.
func TestExpertisePenaltyRefreshNeverStoresAnOlderReading(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalB, Weapon: &item.WeaponDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	inst := inv.AddNew(1, 1, 101)
	tmpl, _ := templates.Get(1)
	inv.EquipItem(inst, tmpl)
	c := &Character{ID: 1}
	c.AttachRuntime(nil, inv)
	recordEvents(c)
	c.RefreshExpertisePenalty()
	for round := range refreshRounds {
		expertise, penalty := int(item.CrystalB), false
		if round%2 == 0 {
			expertise, penalty = int(item.CrystalNone), true
		}
		raceRefresh(func() {
			c.SetSkillLevel(expertiseSkillID, expertise)
			c.RefreshExpertisePenalty()
		}, c.RefreshExpertisePenalty)
		wantSkill := 0
		if penalty {
			wantSkill = 1
		}
		if got, skill := c.WeaponGradePenalty(), c.SkillLevel(gradePenaltySkillID); got != penalty || skill != wantSkill {
			t.Fatalf("round %d: weapon penalty %v, penalty skill %d; want %v, %d for Expertise %d", round, got, skill, penalty, wantSkill, expertise)
		}
	}
}

// TestWeightPenaltyRefreshNeverStoresAnOlderReading: a weight change and its
// band refresh race a refresh from another queue; once both are done, the
// band is the one the current weight gives.
func TestWeightPenaltyRefreshNeverStoresAnOlderReading(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	setRules(c, func(r *Rules) { r.WeightLimitMultiplier = 1 })
	recordEvents(c)
	heavy := int(math.Ceil(float64(c.WeightLimit()) * .7))
	inv.AddNew(1, heavy, 1)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	for round := range refreshRounds {
		// Even rounds drop the stack to one item, odd ones restore it.
		change, band := func() { inv.AddNew(1, heavy-1, 1) }, 2
		if round%2 == 0 {
			change, band = func() { inv.DestroyByTemplateID(1, heavy-1) }, 0
		}
		raceRefresh(func() {
			change()
			inv.UpdateWeight()
			c.RefreshWeightPenalty()
		}, c.RefreshWeightPenalty)
		if got := c.WeightPenalty(); got != band {
			t.Fatalf("round %d: weight band = %d, want %d for weight %d of %d", round, got, band, c.CurrentWeight(), c.WeightLimit())
		}
	}
}
