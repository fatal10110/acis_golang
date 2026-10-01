package npc

import "testing"

// TestFolkMerchantKinds pins the civilian types a buylist purchase or
// try-on may target: the merchant and every type built on it, the wyvern
// manager included through the castle chamberlain, each with its -bought
// page; a mercenary manager may buy but has no page, and a warehouse
// keeper is no merchant.
func TestFolkMerchantKinds(t *testing.T) {
	for _, tc := range []struct {
		kind      string
		merchant  bool
		mercenary bool
		page      string
	}{
		{"Merchant", true, false, "data/html/merchant/30001-bought.htm"},
		{"Fisherman", true, false, "data/html/fisherman/30001-bought.htm"},
		{"CastleChamberlain", true, false, "data/html/merchant/30001-bought.htm"},
		{"ClanHallManagerNpc", true, false, "data/html/merchant/30001-bought.htm"},
		{"ManorManagerNpc", true, false, "data/html/merchant/30001-bought.htm"},
		{"WyvernManagerNpc", true, false, "data/html/merchant/30001-bought.htm"},
		{"MercenaryManagerNpc", false, true, ""},
		{"WarehouseKeeper", false, false, ""},
	} {
		inst, err := NewInstance(1, &Template{ID: 30001, TemplateID: 30001, Type: tc.kind, Level: 1, HPMax: 100})
		if err != nil {
			t.Fatalf("%s: new instance: %v", tc.kind, err)
		}
		f, err := NewFolk(inst, false)
		if err != nil {
			t.Fatalf("%s: new folk: %v", tc.kind, err)
		}
		if got := f.Merchant(); got != tc.merchant {
			t.Errorf("%s: Merchant() = %v, want %v", tc.kind, got, tc.merchant)
		}
		if got := f.MercenaryManager(); got != tc.mercenary {
			t.Errorf("%s: MercenaryManager() = %v, want %v", tc.kind, got, tc.mercenary)
		}
		page, ok := f.BoughtPage()
		if page != tc.page || ok != (tc.page != "") {
			t.Errorf("%s: BoughtPage() = %q, %v, want %q", tc.kind, page, ok, tc.page)
		}
	}
}
