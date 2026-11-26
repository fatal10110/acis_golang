package effect

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestModOwnerOwnedStats pins the stats a holder scans to drop an owner's
// Mods: an effect's own func stats in stat order, each once; every stat for
// an owner that does not know its own.
func TestModOwnerOwnedStats(t *testing.T) {
	t.Parallel()
	e := &Effect{}
	e.Funcs = []Mod{
		{Stat: stat.MagicDefence, Owner: ModOwnerEffect(e)},
		{Stat: stat.PowerAttack, Owner: ModOwnerEffect(e)},
		{Stat: stat.MagicDefence, Owner: ModOwnerEffect(e)},
		{Stat: stat.Stat(0), Owner: ModOwnerEffect(e)},
	}
	want := []stat.Stat{stat.Stat(0), stat.PowerAttack, stat.MagicDefence}
	slices.Sort(want)
	if got := slices.Collect(ModOwnerEffect(e).OwnedStats()); !slices.Equal(got, want) {
		t.Fatalf("OwnedStats(effect with funcs) = %v, want %v", got, want)
	}
	for s := range ModOwnerEffect(e).OwnedStats() {
		if s != want[0] {
			t.Fatalf("first OwnedStats() = %v, want %v", s, want[0])
		}
		break // stopping early must not panic
	}

	every := make([]stat.Stat, stat.Count)
	for i := range every {
		every[i] = stat.Stat(i)
	}
	for name, owner := range map[string]ModOwner{
		"funcless effect": ModOwnerEffect(&Effect{}),
		"skill":           ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1}),
	} {
		if got := slices.Collect(owner.OwnedStats()); !slices.Equal(got, every) {
			t.Errorf("OwnedStats(%s) yields %d stats, want all %d", name, len(got), stat.Count)
		}
	}
}
