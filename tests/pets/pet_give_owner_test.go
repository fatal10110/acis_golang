package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// hpSwordID is the shared catalog's sword, rigged below to add
	// hpSwordBonus max HP while worn.
	hpSwordID    = int32(30)
	hpSwordBonus = 100
)

// hpSwordCatalog is the shared catalog with the sword adding hpSwordBonus max
// HP while worn.
func hpSwordCatalog() *item.Table {
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == hpSwordID {
			rigged := *tmpl
			rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: hpSwordBonus}}
			tmpl = &rigged
		}
		templates = append(templates, tmpl)
	}
	return item.NewTable(templates)
}

// TestGiveWornItemToPetTakesItsStatsOff has the owner wear a sword adding max
// HP, then hand it to the wolf. The sword comes off as it leaves: the owner's
// max HP drops back to base and UserInfo shows it. Taken back and worn again,
// the sword adds its bonus once, not twice.
func TestGiveWornItemToPetTakesItsStatsOff(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(hpSwordCatalog())},
		seedItem{TemplateID: hpSwordID, Count: 1})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	sword := h.seededItem(t, hpSwordID)
	base := h.srv.PlayerMaxHP(t, h.ownerID)

	h.sendAndTick(t, encodeUseItem(sword, false))
	worn := h.srv.PlayerMaxHP(t, h.ownerID)
	if worn != base+hpSwordBonus {
		t.Fatalf("max HP wearing the sword = %d, want %d", worn, base+hpSwordBonus)
	}

	frames := h.sendAndTick(t, encodeRequestGiveItemToPet(sword, 1))
	if got := h.srv.PlayerMaxHP(t, h.ownerID); got != base {
		t.Fatalf("max HP after handing the worn sword over = %d, want base %d", got, base)
	}
	if !hasOpcode(frames, serverpackets.OpcodeUserInfo) {
		t.Fatalf("give frames = %x, want the owner's UserInfo", frameOpcodes(frames))
	}
	if got := h.collarItemCount(t, hpSwordID); got != 1 {
		t.Fatalf("pet swords = %d, want 1", got)
	}

	h.sendAndTick(t, encodeRequestGetItemFromPet(sword, 1))
	h.sendAndTick(t, encodeUseItem(sword, false))
	if got := h.srv.PlayerMaxHP(t, h.ownerID); got != worn {
		t.Fatalf("max HP wearing the sword again = %d, want %d", got, worn)
	}
}

// TestDeadOwnerGivesItemToPet hands adena to a living wolf from a dead
// owner. The owner's death does not gate the hand-over: the adena moves.
func TestDeadOwnerGivesItemToPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	adena := h.seededItem(t, item.AdenaID)
	h.srv.MarkPlayerDead(t, h.ownerID)

	h.sendAndTick(t, encodeRequestGiveItemToPet(adena, 10))
	if got := h.collarItemCount(t, item.AdenaID); got != 10 {
		t.Fatalf("pet adena = %d, want 10", got)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 90 {
		t.Fatalf("owner adena = %d, want 90", got)
	}
}
