package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestHennaBonusFromRestoredDyes(t *testing.T) {
	table := henna.NewTable([]henna.Henna{
		{SymbolID: 1, STR: 1, CON: -3, Classes: []int{1}},
	})
	c := &Character{ClassID: 1}
	c.RestoreHennas([]henna.Row{{Slot: 1, SymbolID: 1}}, table.Find)
	actor := characterStatActor{c: c}
	if got := actor.HennaBonus(stat.StatSTR); got != 1 {
		t.Fatalf("STR bonus = %v, want 1", got)
	}
	if got := actor.HennaBonus(stat.StatCON); got != -3 {
		t.Fatalf("CON bonus = %v, want -3", got)
	}
	if got := actor.HennaBonus(stat.StatINT); got != 0 {
		t.Fatalf("INT bonus = %v, want 0", got)
	}
	snap := c.HennaSnapshot()
	if snap.MaxSlots != 2 || snap.STR != 1 || snap.CON != -3 || len(snap.Equipped) != 1 || snap.Equipped[0].ActiveSymbolID != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}
