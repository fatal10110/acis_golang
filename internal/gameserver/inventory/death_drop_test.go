package inventory

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// TestDeathDroppable pins the reference's onDieDropItem item filter: an
// item that may not be dropped (augmented included), a shadow item, adena,
// a quest item and a template on the kept lists stay; any other droppable
// item may go.
func TestDeathDroppable(t *testing.T) {
	const (
		plainID    int32 = 100
		lockedID   int32 = 101
		shadowID   int32 = 102
		questID    int32 = 103
		keptID     int32 = 104
		templateID int32 = 105
	)
	etc := func(id int32, dropable bool, duration int32, typ item.EtcItemType) *item.Template {
		return &item.Template{ID: id, Kind: item.KindEtcItem, Dropable: dropable, Duration: duration, EtcItem: &item.EtcItemDetail{Type: typ}}
	}
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{
		etc(item.AdenaID, true, -1, item.EtcItemNone),
		etc(plainID, true, -1, item.EtcItemNone),
		etc(lockedID, false, -1, item.EtcItemNone),
		etc(shadowID, true, 60, item.EtcItemNone),
		etc(questID, true, -1, item.EtcItemQuest),
		etc(keptID, true, -1, item.EtcItemNone),
	}))
	kept := []int32{keptID}
	for _, tt := range []struct {
		name string
		inst *item.Instance
		want bool
	}{
		{"plain droppable item", &item.Instance{ObjectID: 1, TemplateID: plainID, Count: 1}, true},
		{"adena", &item.Instance{ObjectID: 2, TemplateID: item.AdenaID, Count: 100}, false},
		{"not droppable", &item.Instance{ObjectID: 3, TemplateID: lockedID, Count: 1}, false},
		{"augmented", &item.Instance{ObjectID: 4, TemplateID: plainID, Count: 1, Augmentation: &item.Augmentation{Attributes: 1}}, false},
		{"shadow item", &item.Instance{ObjectID: 5, TemplateID: shadowID, Count: 1}, false},
		{"quest item", &item.Instance{ObjectID: 6, TemplateID: questID, Count: 1}, false},
		{"kept template", &item.Instance{ObjectID: 7, TemplateID: keptID, Count: 1}, false},
		{"unknown template", &item.Instance{ObjectID: 8, TemplateID: templateID, Count: 1}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, got := DeathDroppable(inv, tt.inst, kept)
			if got != tt.want {
				t.Fatalf("DeathDroppable() = %v, want %v", got, tt.want)
			}
			if got && (tmpl == nil || tmpl.ID != tt.inst.TemplateID) {
				t.Fatalf("DeathDroppable() template = %+v, want template %d", tmpl, tt.inst.TemplateID)
			}
		})
	}
}
