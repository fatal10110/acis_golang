package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fishUngradedLure is a GM-shop lure the grade table does not list.
const fishUngradedLure int32 = 6528

// TestFishingCastRefusesUngradedLure pins #5118: a cast baited with a lure
// that has no grade is refused with the bait notice before any lure is used
// up or the line is cast, so the fisher is never held in place: it walks
// off, and a second cast is refused the same way instead of reeling in.
func TestFishingCastRefusesUngradedLure(t *testing.T) {
	t.Parallel()
	catalog := item.NewTable(append(fishingCatalog().All(), &item.Template{
		ID: fishUngradedLure, Name: "Ungraded Lure", Kind: item.KindEtcItem, Slot: item.SlotLHand, Duration: -1,
		Stackable: true, Destroyable: true, DefaultAction: item.ActionEquip,
		EtcItem: &item.EtcItemDetail{Type: item.EtcItemLure},
	}))
	r := bootFishingSpec(t, fishingSpec{equipLure: true, actionLevel: 1, lure: fishUngradedLure}, gameservertest.WithItemTemplates(catalog))
	lure := r.lure
	if worn := r.srv.PlayerInventory(t, r.objID).ItemAt(itemcontainer.LHand); worn == nil || worn.ObjectID != lure {
		t.Fatalf("off hand holds %v, want the ungraded lure %d", worn, lure)
	}

	for _, what := range []string{"first cast", "second cast"} {
		requireEvents(t, what, r.cast(t, fishingSkill), append(castStart(fishingSkill), "msg 1454")...)
	}
	r.srv.FlushItems(t)
	if inst := mustFindItem(t, r.srv, r.objID, lure); inst.Count != 5 {
		t.Fatalf("ungraded lures after two refused casts = %d, want 5", inst.Count)
	}

	r.c.Send(encodeMoveBackwardToLocation(-200, 20, 30, 10, 20, 30))
	for _, f := range collectUntilQuiet(t, r.c) {
		if f[0] == serverpackets.OpcodeMoveToLocation {
			return
		}
	}
	t.Fatal("no MoveToLocation after a refused cast: the fisher is held in place")
}
