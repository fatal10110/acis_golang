package player

import (
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// ---- from character_regenmax_test.go ----
func TestCharacterIsPlayer(t *testing.T) {
	if _, ok := any(&Character{}).(interface{ IsPlayer() bool }); !ok {
		t.Fatal("Character does not identify itself as a player to effects")
	}
}

func TestNewCharacter(t *testing.T) {
	tmpl := humanFighterTemplate()

	c, err := NewCharacter(0x10000001, tmpl, "acct1", "Newbie", 1, 2, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}

	if c.ID != 0x10000001 || c.AccountName != "acct1" || c.Name != "Newbie" {
		t.Fatalf("NewCharacter() identity = %+v", c)
	}
	if c.ClassID() != 0 || c.BaseClassID() != 0 {
		t.Errorf("ClassID/BaseClassID = %d/%d, want 0/0", c.ClassID(), c.BaseClassID())
	}
	if c.Race != RaceHuman {
		t.Errorf("Race = %v, want %v", c.Race, RaceHuman)
	}
	if c.Sex() != SexMale {
		t.Errorf("Sex = %v, want %v", c.Sex(), SexMale)
	}
	if c.CharLevel != 1 {
		t.Errorf("Level = %d, want 1", c.CharLevel)
	}
	res := c.ResourceValues()
	if res.MaxHP != tmpl.HPTable[0] {
		t.Errorf("stored max HP base = %v, want raw table value %v", res.MaxHP, tmpl.HPTable[0])
	}
	if want := float64(int(tmpl.HPTable[0] * statbonus.CONBonus[tmpl.CON])); res.CurrentHP != want {
		t.Errorf("CurrentHP = %v, want computed max %v", res.CurrentHP, want)
	}
	if res.MaxMP != tmpl.MPTable[0] {
		t.Errorf("stored max MP base = %v, want raw table value %v", res.MaxMP, tmpl.MPTable[0])
	}
	if want := float64(int(tmpl.MPTable[0] * statbonus.MENBonus[tmpl.MEN])); res.CurrentMP != want {
		t.Errorf("CurrentMP = %v, want computed max %v", res.CurrentMP, want)
	}
	if res.MaxCP != tmpl.CPTable[0] {
		t.Errorf("stored max CP base = %v, want raw table value %v", res.MaxCP, tmpl.CPTable[0])
	}
	if res.CurrentCP != 0 {
		t.Errorf("CurrentCP = %v, want 0", res.CurrentCP)
	}
	if c.HairStyle != 1 || c.HairColor != 2 || c.Face != 0 {
		t.Errorf("appearance = hairStyle=%d hairColor=%d face=%d, want 1/2/0", c.HairStyle, c.HairColor, c.Face)
	}
	if c.Location != tmpl.Spawns[0] {
		t.Errorf("Location = %+v, want %+v", c.Location, tmpl.Spawns[0])
	}
	if c.AccessLevel != defaultAccessLevel {
		t.Errorf("AccessLevel = %d, want %d", c.AccessLevel, defaultAccessLevel)
	}
}

// TestNewCharacterVitalsApplyBonusOnce pins that a freshly created
// character's computed maxima fold the CON/MEN bonus exactly once: the raw
// level-table bases stored at creation are finalized through the live stat
// calculator without pre-multiplication (then truncated to whole points), and
// no current value starts above its own maximum.
func TestNewCharacterVitalsApplyBonusOnce(t *testing.T) {
	tmpl := humanFighterTemplate()

	c, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	c.AttachRuntime(tmpl, nil)

	res := c.ResourceValues()
	if want := math.Trunc(tmpl.HPTable[0] * statbonus.CONBonus[tmpl.CON]); res.MaxHP != want {
		t.Errorf("MaxHPValue() = %v, want table*CONBonus applied once, truncated = %v", res.MaxHP, want)
	}
	if want := math.Trunc(tmpl.MPTable[0] * statbonus.MENBonus[tmpl.MEN]); res.MaxMP != want {
		t.Errorf("MaxMPValue() = %v, want table*MENBonus applied once, truncated = %v", res.MaxMP, want)
	}
	if want := math.Trunc(tmpl.CPTable[0] * statbonus.CONBonus[tmpl.CON]); res.MaxCP != want {
		t.Errorf("MaxCPValue() = %v, want table*CONBonus applied once, truncated = %v", res.MaxCP, want)
	}
	if res.CurrentHP > res.MaxHP || res.CurrentMP > res.MaxMP || res.CurrentCP > res.MaxCP {
		t.Errorf("current vitals %+v exceed their maxima on a fresh character", res)
	}
}

func TestNewCharacter_NilTemplate(t *testing.T) {
	if _, err := NewCharacter(1, nil, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with nil template: want error, got nil")
	}
}

// TestRestoreVitalsRebasesFromTemplate pins the restore boundary for
// persisted vitals: a characters row stores finalized max snapshots (Save
// writes ResourceValues), so restoring them verbatim into the raw base fields
// would re-apply the CON/MEN finalize on every read and compound across
// save→load cycles. RestoreVitals must re-derive the bases from the class
// level tables — keeping each cycle's computed maxima identical — while the
// current values survive untouched.
func TestRestoreVitalsRebasesFromTemplate(t *testing.T) {
	tmpl := humanFighterTemplate()

	restored, err := NewCharacter(1, tmpl, "acct1", "Restored", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	restored.CharLevel = 2

	finalHP := tmpl.HPTable[1] * statbonus.CONBonus[tmpl.CON]
	finalMP := tmpl.MPTable[1] * statbonus.MENBonus[tmpl.MEN]
	restored.SetResourceValues(Resources{
		MaxHP: finalHP, CurrentHP: finalHP / 2,
		MaxCP: tmpl.CPTable[1] * statbonus.CONBonus[tmpl.CON], CurrentCP: 0,
		MaxMP: finalMP, CurrentMP: finalMP / 2,
	})
	restored.AttachRuntime(tmpl, nil)

	restored.RestoreVitals(tmpl)

	res := restored.ResourceValues()
	if want := math.Trunc(tmpl.HPTable[1] * statbonus.CONBonus[tmpl.CON]); res.MaxHP != want {
		t.Errorf("MaxHPValue() = %v, want table*CONBonus applied once, truncated = %v", res.MaxHP, want)
	}
	if want := math.Trunc(tmpl.MPTable[1] * statbonus.MENBonus[tmpl.MEN]); res.MaxMP != want {
		t.Errorf("MaxMPValue() = %v, want table*MENBonus applied once, truncated = %v", res.MaxMP, want)
	}
	if want := math.Trunc(tmpl.CPTable[1] * statbonus.CONBonus[tmpl.CON]); res.MaxCP != want {
		t.Errorf("MaxCPValue() = %v, want table*CONBonus applied once, truncated = %v", res.MaxCP, want)
	}
	if want := finalHP / 2; res.CurrentHP != want {
		t.Errorf("CurrentHP = %v, want restored value preserved = %v", res.CurrentHP, want)
	}
	if want := finalMP / 2; res.CurrentMP != want {
		t.Errorf("CurrentMP = %v, want restored value preserved = %v", res.CurrentMP, want)
	}

	// A second save→load cycle must land on exactly the same maxima and
	// currents: snapshot is what Save persists, replayed through the restore
	// boundary.
	snapshot := restored.ResourceValues()
	recycled, err := NewCharacter(1, tmpl, "acct1", "Restored", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	recycled.CharLevel = 2
	recycled.SetResourceValues(snapshot)
	recycled.AttachRuntime(tmpl, nil)
	recycled.RestoreVitals(tmpl)

	if next := recycled.ResourceValues(); next != snapshot {
		t.Errorf("second round trip = %+v, want unchanged %+v", next, snapshot)
	}

	// Levels without a table row leave the stored bases untouched, matching
	// AddLevel's refill convention, and a nil template is a no-op.
	recycled.CharLevel = len(tmpl.HPTable) + 1
	stale := recycled.ResourceValues()
	recycled.RestoreVitals(tmpl)
	if after := recycled.ResourceValues(); after.MaxHP != stale.MaxHP || after.MaxMP != stale.MaxMP || after.MaxCP != stale.MaxCP {
		t.Errorf("RestoreVitals beyond the table changed maxima %+v → %+v, want untouched", stale, after)
	}
	recycled.CharLevel = 2
	recycled.RestoreVitals(nil)
	if after := recycled.ResourceValues(); after.MaxHP != stale.MaxHP {
		t.Errorf("RestoreVitals(nil) changed MaxHP to %v, want untouched %v", after.MaxHP, stale.MaxHP)
	}
}

func TestNewCharacter_UnknownClass(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.ID = 9999
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with unknown class id: want error, got nil")
	}
}

func TestNewCharacter_MissingLevelTables(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.HPTable = nil
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with no HP table: want error, got nil")
	}
}

func TestNewCharacter_NoSpawnsLeavesZeroPosition(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.Spawns = nil

	c, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	if c.Location != (location.Location{}) {
		t.Errorf("Location = %+v, want zero value", c.Location)
	}
}

// ---- from character_vitals_test.go ----
func TestCharacterResourcesAreNotExportedFields(t *testing.T) {
	typ := reflect.TypeOf(Character{})
	for _, name := range []string{"MaxHP", "CurHP", "MaxMP", "CurMP", "MaxCP", "CurCP"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("Character exports mutable resource field %s", name)
		}
	}
}

