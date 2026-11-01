package pets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestObserverSeesWhatAPetWears pins the weapon slot and the name of the
// NpcInfo a pet's observers get: the weapon the pet holds in its right hand
// (not the template's), and an empty name, because only the owner's PetInfo
// carries the pet's own name.
func TestObserverSeesWhatAPetWears(t *testing.T) {
	t.Parallel()
	catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), petWeaponTemplate()))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(catalog)})
	wolf, _ := h.spawnWolf(t)
	runOnPetQueue(t, wolf, func() {
		inv := wolf.PetInventory()
		inst := inv.AddNew(fixturePetWeaponID, 1, 1_900_000)
		tmpl, _ := inv.Templates().Get(fixturePetWeaponID)
		if inst == nil || tmpl == nil {
			t.Error("pet weapon not added to the pet inventory")
			return
		}
		inv.SetPaperdollItem(itemcontainer.RHand, inst, tmpl)
	})

	var seen int
	for _, f := range enterWorldSeeing(t, h.srv, "player2", "Late") {
		if f[0] != serverpackets.OpcodeNPCInfo || int32(binary.LittleEndian.Uint32(f[1:5])) != wolf.ObjectID() {
			continue
		}
		seen++
		// The weapon follows the four collision doubles at 1+104; the name
		// string starts 17 bytes on, after the chest, the left hand and the
		// four flag bytes.
		if got := int32(binary.LittleEndian.Uint32(f[1+104:])); got != fixturePetWeaponID {
			t.Errorf("NpcInfo right hand = %d, want the pet weapon %d", got, fixturePetWeaponID)
		}
		r := wire.NewReader(f[1+121:])
		if name := r.ReadString(); name != "" {
			t.Errorf("NpcInfo name = %q, want empty", name)
		}
	}
	if seen != 1 {
		t.Fatalf("late viewer got %d NpcInfo of the pet, want 1", seen)
	}
}
