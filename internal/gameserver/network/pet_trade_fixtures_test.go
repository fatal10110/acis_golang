package network

// Shared fixtures for the surviving pet/trade unit tests that were
// previously declared inside the flow-covered test files deleted for
// #1681 (pet_test.go, trade_integration_test.go).

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func petTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          2375,
			Name:        "Wolf Tooth",
			Kind:        item.KindWeapon,
			Slot:        item.SlotWolf,
			Duration:    -1,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Weapon:      &item.WeaponDetail{Type: item.WeaponPet},
		},
		{
			ID:          9000,
			Name:        "Forbidden",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    false,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
	})
}

func newDirectTradeFixture(t *testing.T) (*GameClientLink, *gamesql.ItemStore, *testsupport.FrameCapture, *testsupport.FrameCapture, *livePlayer, *livePlayer) {
	t.Helper()
	state := world.New()
	firstCap, secondCap := &testsupport.FrameCapture{}, &testsupport.FrameCapture{}
	first := newTestLivePlayer(t, 1, firstCap)
	first.Name = "TraderOne"
	second := newTestLivePlayer(t, 2, secondCap)
	second.Name = "TraderTwo"
	state.Spawn(first, 0, 0, 0, 0)
	state.AddPlayer(first)
	state.Spawn(second, 100, 0, 0, 0)
	state.AddPlayer(second)
	testsupport.ResetCapture(firstCap, secondCap)

	store := gamesql.NewItemStore(sqltest.SharedDB(t))
	ids := &sequentialIDs{next: 1000}
	updates := task.NewInventoryUpdates()
	link := &GameClientLink{
		world:            state,
		itemTemplates:    testItemTemplates(),
		items:            store,
		ids:              ids,
		inventory:        invops.NewService(ids),
		trades:           tradebook.NewBook(time.Now),
		inventoryUpdates: updates,
		log:              zerolog.Nop(),
	}
	for _, live := range []*livePlayer{first, second} {
		inv := live.Inventory()
		live := live
		inv.SetUpdateNotifier(func() {
			updates.Add(inv, live)
		})
	}
	return link, store, firstCap, secondCap, first, second
}

func assertTradeDoneFrame(t *testing.T, frame []byte, success bool) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSendTradeDone {
		t.Fatalf("SendTradeDone opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSendTradeDone)
	}
	r := wire.NewReader(frame[1:])
	got := r.ReadInt32()
	want := int32(0)
	if success {
		want = 1
	}
	if got != want {
		t.Fatalf("SendTradeDone success = %d, want %d", got, want)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SendTradeDone: %v", err)
	}
}