func TestCharacterVitals(t *testing.T) {
	ch := &Character{}
	ch.SetResourceValues(Resources{CurrentHP: 12.9, CurrentMP: 7.1})

	got := ch.Vitals()
	want := Vitals{HP: 12, MP: 7}
	if got != want {
		t.Fatalf("Vitals() = %+v, want %+v", got, want)
	}
}

func TestHPFull(t *testing.T) {
	ch := &Character{}
	ch.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 99})
	if ch.HPFull() {
		t.Fatal("HPFull() = true below maximum")
	}
	ch.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 100})
	if !ch.HPFull() {
		t.Fatal("HPFull() = false at maximum")
	}
}

func TestVitalsChangesTo(t *testing.T) {
	before := Vitals{HP: 100, MP: 50}

	got := before.ChangesTo(Vitals{HP: 75, MP: 50})
	want := VitalsChange{HP: 75, HPChanged: true, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo() = %+v, want %+v", got, want)
	}
	if !got.Changed() {
		t.Fatal("Changed() = false, want true")
	}

	got = before.ChangesTo(before)
	want = VitalsChange{HP: 100, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo(unchanged) = %+v, want %+v", got, want)
	}
	if got.Changed() {
		t.Fatal("Changed() unchanged = true, want false")
	}
}

// ---- from character_weight_penalty_test.go ----
func TestRefreshWeightPenalty(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		want  int
	}{
		{"below half", .499, 0},
		{"half", .5, 1},
		{"two thirds", .666, 2},
		{"four fifths", .8, 3},
		{"full", 1, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
			c := &Character{}
			c.AttachRuntime(&Template{CON: 20}, inv)
			c.weightLimitMultiplier = 1
			inv.AddNew(1, int(math.Ceil(float64(c.WeightLimit())*tc.ratio)), 1)
			inv.UpdateWeight()
			c.RefreshWeightPenalty()
			if got := c.WeightPenalty(); got != tc.want {
				t.Fatalf("WeightPenalty() = %d, want %d (limit %d weight %d)", got, tc.want, c.WeightLimit(), c.CurrentWeight())
			}
		})
	}
}

