package items

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Catalog items of the kill-drop fixtures: a non-stackable weapon and a
// stackable potion.
const (
	killDropWeaponID int32 = 30
	killDropPotionID int32 = 20
)

// killDropNpc is a level-1 hostile of kind with one guaranteed roll per
// entry of drops (item id, count), each in a category of its own.
func killDropNpc(kind string, drops ...[2]int32) *npc.Template {
	tmpl := &npc.Template{
		ID: 25001, TemplateID: 25001, Name: "Greyclaw Kutus", Type: kind, Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	for _, d := range drops {
		kind := item.DropNormal
		if d[0] == item.AdenaID {
			kind = item.DropCurrency
		}
		tmpl.Drops = append(tmpl.Drops, item.DropCategory{Kind: kind, Chance: 100, Drops: []item.Drop{{ItemID: d[0], Min: d[1], Max: d[1], Chance: 100}}})
	}
	return tmpl
}

// killedForDrops is a kill of a drop fixture: the server, the killer, the
// dead npc and the frames the killer saw from the kill on.
type killedForDrops struct {
	srv           *gameservertest.Server
	killer, npcID int32
	frames        [][]byte
}

// killForDrops boots one player, spawns tmpl beside it and kills it with
// that player's hit.
func killForDrops(t *testing.T, tmpl *npc.Template, opts ...gameservertest.Option) killedForDrops {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)
	monster := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !monster.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the npc")
	}
	return killedForDrops{srv: srv, killer: objID, npcID: monster.ObjectID(), frames: collectUntilQuiet(t, c)}
}

// dropEvent is one DropItem (object, item, count) or one
// S1_DIED_DROPPED_S3_S2 (boss name, item, count) the player saw, in order.
type dropEvent struct {
	kind             string
	objectID, itemID int32
	count            int32
	name             string
}

func (e dropEvent) String() string {
	return fmt.Sprintf("%s(%s obj=%d item=%d count=%d)", e.kind, e.name, e.objectID, e.itemID, e.count)
}

// dropEvents picks the DropItem and S1_DIED_DROPPED_S3_S2 frames out of
// frames, decoding each; dropperID must be the dead npc for every DropItem.
func dropEvents(t *testing.T, frames [][]byte, dropperID int32) []dropEvent {
	t.Helper()
	var out []dropEvent
	for _, f := range frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeDropItem:
			if got := r.ReadInt32(); got != dropperID {
				t.Fatalf("DropItem dropper = %d, want the npc %d", got, dropperID)
			}
			e := dropEvent{kind: "drop", objectID: r.ReadInt32(), itemID: r.ReadInt32()}
			r.ReadInt32()
			r.ReadInt32()
			r.ReadInt32()
			r.ReadInt32() // stackable
			e.count = r.ReadInt32()
			out = append(out, e)
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() != serverpackets.SystemMessageS1DiedDroppedS3S2 {
				continue
			}
			if n := r.ReadInt32(); n != 3 {
				t.Fatalf("S1_DIED_DROPPED_S3_S2 carries %d parameters, want 3", n)
			}
			e := dropEvent{kind: "named"}
			if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
				t.Fatalf("parameter 1 type = %d, want text", typ)
			}
			e.name = r.ReadString()
			if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
				t.Fatalf("parameter 2 type = %d, want item name", typ)
			}
			e.itemID = r.ReadInt32()
			if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
				t.Fatalf("parameter 3 type = %d, want number", typ)
			}
			e.count = r.ReadInt32()
			out = append(out, e)
		}
	}
	return out
}

