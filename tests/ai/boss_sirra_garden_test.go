package ai

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sirra"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The garden sculpture, a civilian with no script standing beside it, and
// the skill the sculpture casts.
const (
	sculptureID         = int32(32030)
	sculptureNeighborID = int32(32032)
	sculptureResistCold = int32(4479)
)

// sculptureOutside lies north of the garden's spawn territory, still in sight
// of the sculpture.
var sculptureOutside = location.Location{X: 115451, Y: -125456, Z: -3388}

// sculptureSpawnlist is the sculpture's npcmaker over a territory holding
// sirraAtDeacon, where the character enters the world, and a civilian 50
// north of the sculpture.
const sculptureSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="garden_territory" minZ="-3488" maxZ="-3288">
		<node x="115351" y="-125806"/>
		<node x="115551" y="-125806"/>
		<node x="115551" y="-125606"/>
		<node x="115351" y="-125606"/>
	</territory>
	<npcmaker name="sculpture_garden_maker" territory="garden_territory" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="32030" pos="115451;-125706;-3388;0" total="1" respawn="1min"/>
	</npcmaker>
	<npcmaker name="garden_neighbor" territory="garden_territory" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="32032" pos="115451;-125656;-3388;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// bootSculpture boots the sculpture's garden with the character entering the
// world inside the sculpture's spawn territory, the global memo empty, and
// the sculpture as scripts.xml lists it.
func bootSculpture(t *testing.T) *sirraWorld {
	t.Helper()
	w := &sirraWorld{baiumWorld: &baiumWorld{start: time.Now()}, log: &sailrenLog{}}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "garden.xml"), []byte(sculptureSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[int32]script.NPCKind{sculptureID: script.KindFolk, sculptureNeighborID: script.KindFolk}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.sirra.SculptureGarden"},
		{Path: "test." + sirraDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":      defaultnpc.New,
		"script.ai.boss.sirra.SculptureGarden": sirra.SculptureGarden,
		"test." + sirraDirector:                func() script.Script { return script.Script{} },
	}
	resistCold := modelskill.Definition{
		ID: modelskill.ID(sculptureResistCold), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", HitTime: 4000,
	}
	db := sqltest.SharedDB(t)
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, resistCold})
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{sirraAtDeacon}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", int(sculptureID)), gameservertest.FolkTemplate("Folk", int(sculptureNeighborID)),
		})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSkills(skills), gameservertest.WithNpcSpawnSkills(skills),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
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
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	w.warriorWorld = &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
	return w
}

// memo7 returns the global memo's value 7.
func (w *sirraWorld) memo7(t *testing.T) int {
	t.Helper()
	var v int
	w.srv.RunScript(t, w.player.ObjectID(), sirraDirector, func(s *script.Script, _ *script.Player) {
		v = s.GlobalMemo().Int("7", 0)
	})
	return v
}

// sculptureEvent sends the sculpture n script event eventID, from the
// director on the character's queue, and returns the frames the character
// was sent.
func (w *sirraWorld) sculptureEvent(t *testing.T, n attackable.Combatant, eventID int32) [][]byte {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), sirraDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(n), eventID, 0, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// sculptureCasts returns the targets of the sculpture's casts among frames,
// failing unless each is Resist Cold with its 4000 ms hit time, and unless
// no cast of it is launched.
func sculptureCasts(t *testing.T, frames [][]byte, garden int32) []int32 {
	t.Helper()
	var targets []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, garden) {
		field := func(i int) int32 { return int32(binary.LittleEndian.Uint32(f[1+4*i:])) }
		if id, level, hitTime := field(2), field(3), field(4); id != sculptureResistCold || level != 1 || hitTime != 4000 {
			t.Fatalf("MagicSkillUse skill %d-%d hit time %d, want %d-1 4000", id, level, hitTime, sculptureResistCold)
		}
		targets = append(targets, field(1))
	}
	if launched := framesOf(frames, serverpackets.OpcodeMagicSkillLaunched, garden); len(launched) != 0 {
		t.Fatalf("MagicSkillLaunched frames = %d, want none", len(launched))
	}
	return targets
}

