package summon

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// Wolf (npc 12077) growth thresholds from the shipped datapack: the
// experience each level starts at. Level 81 is the sentinel row every pet
// table ends on; no pet can hold it.
const (
	wolfExpLevel79 int64 = 480562077
	wolfExpLevel80 int64 = 555934039
	wolfExpLevel81 int64 = 639402879
)

// wolfTopGrowth is the top of the wolf growth table (levels 78-81), enough
// for a pet at level 79 or 80 to walk its thresholds.
func wolfTopGrowth() *npc.PetData {
	return &npc.PetData{Levels: map[int]npc.PetLevelStats{
		78: {MaxExp: 412836202, MaxHP: 1401, MaxMP: 803},
		79: {MaxExp: wolfExpLevel79, MaxHP: 1421, MaxMP: 820},
		80: {MaxExp: wolfExpLevel80, MaxHP: 1440, MaxMP: 837},
		81: {MaxExp: wolfExpLevel81, MaxHP: 1458, MaxMP: 838},
	}}
}

// capPet returns a wolf pet at level with exp, its HP lowered to 100 so a
// level-up's vitals reset would show, and a recorder attached afterwards.
func capPet(t *testing.T, level int, exp int64) (*Actor, *event.Recorder) {
	t.Helper()
	growth := wolfTopGrowth()
	row := growth.Levels[level]
	pet := mustPet(t, PetConfig{
		ObjectID: 1,
		NPCID:    12077,
		Level:    level,
		Exp:      exp,
		Growth:   growth,
		Stats:    CombatStats{MaxHP: row.MaxHP, MaxMP: row.MaxMP},
	})
	pet.SetHP(100)
	rec := &event.Recorder{}
	pet.Attach(Runtime{Sink: rec})
	return pet, rec
}

func socialActions(events []event.Event) []int32 {
	var ids []int32
	for _, ev := range events {
		if sa, ok := ev.(event.SocialAction); ok {
			ids = append(ids, sa.ID)
		}
	}
	return ids
}

// TestPetExpPastSentinelKeepsLevel pins the pet level cap: experience that
// reaches the level-81 threshold is kept, but the level does not move. A
// grant from level 79 that crosses both the level-80 and level-81 thresholds
// is refused whole, so the pet stays at 79. Neither case plays the level-up
// animation or restores HP; the grant only refreshes status and reports the
// exp earned.
func TestPetExpPastSentinelKeepsLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		level    int
		startExp int64
		grant    int64
	}{
		{name: "level 80 crosses the level 81 threshold", level: 80, startExp: wolfExpLevel81 - 10, grant: 5000},
		{name: "level 80 lands exactly on the level 81 threshold", level: 80, startExp: wolfExpLevel81 - 10, grant: 10},
		{name: "level 79 overshoots the level 81 threshold in one grant", level: 79, startExp: wolfExpLevel79, grant: wolfExpLevel81 - wolfExpLevel79 + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pet, rec := capPet(t, tt.level, tt.startExp)

			pet.AddExpAndSp(tt.grant, 0)

			if got := pet.Level(); got != tt.level {
				t.Fatalf("Level() = %d, want unchanged %d", got, tt.level)
			}
			if got, want := pet.Exp(), tt.startExp+tt.grant; got != want {
				t.Errorf("Exp() = %d, want %d (experience is kept)", got, want)
			}
			if got := pet.HP(); got != 100 {
				t.Errorf("HP() = %v, want 100 (no level-up vitals reset)", got)
			}
			want := []event.Event{event.StatusChanged{}, event.ExpGained{Exp: tt.grant}}
			if got := rec.Events(); !reflect.DeepEqual(got, want) {
				t.Errorf("events = %+v, want %+v", got, want)
			}
		})
	}
}

// TestPetExpReachesRealMaxLevel is the control: a grant that lands inside
// level 80 still levels the pet up with the animation and a vitals reset.
func TestPetExpReachesRealMaxLevel(t *testing.T) {
	t.Parallel()
	pet, rec := capPet(t, 79, wolfExpLevel79)

	pet.AddExpAndSp(wolfExpLevel81-wolfExpLevel79-1, 0)

	if got := pet.Level(); got != 80 {
		t.Fatalf("Level() = %d, want 80", got)
	}
	if got, want := pet.HP(), pet.MaxHPValue(); got != want {
		t.Errorf("HP() = %v, want max HP %v (level-up restores full HP)", got, want)
	}
	if got := socialActions(rec.Events()); !reflect.DeepEqual(got, []int32{socialActionLevelUp}) {
		t.Errorf("SocialAction ids = %v, want [%d]", got, socialActionLevelUp)
	}
}

// TestPetRestoredExpPastSentinelKeepsLevel covers the resurrection path,
// which gives experience back through the same level walk: restoring a
// level-80 pet past the level-81 threshold keeps it at 80 and plays no
// animation.
func TestPetRestoredExpPastSentinelKeepsLevel(t *testing.T) {
	t.Parallel()
	pet, rec := capPet(t, 80, wolfExpLevel80+1000)
	pet.statusMu.Lock()
	pet.expBeforeDeath = wolfExpLevel81 + 5000
	pet.statusMu.Unlock()

	pet.restoreExp(100)

	if got := pet.Level(); got != 80 {
		t.Fatalf("Level() = %d, want unchanged 80", got)
	}
	if got, want := pet.Exp(), wolfExpLevel81+5000; got != want {
		t.Errorf("Exp() = %d, want %d", got, want)
	}
	if got := socialActions(rec.Events()); len(got) != 0 {
		t.Errorf("SocialAction ids = %v, want none", got)
	}
}
