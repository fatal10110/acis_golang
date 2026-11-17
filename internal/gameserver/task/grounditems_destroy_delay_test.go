package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func groundItemOf(t *testing.T, tmpl *item.Template) *grounditem.Item {
	t.Helper()
	inst := item.Instance{ObjectID: 1, TemplateID: tmpl.ID, Count: 1, Location: item.LocationVoid}
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New() error = %v", err)
	}
	return ground
}

// TestGroundItemsDestroyDelayPerItemClass pins which time a ground item gets:
// a special time first, then the herb, equipable and castle-ticket classes,
// else the plain item time; a drop by a player or its summon is multiplied,
// a ticket's zero stays zero.
func TestGroundItemsDestroyDelayPerItemClass(t *testing.T) {
	opts := GroundItemOptions{
		HerbAutoDestroy:         3 * time.Second,
		ItemAutoDestroy:         10 * time.Second,
		EquipableAutoDestroy:    20 * time.Second,
		SpecialAutoDestroy:      map[int32]time.Duration{100: 40 * time.Second, 200: 50 * time.Second, 300: 60 * time.Second},
		PlayerDroppedMultiplier: 5,
	}
	plain := &item.Template{ID: 1, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}
	herb := &item.Template{ID: 2, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemHerb}}
	armor := &item.Template{ID: 3, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{}}
	ticket := &item.Template{ID: 4, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}
	specialPlain := &item.Template{ID: 100, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}
	specialHerb := &item.Template{ID: 200, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemHerb}}
	specialArmor := &item.Template{ID: 300, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{}}
	ticketIDs := map[int32]bool{ticket.ID: true}

	for _, tc := range []struct {
		name         string
		tmpl         *item.Template
		playerDrop   bool
		want         time.Duration
		withTicketAt bool
	}{
		{"plain item", plain, false, 10 * time.Second, true},
		{"plain item dropped by a player", plain, true, 50 * time.Second, true},
		{"herb", herb, false, 3 * time.Second, true},
		{"herb dropped by a player", herb, true, 15 * time.Second, true},
		{"equipable", armor, false, 20 * time.Second, true},
		{"equipable dropped by a player", armor, true, 100 * time.Second, true},
		{"castle ticket on its castle ground", ticket, false, 0, true},
		{"castle ticket dropped by a player stays never-destroyed", ticket, true, 0, true},
		{"castle ticket id elsewhere is a plain item", ticket, false, 10 * time.Second, false},
		{"special over plain", specialPlain, false, 40 * time.Second, true},
		{"special over herb", specialHerb, false, 50 * time.Second, true},
		{"special over equipable", specialArmor, false, 60 * time.Second, true},
		{"special dropped by a player", specialPlain, true, 200 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGroundItems(nil, opts, time.Now, nil)
			g.SetKept(func(itemID int32, _, _, _ int) bool { return tc.withTicketAt && ticketIDs[itemID] })
			if got := g.destroyDelay(groundItemOf(t, tc.tmpl), tc.playerDrop); got != tc.want {
				t.Errorf("destroyDelay() = %v, want %v", got, tc.want)
			}
		})
	}
}
