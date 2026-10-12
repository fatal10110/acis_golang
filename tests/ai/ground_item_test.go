package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The ground-item scenarios' fixtures: a hostile and a civilian NPC that
// walk, both bound to the picker behavior, and the items they find.
const (
	pickerHostileID = int32(29095)
	pickerFolkID    = int32(35600)
	potionID        = int32(20)
	soulshotID      = int32(1463)
	zaricheItemID   = int32(8190)
)

// picker is a behavior that, told about ground items, logs them and asks
// its NPC to pick each one up, and logs each item it picks up.
func picker(log *folkLog) script.Catalog {
	return script.Catalog{"ai.GroundPicker": func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{pickerHostileID, pickerFolkID}, Hooks: script.Hooks{
			OnSeeItem: func(_ *script.Script, e script.SeeItem) {
				var seen []string
				for _, it := range e.Items {
					seen = append(seen, fmt.Sprintf("%d/%d", it.ObjectID, it.ItemID))
				}
				slices.Sort(seen)
				log.add("see %d q=%d %v", e.NPC.NpcID(), e.Quantity, seen)
				for _, it := range e.Items {
					e.NPC.AddPickUpDesire(it.ObjectID, 1_000_000)
				}
			},
			OnPickedItem: func(_ *script.Script, e script.PickedItem) {
				log.add("picked %d %d/%d", e.NPC.NpcID(), e.Item.ObjectID, e.Item.ItemID)
			},
		}}
	}}
}

// pickerTemplate is a walking fixture NPC of kind typ.
func pickerTemplate(id int32, typ string) *npc.Template {
	return &npc.Template{
		ID: int(id), TemplateID: int(id), Type: typ, Name: "Picker", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10, CollisionHeight: 20,
	}
}

// groundWorld is one character in the world beside the picker NPCs.
type groundWorld struct {
	srv *gameservertest.Server
	at  location.Location
	log *folkLog
}

// bootGround boots one character in the world with the AI task, the picker
// behavior and Zariche as a cursed weapon.
func bootGround(t *testing.T) *groundWorld {
	t.Helper()
	log := &folkLog{}
	catalog := picker(log)
	zariche, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: zaricheItemID, Skill: modelskill.Ref{ID: 3603, Level: 1}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{pickerTemplate(pickerHostileID, "Monster"), pickerTemplate(pickerFolkID, "Folk")})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithCursedWeapons(zariche),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{pickerHostileID: script.KindHostile, pickerFolkID: script.KindFolk},
			[]script.Listing{{Path: "ai.GroundPicker"}}, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	return &groundWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, log: log}
}

// spawn places the NPC id dx along x from the character.
func (w *groundWorld) spawn(t *testing.T, id int32, dx int) any {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return n
}

// drop lays one itemID dx along x from the character and returns it.
func (w *groundWorld) drop(t *testing.T, itemID int32, dx int) *grounditem.Item {
	t.Helper()
	w.srv.SeedGroundItem(t, 0, itemID, 1, w.at.X+dx, w.at.Y, w.at.Z)
	w.srv.ReadQueued(t, w.srv.Client)
	var found *grounditem.Item
	for _, obj := range w.srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == itemID {
			if x, _, _ := g.Position(); x == w.at.X+dx {
				found = g
			}
		}
	}
	if found == nil {
		t.Fatalf("no item %d on the ground", itemID)
	}
	return found
}

// look runs the NPC's look for ground items on its own queue.
func (w *groundWorld) look(t *testing.T, queue interface{ Post(func()) bool }, n *script.NPC, radius int, ids ...int32) {
	t.Helper()
	if !queue.Post(func() { n.LookItem(radius, 1, ids...) }) {
		t.Fatal("the NPC's queue is closed")
	}
	w.srv.Settle(t)
}

// walk lets the NPC's walk run until it arrives.
func (w *groundWorld) walk(t *testing.T, moving func() bool) {
	t.Helper()
	for i := 0; moving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.ReadQueued(t, w.srv.Client)
}