// The first sculpture created while global memo 7 names no creature
// stores its object id there. One created while it names a sculpture
// still in the world leaves it be; one created after that sculpture left
// the world takes it over, though the second is still about.
func TestSculptureGardenTakesTheMemoWhenItNamesNoCreature(t *testing.T) {
	t.Parallel()
	w := bootSculpture(t)
	first := soleBaiumNPC(t, w.baiumWorld, sculptureID)
	if got := w.memo7(t); got != int(first.ObjectID()) {
		t.Fatalf("after the first sculpture: memo 7 = %d, want its object id %d", got, first.ObjectID())
	}

	if _, err := w.srv.NpcSpawns.AddSpawn(sculptureID, w.at.X+300, w.at.Y, w.at.Z, 0, false, 0); err != nil {
		t.Fatalf("spawn %d: %v", sculptureID, err)
	}
	w.srv.Settle(t)
	if got := w.memo7(t); got != int(first.ObjectID()) {
		t.Fatalf("after a second sculpture: memo 7 = %d, want the first's %d", got, first.ObjectID())
	}

	w.srv.RunScript(t, w.player.ObjectID(), sirraDirector, func(_ *script.Script, _ *script.Player) {
		script.NPCOf(first).DeleteMe()
	})
	w.srv.Settle(t)
	third, err := w.srv.NpcSpawns.AddSpawn(sculptureID, w.at.X-300, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", sculptureID, err)
	}
	w.srv.Settle(t)
	if got := w.memo7(t); got != int(third.ObjectID()) {
		t.Fatalf("after the first sculpture left: memo 7 = %d, want the third's %d", got, third.ObjectID())
	}
}

// Script event 10027 sends every player standing in the sculpture's spawn
// territory to 115792,-125760,-3373, up to 200 away on each axis; a
// player outside it stays.
func TestSculptureGardenTeleportsThePlayersInItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootSculpture(t)
	garden := soleBaiumNPC(t, w.baiumWorld, sculptureID)
	mate := w.login(t, "mateacc", "Mate", 0)
	w.place(t, mate.id, mate.c, sculptureOutside)

	w.sculptureEvent(t, garden, 10027)
	w.srv.ReadQueued(t, mate.c)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if abs(x-115792) > 200 || abs(y+125760) > 200 || abs(z+3373) > 200 {
		t.Fatalf("character at %d,%d,%d, want within 200 of 115792,-125760,-3373", x, y, z)
	}
	if x, y, _ := w.srv.PlayerPosition(t, mate.id); x != sculptureOutside.X || y != sculptureOutside.Y {
		t.Fatalf("Mate outside the territory moved to %d,%d", x, y)
	}
}

// Script event 11038 makes the sculpture look around, at most once per
// 30 s, remembering the player it sees but not the civilian beside it, then cast Resist Cold on
// each member of that player's party standing in its spawn territory,
// shown as one MagicSkillUse each and nothing launched. A player in no
// party gets nothing, nor does a member outside the territory; other
// events do nothing.
func TestSculptureGardenCastsResistColdOnThePartyInItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootSculpture(t)
	garden := soleBaiumNPC(t, w.baiumWorld, sculptureID)
	seen := func() script.Creature { return script.NPCOf(garden).Slot(script.SlotCreature0) }
	if seen() != nil {
		t.Fatal("setup: the sculpture remembers a creature before looking around")
	}

	away := location.Location{X: sirraAtDeacon.X + 3000, Y: sirraAtDeacon.Y, Z: sirraAtDeacon.Z}
	w.place(t, w.player.ObjectID(), w.srv.Client, away)
	if got := sculptureCasts(t, w.sculptureEvent(t, garden, 11038), garden.ObjectID()); len(got) != 0 {
		t.Fatalf("only the civilian in sight: casts on %v, want none", got)
	}
	if seen() != nil {
		t.Fatalf("with only the civilian in sight the sculpture remembers %v, want nothing", seen())
	}

	w.advance(t, 30*time.Second)
	w.place(t, w.player.ObjectID(), w.srv.Client, sirraAtDeacon)
	if seen() != nil {
		t.Fatal("setup: the sculpture remembers a creature before looking around again")
	}
	if got := sculptureCasts(t, w.sculptureEvent(t, garden, 11038), garden.ObjectID()); len(got) != 0 {
		t.Fatalf("no party: casts on %v, want none", got)
	}
	if p, ok := seen().(*script.Player); !ok || p.ObjectID() != w.player.ObjectID() {
		t.Fatalf("after looking around the sculpture remembers %v, want the character", seen())
	}

	inside := w.join(t, "insideacc", "Inside", 0)
	outside := w.join(t, "outsideacc", "Outside", 0)
	w.place(t, outside.id, outside.c, sculptureOutside)
	got := sculptureCasts(t, w.sculptureEvent(t, garden, 11038), garden.ObjectID())
	if want := []int32{w.player.ObjectID(), inside.id}; !slices.Equal(got, want) {
		t.Fatalf("party: casts on %v, want %v", got, want)
	}
	if got := sculptureCasts(t, w.sculptureEvent(t, garden, 10001), garden.ObjectID()); len(got) != 0 {
		t.Fatalf("event 10001: casts on %v, want none", got)
	}
}
