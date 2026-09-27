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
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func newDirectTradeFixture(t *testing.T) (*GameClientLink, *gamesql.ItemStore, *testsupport.FrameCapture, *testsupport.FrameCapture, *livePlayer, *livePlayer) {
	t.Helper()
	updates := task.NewInventoryUpdates()
	state := world.New()
	firstCap, secondCap := &testsupport.FrameCapture{}, &testsupport.FrameCapture{}
	firstDelivery := &playerInventoryDelivery{updates: updates}
	first := newTestLivePlayer(t, 1, firstCap, firstDelivery)
	firstDelivery.live, firstDelivery.character = first, first.Character
	first.Name = "TraderOne"
	secondDelivery := &playerInventoryDelivery{updates: updates}
	second := newTestLivePlayer(t, 2, secondCap, secondDelivery)
	secondDelivery.live, secondDelivery.character = second, second.Character
	second.Name = "TraderTwo"
	state.Spawn(first, 0, 0, 0, 0)
	state.AddPlayer(first)
	state.Spawn(second, 100, 0, 0, 0)
	state.AddPlayer(second)
	testsupport.ResetCapture(firstCap, secondCap)

	store := gamesql.NewItemStore(sqltest.SharedDB(t))
	ids := &sequentialIDs{next: 1000}
	link := &GameClientLink{
		world:            state,
		itemTemplates:    testItemTemplates(),
		items:            store,
		itemWrites:       persist.NewOrder(),
		ids:              ids,
		inventory:        invops.NewService(ids),
		trades:           tradebook.NewBook(time.Now),
		inventoryUpdates: updates,
		log:              zerolog.Nop(),
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