func line(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// TestNPCSeesAndPicksUpGroundItem runs a hostile NPC whose behavior looks
// for ground items and picks each one it is told about up. A look with an
// id filter tells the behavior once, with the quantity and the matching
// items only; a look with no ids tells it about every item in range; a look
// that matches nothing tells it nothing. The pick-up desire walks the NPC to
// the item, then its picked-item hook runs once and the item leaves the
// world, which the character sees as DeleteObject.
func TestNPCSeesAndPicksUpGroundItem(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	h := w.spawn(t, pickerHostileID, 200).(*npc.Hostile)
	n := script.NPCOf(h)
	potion := w.drop(t, potionID, 500)
	shot := w.drop(t, soulshotID, 450)

	w.look(t, h.Queue(), n, 500, zaricheItemID)
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a look matching nothing told %q", got)
	}
	if h.AI().Desires().Len() != 0 {
		t.Fatal("a look matching nothing queued a desire")
	}

	w.look(t, h.Queue(), n, 500, potionID, zaricheItemID)
	if want := []string{line("see %d q=1 [%d/%d]", pickerHostileID, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("filtered look: want %q", want)
	}

	// The first tick lets the NPC's life begin; the next takes up the
	// pick-up and walks toward the item.
	cycle(t, w.srv)
	frames := cycle(t, w.srv)
	moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, h.ObjectID())
	if len(moves) != 1 {
		t.Fatalf("walks toward the item = %d, want 1", len(moves))
	}
	if got := moveDestination(moves[0]); got.X != w.at.X+500 || got.Y != w.at.Y {
		t.Fatalf("walk destination = %+v, want the item at x %d", got, w.at.X+500)
	}
	if got := h.AI().CurrentIntention(); got != actorai.IntentionPickUp {
		t.Fatalf("intention = %v, want %v", got, actorai.IntentionPickUp)
	}
	w.walk(t, h.IsMoving)
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("the arrival told %q, want nothing before the next think", got)
	}

	frames = cycle(t, w.srv)
	if want := []string{line("picked %d %d/%d", pickerHostileID, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("pickup: want %q", want)
	}
	if len(framesOf(frames, serverpackets.OpcodeDeleteObject, potion.ObjectID())) != 1 {
		t.Fatal("the character saw no DeleteObject of the picked item")
	}
	if _, ok := w.srv.State.Object(potion.ObjectID()); ok {
		t.Fatal("the picked item is still in the world")
	}
	if h.AI().Desires().Len() != 0 {
		t.Fatalf("desires after the pickup = %d, want none", h.AI().Desires().Len())
	}
	cycle(t, w.srv)
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a later tick told %q, want the pickup once", got)
	}

	// No ids: every item in range, here the soulshot that is left.
	w.look(t, h.Queue(), n, 500)
	if want := []string{line("see %d q=1 [%d/%d]", pickerHostileID, shot.ObjectID(), soulshotID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("unfiltered look: want %q", want)
	}
}

// TestNPCPickUpOfGoneItem: an item that leaves the ground before the NPC
// reaches it raises no picked-item hook; the desire stays queued, its step
// doing nothing.
func TestNPCPickUpOfGoneItem(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	h := w.spawn(t, pickerHostileID, 200).(*npc.Hostile)
	potion := w.drop(t, potionID, 500)
	w.look(t, h.Queue(), script.NPCOf(h), 500, potionID)
	w.log.take()
	w.srv.DespawnGroundItem(t, potion.ObjectID())

	var frames [][]byte
	for range 3 {
		frames = append(frames, cycle(t, w.srv)...)
	}
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a gone item told %q", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeMoveToLocation, h.ObjectID())) != 0 {
		t.Fatal("the NPC walked toward a gone item")
	}
	if !h.AI().Desires().Has(&actorai.Desire{Kind: actorai.IntentionPickUp, ItemObjectID: potion.ObjectID()}) {
		t.Fatal("the pick-up desire of a gone item left the queue")
	}
}