func TestRefreshWeightPenaltyChangesOnlyOnBandChange(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.weightLimitMultiplier = 1
	inv.AddNew(1, c.WeightLimit()/2, 1)
	inv.UpdateWeight()
	rec := recordEvents(c)

	c.RefreshWeightPenalty()
	c.RefreshWeightPenalty()
	if updates := event.Count[event.WeightPenaltyChanged](rec); updates != 1 {
		t.Fatalf("updates after unchanged refresh = %d, want 1", updates)
	}

	inv.AddNew(1, c.WeightLimit()/6, 2)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 2; got != want {
		t.Fatalf("WeightPenalty() = %d, want %d", got, want)
	}
	if updates := event.Count[event.WeightPenaltyChanged](rec); updates != 2 {
		t.Fatalf("updates after band change = %d, want 2", updates)
	}
}

// TestWeightPenaltySpeedMultiplier pins the per-band speed multiplier to
// WeightPenalty.java:5-9 (NONE 1, LEVEL_1 1, LEVEL_2 0.5, LEVEL_3 0.5,
// LEVEL_4 0). LEVEL_4 must be 0, not 0.5 — a fully overloaded reference
// player cannot move (PlayerStatus.getMoveSpeed, PlayerStatus.java:944-947).
func TestWeightPenaltySpeedMultiplier(t *testing.T) {
	tests := []struct {
		band int
		want float64
	}{
		{0, 1}, {1, 1}, {2, .5}, {3, .5}, {4, 0},
	}
	for _, tc := range tests {
		c := &Character{}
		c.stateMu.Lock()
		c.weightPenalty = tc.band
		c.stateMu.Unlock()
		if got := c.weightPenaltySpeedMultiplier(); got != tc.want {
			t.Errorf("band %d: weightPenaltySpeedMultiplier() = %v, want %v", tc.band, got, tc.want)
		}
	}
}

// TestAddLevelRefreshesWeightPenalty pins PlayerStatus.addLevel's direct
// call to _actor.refreshWeightPenalty() on every level change
// (PlayerStatus.java:644, before the UserInfo send at :648). The band is
// forced stale beforehand so a passing test proves AddLevel actually
// recomputed it rather than leaving it untouched.
func TestAddLevelRefreshesWeightPenalty(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{CharLevel: 1}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.weightLimitMultiplier = 1
	inv.AddNew(1, c.WeightLimit(), 1) // full overload -> band 4
	inv.UpdateWeight()

	c.stateMu.Lock()
	c.weightPenalty = 0 // stale: as if never refreshed
	c.stateMu.Unlock()

	table, err := NewLevelTable(map[int]Level{1: {RequiredExpToLevelUp: 0}, 2: {RequiredExpToLevelUp: 100}, 3: {RequiredExpToLevelUp: 200}})
	if err != nil {
		t.Fatalf("NewLevelTable: %v", err)
	}
	c.AddLevel(table, nil, 1)

	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after AddLevel = %d, want %d (stale band never recomputed)", got, want)
	}
}

func TestRefreshWeightPenaltyKeepsStateWhenLimitIsZero(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.weightLimitMultiplier = 1
	inv.AddNew(1, c.WeightLimit(), 1)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	c.weightLimitMultiplier = 0
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after zero limit = %d, want %d", got, want)
	}
}

// reads the last-known state the way a save or a range check would. Run
// with -race.
func TestCharacterPositionAccessIsRaceFree(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SyncPosition(location.Location{X: i, Y: 0, Z: 0})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SetLastKnownPosition(location.Location{X: -i, Y: 0, Z: 0}, i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.CurrentLocation()
			_ = c.CurrentHeading()
		}
	}()

	wg.Wait()
}
