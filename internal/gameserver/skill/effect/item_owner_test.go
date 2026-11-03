package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestCalculatorRemoveItemOwnerIgnoresTemplate removes an item's functions
// by its instance alone: given the template a reload swapped in, or none at
// all, they still go, while another instance of the same template keeps
// its own.
func TestCalculatorRemoveItemOwnerIgnoresTemplate(t *testing.T) {
	attached := &item.Template{ID: 30}
	worn, other := &item.Instance{ObjectID: 1, TemplateID: 30}, &item.Instance{ObjectID: 2, TemplateID: 30}
	for name, given := range map[string]*item.Template{"reloaded": {ID: 30}, "removed": nil, "same": attached} {
		t.Run(name, func(t *testing.T) {
			var c Calculator
			c.AddMod(Mod{Stat: stat.MaxHP, Op: OpAdd, Value: 100, Owner: ModOwnerItem(ItemOwner{Inst: worn, Tmpl: attached})})
			c.AddMod(Mod{Stat: stat.MaxHP, Op: OpAdd, Value: 7, Owner: ModOwnerItem(ItemOwner{Inst: other, Tmpl: attached})})
			c.AddMod(Mod{Stat: stat.MaxHP, Op: OpAdd, Value: 3, Owner: ModOwnerAugmentation(worn)})

			if got := c.RemoveOwner(ModOwnerItem(ItemOwner{Inst: worn, Tmpl: given})); got != 1 {
				t.Fatalf("RemoveOwner = %d, want the worn instance's one function", got)
			}
			if got := c.Calc(nil, 0, nil, nil); got != 10 {
				t.Fatalf("Calc() = %v, want 10: the other instance's and the augmentation's functions kept", got)
			}
		})
	}
}
