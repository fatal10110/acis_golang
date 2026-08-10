package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestUseSummonItemSpawnsDecorativeNPCAndPreventsNearbyDuplicate(t *testing.T) {
	link, state := newSummonTestLink(t)
	link.ids = &fakeSummonIDs{next: 100}
	link.npcs = npc.NewTable([]*npc.Template{{
		ID: 13006, TemplateID: 13006, Type: "ChristmasTree", Name: "Tree",
		AtkSpd: 300, WalkSpeed: 30, RunSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}})
	items, err := item.NewSummonItemTable([]item.SummonItem{{
		ItemID: summonTestCollarTemplateID, NPCID: 13006, SummonType: summonItemTypeDecorative,
	}})
	if err != nil {
		t.Fatalf("build summon item table: %v", err)
	}
	link.summonItems = items

	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 100, 200, 300, 400)
	first := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{first})

	if !link.useSummonItem(live, live.Inventory(), first) {
		t.Fatal("useSummonItem returned false, want handled decorative item")
	}
	if got := live.Inventory().ItemByObjectID(first.ObjectID); got != nil {
		t.Fatalf("decorative item still in inventory: %+v", got)
	}
	obj, ok := state.Object(101)
	if !ok {
		t.Fatal("decoration not registered in world")
	}
	decoration, ok := obj.(*npc.Decoration)
	if !ok {
		t.Fatalf("world object = %T, want *npc.Decoration", obj)
	}
	if got := decoration.NPCInfoSnapshot(); got.Attackable || got.Running || got.Title != "Player" || got.X != 100 || got.Y != 200 || got.Z != 300 || got.Heading != 400 {
		t.Fatalf("decoration snapshot = %+v", got)
	}
	assertOpcodeSequence(t, frames.frames, serverpackets.OpcodeNPCInfo)

	frames.frames = nil
	second := &item.Instance{ObjectID: 501, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{second})
	if !link.useSummonItem(live, live.Inventory(), second) {
		t.Fatal("useSummonItem returned false, want handled duplicate rejection")
	}
	if got := live.Inventory().ItemByObjectID(second.ObjectID); got == nil {
		t.Fatal("duplicate rejection consumed the item")
	}
	if len(frames.frames) != 1 {
		t.Fatalf("frames = %d, want one duplicate rejection", len(frames.frames))
	}
	assertSystemMessageStringFrame(t, frames.frames[0], 1142, "Tree")
}