// TestKillDropSplitsNonStackableAndKeepsCategoriesApart pins
// Monster.doItemDrop and Npc.dropItem under the shipped MultipleItemDrop =
// True: a non-stackable weapon rolled 3 times falls as 3 count-1 ground
// items with their own object ids, and a potion rolled by two categories
// falls as two stacks, all in category order. An ordinary monster names
// none of it.
func TestKillDropSplitsNonStackableAndKeepsCategoriesApart(t *testing.T) {
	t.Parallel()
	tmpl := killDropNpc("Monster", [2]int32{killDropPotionID, 2}, [2]int32{killDropWeaponID, 3}, [2]int32{killDropPotionID, 5})
	k := killForDrops(t, tmpl)
	ground := k.srv.GroundItems.Snapshots(nil)
	if len(ground) != 5 {
		t.Fatalf("ground items = %+v, want 5", ground)
	}
	events := dropEvents(t, k.frames, k.npcID)
	want := []dropEvent{
		{kind: "drop", itemID: killDropPotionID, count: 2},
		{kind: "drop", itemID: killDropWeaponID, count: 1},
		{kind: "drop", itemID: killDropWeaponID, count: 1},
		{kind: "drop", itemID: killDropWeaponID, count: 1},
		{kind: "drop", itemID: killDropPotionID, count: 5},
	}
	if len(events) != len(want) {
		t.Fatalf("drop frames = %v, want %v", events, want)
	}
	seen := map[int32]bool{}
	for i := range events {
		if id := events[i].objectID; id == 0 || seen[id] {
			t.Fatalf("drop %d object id %d is not a fresh one: %v", i, id, events)
		} else {
			seen[id] = true
		}
		want[i].objectID = events[i].objectID
	}
	if !slices.Equal(events, want) {
		t.Fatalf("drop frames = %v, want %v", events, want)
	}
	for _, g := range ground {
		if g.TemplateID == killDropWeaponID && g.Count != 1 {
			t.Fatalf("non-stackable ground item %+v holds %d", g, g.Count)
		}
		if !seen[g.ObjectID] {
			t.Fatalf("ground item %+v was never shown dropping", g)
		}
	}
}

// TestGrandBossKillNamesEveryDrop pins the raid branches of the drop path
// for a GrandBoss: each dropped item is named with S1_DIED_DROPPED_S3_S2
// (boss name, item, whole count) right after it lands, and AutoLootRaid,
// not AutoLoot, decides whether the killer loots it, naming it all the
// same.
func TestGrandBossKillNamesEveryDrop(t *testing.T) {
	t.Parallel()
	drops := [][2]int32{{killDropWeaponID, 2}, {item.AdenaID, 100}}

	t.Run("ground", func(t *testing.T) {
		t.Parallel()
		k := killForDrops(t, killDropNpc("GrandBoss", drops...), gameservertest.WithAutoLoot(true))
		ground := k.srv.GroundItems.Snapshots(nil)
		if len(ground) != 3 {
			t.Fatalf("ground items = %+v, want 2 weapons and the adena: AutoLoot does not apply to a grand boss", ground)
		}
		for _, g := range ground {
			if g.OwnerID != k.killer {
				t.Fatalf("ground item %+v reserved to %d, want the killer %d", g, g.OwnerID, k.killer)
			}
		}
		events := dropEvents(t, k.frames, k.npcID)
		want := []dropEvent{
			{kind: "drop", itemID: killDropWeaponID, count: 1},
			{kind: "drop", itemID: killDropWeaponID, count: 1},
			{kind: "named", name: "Greyclaw Kutus", itemID: killDropWeaponID, count: 2},
			{kind: "drop", itemID: item.AdenaID, count: 100},
			{kind: "named", name: "Greyclaw Kutus", itemID: item.AdenaID, count: 100},
		}
		for i := range events {
			if i < len(want) {
				want[i].objectID = events[i].objectID
			}
		}
		if !slices.Equal(events, want) {
			t.Fatalf("drop frames = %v, want %v", events, want)
		}
	})

	t.Run("auto-loot raid", func(t *testing.T) {
		t.Parallel()
		k := killForDrops(t, killDropNpc("GrandBoss", drops...), gameservertest.WithAutoLootRaid(true))
		if ground := k.srv.GroundItems.Snapshots(nil); len(ground) != 0 {
			t.Fatalf("ground items = %+v, want none: AutoLootRaid loots a grand boss", ground)
		}
		if got := carriedCount(t, k.srv, k.killer, item.AdenaID); got != 100 {
			t.Fatalf("carried adena = %d, want 100", got)
		}
		events := dropEvents(t, k.frames, k.npcID)
		want := []dropEvent{
			{kind: "named", name: "Greyclaw Kutus", itemID: killDropWeaponID, count: 2},
			{kind: "named", name: "Greyclaw Kutus", itemID: item.AdenaID, count: 100},
		}
		if !slices.Equal(events, want) {
			t.Fatalf("drop frames = %v, want %v", events, want)
		}
	})
}
