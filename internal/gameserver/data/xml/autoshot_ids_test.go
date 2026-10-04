package xml

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestAutoShotIDSetsMatchDatapack checks the item id ranges
// RequestAutoSoulShot.java hard-codes against the shipped item data.
// 3947-3952 (refused in Olympiad) are the BlessedSpiritShots items. The
// SPIRITSHOTS_GRADE_MISMATCH ids (2509-2514, 3947-3952, 5790) are the
// default_action spiritshot items. 6645 is the only summon_soulshot item.
// 6647 is the Olympiad-restricted summon_spiritshot item.
func TestAutoShotIDSetsMatchDatapack(t *testing.T) {
	t.Parallel()
	table, err := LoadItemTemplates(datapackPath(t, filepath.Join("data", "xml", "items")), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	blessed, spirit := 0, 0
	for _, tpl := range table.All() {
		isBlessed := tpl.EtcItem != nil && tpl.EtcItem.Handler == "BlessedSpiritShots"
		if got := item.IsBlessedSpiritshotID(tpl.ID); got != isBlessed {
			t.Errorf("IsBlessedSpiritshotID(%d %q) = %v, want %v", tpl.ID, tpl.Name, got, isBlessed)
		}
		isSpirit := tpl.DefaultAction == item.ActionSpiritshot
		if got := item.IsAutoSpiritshotID(tpl.ID); got != isSpirit {
			t.Errorf("IsAutoSpiritshotID(%d %q) = %v, want %v", tpl.ID, tpl.Name, got, isSpirit)
		}
		isSummonShot := tpl.DefaultAction == item.ActionSummonSoulshot || tpl.DefaultAction == item.ActionSummonSpiritshot
		if got := item.IsSummonShotID(tpl.ID); got != isSummonShot {
			t.Errorf("IsSummonShotID(%d %q) = %v, want %v", tpl.ID, tpl.Name, got, isSummonShot)
		}
		if got, want := tpl.ID == item.BeastSoulshotID, tpl.DefaultAction == item.ActionSummonSoulshot; got != want {
			t.Errorf("item %d %q: is BeastSoulshotID = %v, is summon_soulshot = %v", tpl.ID, tpl.Name, got, want)
		}
		if got, want := tpl.ID == item.BlessedBeastSpiritshotID, tpl.DefaultAction == item.ActionSummonSpiritshot && tpl.OlyRestricted; got != want {
			t.Errorf("item %d %q: is BlessedBeastSpiritshotID = %v, is Olympiad-restricted summon_spiritshot = %v", tpl.ID, tpl.Name, got, want)
		}
		if isBlessed {
			blessed++
		}
		if isSpirit {
			spirit++
		}
	}
	if blessed != 6 || spirit != 13 {
		t.Fatalf("datapack has %d blessed and %d spiritshot items, want 6 and 13", blessed, spirit)
	}
}
