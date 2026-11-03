package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestFrameShopTradeMissingTemplate pins the one-item trade rows: with no
// template for their item there is no row to write, so they fail. The list
// packets skip such an item instead (TestListPacketsSkipItemsWithNoTemplate).
func TestFrameShopTradeMissingTemplate(t *testing.T) {
	if _, err := FrameTradeOwnAdd(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeOwnAdd: want missing-template error")
	}
	if _, err := FrameTradeUpdate(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeUpdate: want missing-template error")
	}
}
