package pets

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func petCond(kind string, attrs map[string]string, children ...item.Condition) item.Condition {
	return item.Condition{Kind: kind, Attrs: attrs, Children: children}
}

// TestPetUseConditionsFollowConditionReader pins a pet's item use to the
// reader skill conditions use, tested with the level-10 wolf as both sides:
// <and> drops a <game chance> child, an unrecognized attribute beside level
// is ignored, <not> reads only its first child, and a player-only check
// fails for a pet. A failing clause answers PET_CANNOT_USE_ITEM whatever its
// own message; a <not> around no condition refuses the use with nothing
// sent, since the reference's check throws before it sends anything.
func TestPetUseConditionsFollowConditionReader(t *testing.T) {
	t.Parallel()
	gameChance := petCond("game", map[string]string{"chance": "50"})
	level := func(n int) item.Condition {
		return petCond("player", map[string]string{"level": strconv.Itoa(n)})
	}
	const (
		equips = iota
		refused
		silent
	)
	for _, tt := range []struct {
		name string
		root item.Condition
		want int
	}{
		{"and drops game chance", petCond("and", nil, gameChance, level(wolfLevel)), equips},
		{"and drops game chance, level too high", petCond("and", nil, gameChance, level(wolfLevel+1)), refused},
		{"unrecognized attribute beside level", petCond("player", map[string]string{"bogus": "1", "level": "1"}), equips},
		{"not reads its first child only", petCond("not", nil, level(99), level(1)), equips},
		{"player-only check", petCond("player", map[string]string{"sex": "0"}), refused},
		{"not of game chance", petCond("not", nil, gameChance), silent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, wolf, weaponID := bootConditionedWolfWeapon(t, item.UseCondition{Root: tt.root, MessageID: serverpackets.SystemMessageCannotEquipItemDueToBadCondition})

			frames := h.sendAndTick(t, encodeRequestPetUseItem(weaponID))
			switch tt.want {
			case equips:
				if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate}; !slices.Equal(got, want) {
					t.Fatalf("%s: frames = %x, want PET_PUT_ON_S1 then PetInventoryUpdate", tt.name, got)
				}
				assertSystemMessageItem(t, frames[0], serverpackets.SystemMessagePetPutOnS1, fixturePetWeaponID)
			case refused:
				if len(frames) != 1 {
					t.Fatalf("%s: frames = %x, want PET_CANNOT_USE_ITEM alone", tt.name, frameOpcodes(frames))
				}
				findSystemMessage(t, frames, serverpackets.SystemMessagePetCannotUseItem)
			case silent:
				if len(frames) != 0 {
					t.Fatalf("%s: frames = %x, want none", tt.name, frameOpcodes(frames))
				}
			}
			held := wolf.PetInventory().ItemAt(itemcontainer.RHand)
			if worn := held != nil && held.ObjectID == weaponID; worn != (tt.want == equips) {
				t.Fatalf("%s: weapon worn = %v, want %v", tt.name, worn, tt.want == equips)
			}
		})
	}
}

// bootConditionedWolfWeapon is bootArmedWolf with uc as the wolf weapon's
// only use condition.
func bootConditionedWolfWeapon(t *testing.T, uc item.UseCondition) (*petWorld, *summon.Actor, int32) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.ID = armedWolfNPCID
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: armedWolfNPCID, SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	weaponTmpl := petWeaponTemplate()
	weaponTmpl.UseConditions = []item.UseCondition{uc}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf})),
		gameservertest.WithSummonItems(summonItems),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), weaponTmpl))),
	})
	weapon := item.Instance{ObjectID: h.srv.NewObjectID(), TemplateID: fixturePetWeaponID, OwnerID: h.collarID, Count: 1, Location: item.LocationPet}
	if err := h.srv.Items.Create(context.Background(), h.collarID, weapon); err != nil {
		t.Fatalf("seed pet weapon: %v", err)
	}
	actor, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	return h, actor, weapon.ObjectID
}