// TestNPCPicksUpCursedWeapon: a cursed weapon lying on the ground ends its
// life when an NPC picks it up: after the picked-item hook it leaves the
// world, and every player is told it disappeared.
func TestNPCPicksUpCursedWeapon(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	h := w.spawn(t, pickerHostileID, 200).(*npc.Hostile)
	// Zariche drops as a monster kill drops it: its life starts, then it is
	// laid down.
	if id, ok := w.srv.CursedWeapons.RollDrop(func(int) int { return 0 }); !ok || id != zaricheItemID {
		t.Fatalf("RollDrop = %d, %v; want Zariche", id, ok)
	}
	zariche := w.drop(t, zaricheItemID, 215)
	x, y, z := zariche.Position()
	w.srv.CursedWeapons.PlaceOnGround(zaricheItemID, zariche.ObjectID(), location.Location{X: x, Y: y, Z: z})

	w.look(t, h.Queue(), script.NPCOf(h), 500, zaricheItemID, 8689)
	if want := []string{line("see %d q=1 [%d/%d]", pickerHostileID, zariche.ObjectID(), zaricheItemID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("look: want %q", want)
	}
	// The item lies within 36 of the NPC: the second tick picks it up.
	cycle(t, w.srv)
	frames := cycle(t, w.srv)
	if want := []string{line("picked %d %d/%d", pickerHostileID, zariche.ObjectID(), zaricheItemID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("pickup: want %q", want)
	}
	deleted := slices.IndexFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeDeleteObject && int32(binary.LittleEndian.Uint32(f[1:5])) == zariche.ObjectID()
	})
	told := slices.IndexFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeSystemMessage && int32(binary.LittleEndian.Uint32(f[1:5])) == serverpackets.SystemMessageS1HasDisappeared &&
			len(f) >= 17 && int32(binary.LittleEndian.Uint32(f[9:13])) == serverpackets.SystemMessageParamItemName &&
			int32(binary.LittleEndian.Uint32(f[13:17])) == zaricheItemID
	})
	if deleted < 0 || told < 0 || told < deleted {
		t.Fatalf("DeleteObject at %d, S1_HAS_DISAPPEARED of Zariche at %d; want both, the removal first", deleted, told)
	}
	if _, ok := w.srv.State.Object(zariche.ObjectID()); ok {
		t.Fatal("Zariche still lies in the world")
	}
	if active := w.srv.CursedWeapons.Active(); len(active) != 0 {
		t.Fatalf("weapons out = %+v, want none", active)
	}
}

// TestFolkPicksUpGroundItem: a civilian NPC's behavior sees and picks up a
// ground item as a hostile one's does.
func TestFolkPicksUpGroundItem(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	f := w.spawn(t, pickerFolkID, 200).(*npc.Folk)
	potion := w.drop(t, potionID, 500)
	w.look(t, f.Queue(), script.NPCOf(f), 500, potionID)
	if want := []string{line("see %d q=1 [%d/%d]", pickerFolkID, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("look: want %q", want)
	}

	cycle(t, w.srv)
	frames := cycle(t, w.srv)
	moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, f.ObjectID())
	if len(moves) != 1 || moveDestination(moves[0]).X != w.at.X+500 {
		t.Fatalf("walks toward the item = %d, want 1 to x %d", len(moves), w.at.X+500)
	}
	w.walk(t, f.IsMoving)
	frames = cycle(t, w.srv)
	if want := []string{line("picked %d %d/%d", pickerFolkID, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("pickup: want %q", want)
	}
	if len(framesOf(frames, serverpackets.OpcodeDeleteObject, potion.ObjectID())) != 1 {
		t.Fatal("the character saw no DeleteObject of the picked item")
	}
	if got := f.CurrentIntention(); got != actorai.IntentionPickUp {
		t.Fatalf("intention after the pickup = %v, want %v kept", got, actorai.IntentionPickUp)
	}
}
