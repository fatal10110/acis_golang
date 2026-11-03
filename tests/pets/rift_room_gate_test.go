package pets

import (
	"os"
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// The first Dimensional Rift room of the datapack (spawnlist/16_12.xml):
// its maker, one of its fixed-position monsters, and points just inside
// and just outside the room's territory (x -112364..-111569, y
// -182829..-182040, z -6786..-6586), both well within aggro range.
const (
	riftRoomMaker = "dimension_gap_00"
	riftMonsterID = 21661
	riftControlID = 21662
)

var (
	riftMonsterPos = location.Location{X: -111736, Y: -182208, Z: -6768}
	riftInside     = location.Location{X: -111800, Y: -182300, Z: -6768}
	riftOutside    = location.Location{X: -111500, Y: -182208, Z: -6768}
)

// riftTemplate is an aggressive monster template of id.
func riftTemplate(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Level: 60, HPMax: 3000, AtkSpd: 300,
		RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, AggroRange: 1000,
	}
}

// riftRoom loads the datapack's rift spawnlist and returns the room's maker
// and a spawn table holding only that room with its one monster at
// riftMonsterPos, as the room's spawn entry declares it, <ai> values
// included. The table's maker has no type of its own, so the room spawns
// its monster at boot as the rift's room maker does on the quest's call.
func riftRoom(t *testing.T) (*spawn.Maker, *spawn.Table) {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "xml", "spawnlist", "16_12.xml"))
	if err != nil {
		t.Fatalf("read rift spawnlist: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "16_12.xml"), data, 0o644); err != nil {
		t.Fatalf("copy rift spawnlist: %v", err)
	}
	full, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load rift spawnlist: %v", err)
	}
	room, ok := full.Maker(riftRoomMaker)
	if !ok {
		t.Fatalf("rift spawnlist has no maker %s", riftRoomMaker)
	}
	var entry spawn.Entry
	found := false
	for _, e := range room.Entries {
		if e.NPCID == riftMonsterID && len(e.Positions) == 1 && e.Positions[0].Location == riftMonsterPos {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatalf("maker %s has no npc %d at %v", riftRoomMaker, riftMonsterID, riftMonsterPos)
	}
	if got := entry.AIParams["CreviceOfDiminsion"]; got != "1" {
		t.Fatalf("rift entry CreviceOfDiminsion = %q, want the datapack's 1", got)
	}
	only := spawn.NewMaker(spawn.Maker{
		Name: room.Name, Territories: room.Territories, BannedTerritories: room.BannedTerritories,
		MaximumNPCs: room.MaximumNPCs, Entries: []spawn.Entry{entry},
	}, 1)
	table, err := spawn.NewTable(room.Territories, []*spawn.Maker{only})
	if err != nil {
		t.Fatalf("rift room table: %v", err)
	}
	return room, table
}

// liveHostileOf returns the one live hostile NPC of template id.
func liveHostileOf(t *testing.T, srv *gameservertest.Server, id int) *npc.Hostile {
	t.Helper()
	var found []*npc.Hostile
	for _, obj := range srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == id {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		t.Fatalf("live hostile NPCs of template %d = %d, want 1", id, len(found))
	}
	return found[0]
}

// TestRiftRoomMonsterSparesPlayersOutsideItsRoom pins the rift-room
// exclusion of the automatic targeting rule: a monster whose spawn entry
// sets CreviceOfDiminsion above 0 never auto-attacks a player standing
// outside its spawn territory, nor that player's summon even when the
// summon stands in the room, and still auto-attacks a player in the room
// and that player's summon. A monster of the same room territory whose
// spawn sets no such value attacks either.
func TestRiftRoomMonsterSparesPlayersOutsideItsRoom(t *testing.T) {
	t.Parallel()
	room, table := riftRoom(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			wolfTemplate(), treeTemplate(), riftTemplate(riftMonsterID), riftTemplate(riftControlID),
		})),
		gameservertest.WithNpcSpawns(table),
	})
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	srv := h.srv

	rift := liveHostileOf(t, srv, riftMonsterID)
	control := srv.SpawnHostileNPCOfMakerAt(t, riftTemplate(riftControlID),
		location.Location{X: riftMonsterPos.X, Y: riftMonsterPos.Y - 150, Z: riftMonsterPos.Z}, room)

	owner, ownerChar := onlinePlayer(t, srv, h.ownerID), onlineCharacterOf(t, srv, h.ownerID)
	// place teleports the owner, whose summon follows it on Appearing, then
	// sets the summon down at petAt.
	place := func(ownerAt, petAt location.Location) {
		t.Helper()
		ownerChar.TeleportTo(ownerAt.X, ownerAt.Y, ownerAt.Z, 0)
		readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner teleport")
		drainUntilQuiet(t, h.client)
		h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
		srv.AdvanceUntil(t, "summon followed its owner", func() bool { return !pet.Teleporting() })
		drainUntilQuiet(t, h.client)
		if x, y, z := ownerChar.Position(); (location.Location{X: x, Y: y, Z: z}) != ownerAt {
			t.Fatalf("owner at %d,%d,%d after the teleport, want %v", x, y, z, ownerAt)
		}
		pet.Queue().Post(func() { pet.SetXYZ(petAt.X, petAt.Y, petAt.Z) })
		srv.Settle(t)
		if x, y, z := pet.Position(); (location.Location{X: x, Y: y, Z: z}) != petAt {
			t.Fatalf("summon at %d,%d,%d, want %v", x, y, z, petAt)
		}
	}

	for _, tc := range []struct {
		name      string
		ownerAt   location.Location
		petAt     location.Location
		riftWants bool
	}{
		{"owner and summon in the room", riftInside, riftInside, true},
		{"owner outside, summon in the room", riftOutside, riftInside, false},
		{"owner in the room, summon outside", riftInside, riftOutside, true},
		{"owner and summon outside", riftOutside, riftOutside, false},
	} {
		place(tc.ownerAt, tc.petAt)

		if got := rift.AutoAttackTargetValid(owner, 1000, false); got != tc.riftWants {
			t.Errorf("%s: rift monster auto-attacks the owner = %v, want %v", tc.name, got, tc.riftWants)
		}
		if got := rift.CanAutoAttack(owner); got != tc.riftWants {
			t.Errorf("%s: rift monster CanAutoAttack(owner) = %v, want %v", tc.name, got, tc.riftWants)
		}
		if got := rift.AutoAttackTargetValid(pet, 1000, false); got != tc.riftWants {
			t.Errorf("%s: rift monster auto-attacks the summon = %v, want %v", tc.name, got, tc.riftWants)
		}
		if !control.AutoAttackTargetValid(owner, 1000, false) || !control.AutoAttackTargetValid(pet, 1000, false) {
			t.Errorf("%s: a monster with no rift memo refuses the owner or the summon", tc.name)
		}
	}
}
