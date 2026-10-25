package ai

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/antharas"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Antharas (its small size), the Behemoth Dragon it calls, the cube it
// leaves, and a fixture monster that hits it.
const (
	antharasID         = int32(29066)
	antharasBehemothID = int32(29069)
	antharasCubeID     = int32(31859)
	antharasHitterID   = int32(29099)
	antharasRow        = "antaras_min"
	antharasDirector   = "AntharasDirector"
)

// antharasHome is where Antharas's spawn places it, inside its lair's
// territory; the character enters the world 1000 due west of it.
var (
	antharasHome      = location.Location{X: 184500, Y: 114835, Z: -7678}
	antharasCharacter = location.Location{X: 183500, Y: 114835, Z: -7678}
)

// antharasSpawnlist is the shipped lair territory and Antharas's npcmaker
// with its GM_ID 1, placing it at antharasHome and respawning it after
// the given time.
const antharasSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="antaras_2521_01" minZ="-7712" maxZ="-6212">
		<node x="175201" y="110789"/>
		<node x="179519" y="110645"/>
		<node x="187612" y="113814"/>
		<node x="187652" y="115788"/>
		<node x="179659" y="118982"/>
		<node x="175206" y="118934"/>
		<node x="173003" y="114720"/>
	</territory>
	<npcmaker name="antaras_2521_01" territory="antaras_2521_01" maximumNpcs="89">
		<ai type="default_maker"/>
		<npc id="29066" pos="184500;114835;-7678;0" total="1" respawn="%s" respawnRand="0sec" dbName="antaras_min" dbSaving="DEATH_TIME;PARAMETERS;POS">
			<ai>
				<set name="GM_ID" val="1"/>
			</ai>
		</npc>
	</npcmaker>
</list>`

// antharasSkills are the skills Antharas casts, as fixture definitions it
// can always pay for.
func antharasSkills() *modelskill.Table {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{4106, 4107, 4108, 4110, 4111, 4112, 4113, 4125, 4239, 4240, 4241, 4258} {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		})
	}
	return modelskill.NewTable(defs)
}

// bootAntharas boots Antharas's lair backed by the spawn_data table, after
// saving row as Antharas's live row when it is set, Antharas respawning
// after respawn, every script draw 0, and the character entering the world
// at antharasCharacter. The world is a baiumWorld: the same character and
// in-game clock.
func bootAntharas(t *testing.T, row *spawn.State, respawn string) *baiumWorld {
	t.Helper()
	if row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{antharasRow: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	w := &baiumWorld{start: time.Now()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "antharas.xml"), []byte(fmt.Sprintf(antharasSpawnlist, respawn)), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := warriorTemplate(antharasID, nil)
	boss.Type, boss.Name, boss.Level, boss.HPMax = "GrandBoss", "Antharas", 79, 1000000
	boss.AggroRange, boss.CanSeeThrough = 2000, true
	boss.RunSpeed, boss.WalkSpeed = 1000, 1000
	behemoth := warriorTemplate(antharasBehemothID, nil)
	behemoth.Name = "Behemoth Dragon"
	hitter := warriorTemplate(antharasHitterID, nil)
	kinds := map[int32]script.NPCKind{
		antharasID: script.KindHostile, antharasBehemothID: script.KindHostile, antharasHitterID: script.KindHostile, antharasCubeID: script.KindFolk,
	}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.boss.antharas.Antharas"}, {Path: "test." + antharasDirector}}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":  defaultnpc.New,
		"script.ai.boss.antharas.Antharas": antharas.Antharas,
		"test." + antharasDirector:         func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{antharasCharacter}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{boss, behemoth, hitter, gameservertest.FolkTemplate("Folk", int(antharasCubeID))})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(antharasSkills()),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
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

// antharasLiveRow is a live spawn_data row of Antharas at its home holding
// value, its HP full.
func antharasLiveRow(value int) *spawn.State {
	return &spawn.State{Name: antharasRow, Status: spawn.StatusAlive, CurrentHP: 1000000, CurrentMP: 1 << 20, Location: antharasHome, DBValue: value}
}

// antharasDBValue shuts the server down and returns the db value saved
// with Antharas's spawn_data row.
func antharasDBValue(t *testing.T, w *baiumWorld) int {
	t.Helper()
	w.srv.Shutdown(t)
	states, err := gamesql.NewSpawnStore(w.srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state, ok := states[antharasRow]
	if !ok {
		t.Fatalf("no spawn_data row %s", antharasRow)
	}
	return state.DBValue
}

// antharasMemo returns what global memo key "1", Antharas's GM_ID, holds;
// -1 for nothing.
func antharasMemo(t *testing.T, w *baiumWorld) int {
	t.Helper()
	v := 0
	w.srv.RunScript(t, w.player.ObjectID(), antharasDirector, func(s *script.Script, _ *script.Player) { v = s.GlobalMemo().Int("1", -1) })
	return v
}

// antharasEvent sends Antharas script event 10001, as Heart of Warding
// does, from the director on the character's queue.
func antharasEvent(t *testing.T, w *baiumWorld, h *npc.Hostile) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), antharasDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), 10001, 0, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// antharasFrames renders a step's frames as baiumFrames does, plus
// SpecialCamera shots, in order.
func antharasFrames(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSpecialCamera {
			out = append(out, baiumFrames(t, [][]byte{f}, roles)...)
			continue
		}
		v := func(i int) int32 { return int32(binary.LittleEndian.Uint32(f[1+4*i:])) }
		out = append(out, fmt.Sprintf("S SpecialCamera obj=%s shot=%d,%d,%d,%d,%d,%d,%d,%d,%d", roles[v(0)], v(1), v(2), v(3), v(4), v(5), v(6), v(7), v(8), v(9)))
	}
	return out
}

// wantAntharasFrames fails unless the step's rendered frames are want.
func wantAntharasFrames(t *testing.T, step string, frames [][]byte, roles map[int32]string, want ...string) {
	t.Helper()
	if got := antharasFrames(t, frames, roles); !slices.Equal(got, want) {
		t.Fatalf("%s frames:\n got %q\nwant %q", step, got, want)
	}
}

// antharasCasts renders h's queued cast desires as "skill@target=weight",
// sorted, roles naming the targets.
func antharasCasts(h *npc.Hostile, roles map[int32]string) []string {
	var out []string
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind != actorai.IntentionCast || d.FinalTarget == nil {
			continue
		}
		out = append(out, fmt.Sprintf("%d@%s=%v", d.Skill.ID, roles[d.FinalTarget.ObjectID()], d.Weight))
	}
	slices.Sort(out)
	return out
}

// TestAntharasSleepsDiesAndComesBack ports Antharas's fresh spawn and death
// (phases 1 and 5): spawned with no row it stores db value 0 and names
// itself under global memo key 1, its GM_ID. Dying, it plays its camera
// (1200, 20, -10, 0, 13000) and BS01_D, clears key 1 and leaves the cube at
// 177615,114941,-7709 for 900 s. Respawned with no live row, it shakes the
// screens around (20 for 10 s), names itself under key 1 again, and its db
// value 0 is saved.
func TestAntharasSleepsDiesAndComesBack(t *testing.T) {
	w := bootAntharas(t, nil, "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	if handle.DBLoaded() || handle.DBValue() != 0 {
		t.Fatalf("Antharas loaded=%v value=%d, want not loaded at 0", handle.DBLoaded(), handle.DBValue())
	}
	if got := antharasMemo(t, w); got != int(boss.ObjectID()) {
		t.Fatalf("memo key 1 = %d, want Antharas %d", got, boss.ObjectID())
	}
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player"}

	w.onQueue(t, boss, func() { handle.DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	x, y, z := boss.Position()
	wantAntharasFrames(t, "dying hooks", w.advance(t, 3*time.Second), roles,
		"S SpecialCamera obj=antharas shot=1200,20,-10,0,13000,0,0,0,0",
		fmt.Sprintf("S PlaySound type=1 file=BS01_D bind=1 obj=antharas loc=%d,%d,%d delay=0", x, y, z))
	cube := soleBaiumNPC(t, w, antharasCubeID)
	if got, want := placeOf(cube), "177615,114941,-7689 heading 0"; got != want {
		t.Fatalf("the cube stands at %s, want %s", got, want)
	}
	if got := antharasMemo(t, w); got != -1 {
		t.Fatalf("memo key 1 after the death = %d, want none (-1)", got)
	}
	w.advance(t, 899*time.Second)
	if n := len(baiumNPCs(w, antharasCubeID)); n != 1 {
		t.Fatalf("%d cubes 899 s after the death, want 1", n)
	}
	w.advance(t, time.Second)
	if n := len(baiumNPCs(w, antharasCubeID)); n != 0 {
		t.Fatalf("%d cubes 900 s after the death, want none", n)
	}

	// The fixture's respawn timers never fire on their own: the test decays
	// the corpse, then fires the respawn its spawn armed.
	w.onQueue(t, boss, func() { boss.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(boss.ObjectID())) })
	const slot = antharasRow // a database-tracked spawn's slot is keyed by its dbName
	if !w.srv.NpcRespawns.Tracked(slot) {
		t.Fatal("Antharas's decay armed no respawn")
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.NpcSpawns.Respawn(slot)
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	again := soleBaiumNPC(t, w, antharasID)
	quake := "S Earthquake loc=184500,114835,-7678 intensity=20 duration=10 npc=1"
	if got := antharasFrames(t, frames, roles); !slices.Contains(got, quake) {
		t.Fatalf("respawn frames = %q, want %q", got, quake)
	}
	if got := antharasMemo(t, w); got != int(again.ObjectID()) {
		t.Fatalf("memo key 1 after the respawn = %d, want Antharas %d", got, again.ObjectID())
	}
	if got := antharasDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestAntharasResumesItsWait ports the restore of db value 1 (phase 1):
// restored from its live row it does not shake the screens; 10 minutes
// later it stores 2, teleports to its lair's entry 185452,114835,-8221,
// plays BS02_A there and wants to walk to 181911,114835,-7678 with 100000.
func TestAntharasResumesItsWait(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(1), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	if !handle.DBLoaded() || handle.DBValue() != 1 {
		t.Fatalf("Antharas loaded=%v value=%d, want loaded at 1", handle.DBLoaded(), handle.DBValue())
	}
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player"}

	wantAntharasFrames(t, "10 minutes less 1 ms", w.advance(t, 10*time.Minute-time.Millisecond), roles)
	wantAntharasFrames(t, "10 minutes", w.advance(t, time.Millisecond), roles,
		"S TeleportToLocation obj=antharas loc=185452,114835,-8221 fast=0",
		"S PlaySound type=1 file=BS02_A bind=1 obj=antharas loc=185452,114835,-8221 delay=0")
	d, ok := desire(boss, actorai.IntentionMoveTo)
	if !ok || d.Weight != 100000 || d.Location != (location.Location{X: 181911, Y: 114835, Z: -7678}) {
		t.Fatalf("Antharas's move desire = %+v (%v), want 181911,114835,-7678 with 100000", d, ok)
	}
	if got := antharasDBValue(t, w); got != 2 {
		t.Fatalf("saved db value = %d, want 2", got)
	}
}

// TestAntharasResumesItsMoveIn ports the restore of db value 2 (phase 1):
// restored from its live row it teleports at once to its lair's entry and
// wants to walk to its stage with 100000; the db value stays 2.
func TestAntharasResumesItsMoveIn(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(2), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	if got, want := placeOf(boss), "185452,114835,-8221 heading 0"; got != want {
		t.Fatalf("Antharas stands at %s, want %s", got, want)
	}
	d, ok := desire(boss, actorai.IntentionMoveTo)
	if !ok || d.Weight != 100000 || d.Location != (location.Location{X: 181911, Y: 114835, Z: -7678}) {
		t.Fatalf("Antharas's move desire = %+v (%v), want 181911,114835,-7678 with 100000", d, ok)
	}
	if got := antharasDBValue(t, w); got != 2 {
		t.Fatalf("saved db value = %d, want 2", got)
	}
}

// TestAntharasAwakens ports the awakening (phase 3), every draw 0: asleep
// at 0, a script event stores 1 and a second one changes nothing; 10 s
// later it stores 2, teleports to its lair's entry and plays BS02_A. Once
// it reaches its stage, 2 s later its camera sequence plays: 700/13/-19,
// 3 s later 700/13/0 over 6 s, 10 s later 3700/0/-3 with its two social
// desires, 0.2 s later 1100/0/-3 over 22 s, 10.8 s later 1100/0/-3 over
// 0.3 s, and it stores 3: it wants its buff 4125 on itself with 4000000
// and to wander, and the db value 3 is saved.
func TestAntharasAwakens(t *testing.T) {
	w := bootAntharas(t, nil, "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player"}

	antharasEvent(t, w, boss)
	if got := handle.DBValue(); got != 1 {
		t.Fatalf("db value after the event = %d, want 1", got)
	}
	antharasEvent(t, w, boss)
	w.srv.ReadQueued(t, w.srv.Client)
	wantAntharasFrames(t, "10 s less 1 ms", w.advance(t, 10*time.Second-time.Millisecond), roles)
	wantAntharasFrames(t, "10 s", w.advance(t, time.Millisecond), roles,
		"S TeleportToLocation obj=antharas loc=185452,114835,-8221 fast=0",
		"S PlaySound type=1 file=BS02_A bind=1 obj=antharas loc=185452,114835,-8221 delay=0")
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value after the move-in = %d, want 2", got)
	}

	stage := location.Location{X: 181911, Y: 114835, Z: -7678}
	w.ticks(t, 2)
	for i := 0; boss.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("Antharas's walk never arrived")
		}
		w.srv.TickPositions()
	}
	if x, y, _ := boss.Position(); x != stage.X || y != stage.Y {
		t.Fatalf("Antharas stopped at %d,%d, want its stage %d,%d", x, y, stage.X, stage.Y)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	if _, ok := desire(boss, actorai.IntentionMoveTo); ok {
		t.Fatal("Antharas still wants to move once at its stage")
	}

	wantAntharasFrames(t, "+2 s", w.advance(t, 2*time.Second), roles, "S SpecialCamera obj=antharas shot=700,13,-19,0,20000,0,0,0,0")
	wantAntharasFrames(t, "+3 s", w.advance(t, 3*time.Second), roles, "S SpecialCamera obj=antharas shot=700,13,0,6000,20000,0,0,0,0")
	wantAntharasFrames(t, "+10 s", w.advance(t, 10*time.Second), roles, "S SpecialCamera obj=antharas shot=3700,0,-3,0,10000,0,0,0,0")
	var socials []string
	for _, d := range boss.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionSocial {
			socials = append(socials, fmt.Sprintf("%d/%d=%v", d.ItemObjectID, d.Timer, d.Weight))
		}
	}
	slices.Sort(socials)
	if want := []string{"1/8000=1e+07", "2/6000=5e+06"}; !slices.Equal(socials, want) {
		t.Fatalf("Antharas's social desires = %q, want %q", socials, want)
	}
	wantAntharasFrames(t, "+0.2 s", w.advance(t, 200*time.Millisecond), roles, "S SpecialCamera obj=antharas shot=1100,0,-3,22000,30000,0,0,0,0")
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value before the last shot = %d, want 2", got)
	}
	frames := w.advance(t, 10800*time.Millisecond)
	if got := antharasFrames(t, frames, roles); len(got) == 0 || got[0] != "S SpecialCamera obj=antharas shot=1100,0,-3,300,7000,0,0,0,0" {
		t.Fatalf("+10.8 s frames = %q, want the last shot first", got)
	}
	if got := handle.DBValue(); got != 3 {
		t.Fatalf("db value after the last shot = %d, want 3", got)
	}
	if !slices.Contains(antharasCasts(boss, roles), "4125@antharas=4e+06") {
		t.Fatalf("Antharas's cast desires = %q, want 4125 on itself with 4000000", antharasCasts(boss, roles))
	}
	if _, ok := desire(boss, actorai.IntentionWander); !ok {
		t.Fatal("Antharas does not want to wander after its awakening")
	}
	if got := antharasDBValue(t, w); got != 3 {
		t.Fatalf("saved db value = %d, want 3", got)
	}
}

// TestAntharasSendsItsAttackerAwayAsleep ports a hit while it sleeps (phase
// 4): at db value 0 a hit sends the players of its lair to
// 80464,152294,-3534, up to 100 off on each axis.
func TestAntharasSendsItsAttackerAwayAsleep(t *testing.T) {
	w := bootAntharas(t, nil, "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	w.hit(t, boss, 1)
	w.srv.Settle(t)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if x < 80464-100 || x > 80464+100 || y < 152294-100 || y > 152294+100 || z != -3534 {
		t.Fatalf("the character stands at %d,%d,%d, want within 100 of 80464,152294,-3534", x, y, z)
	}
}

// TestAntharasResetsAfterFifteenQuietMinutes ports the reset (phase 4):
// restored fighting at 3 it checks every minute since its last hit; at 15
// minutes it goes on, at 16 it stores 0, sends the players of its lair to
// 80464,152294,-3534 (up to 100 off on each axis) and teleports out of its
// lair to -105200,-253104,-15264. The db value 0 is saved.
func TestAntharasResetsAfterFifteenQuietMinutes(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player"}

	w.advance(t, 15*time.Minute)
	if got := handle.DBValue(); got != 3 {
		t.Fatalf("db value at 15 minutes = %d, want 3", got)
	}
	frames := w.advance(t, time.Minute)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if x < 80464-100 || x > 80464+100 || y < 152294-100 || y > 152294+100 || z != -3534 {
		t.Fatalf("the character stands at %d,%d,%d, want within 100 of 80464,152294,-3534", x, y, z)
	}
	var teleports []string
	for _, line := range antharasFrames(t, frames, roles) {
		if len(line) > 20 && line[:20] == "S TeleportToLocation" {
			teleports = append(teleports, line)
		}
	}
	if want := []string{
		"S TeleportToLocation obj=antharas loc=-105200,-253104,-15264 fast=0",
		fmt.Sprintf("S TeleportToLocation obj=player loc=%d,%d,%d fast=0", x, y, z),
	}; !slices.Equal(teleports, want) {
		t.Fatalf("16 minutes teleports:\n got %q\nwant %q", teleports, want)
	}
	if got := handle.DBValue(); got != 0 {
		t.Fatalf("db value after the reset = %d, want 0", got)
	}
	if got := antharasDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestAntharasRanksWhatItSees ports the fight's ranking of a sighting
// (phase 4), every draw 0 (every even draw true). Restored fighting at 3,
// it sees the character, 1000 due west of it and no cleric, and ranks it
// first with 1000; then, idle, it casts at its best target: its draw of 70
// in 100 drops the character to 500, and above 3/4 of its HP its draw of
// 80 in 100 with the character in its narrow cone wants 4107 on itself.
func TestAntharasRanksWhatItSees(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player"}

	for i := 0; handle.Slot(script.SlotCreature2) == nil; i++ {
		if i == 10 {
			t.Fatal("Antharas ranked no one after 10 AI ticks")
		}
		w.ticks(t, 1)
	}
	if got := handle.Slot(script.SlotCreature2); got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Antharas's first target = %v, want the character", got)
	}
	if got := handle.Int(script.SlotIntAI2); got != 500 {
		t.Fatalf("the character's points after its sighting = %d, want 500", got)
	}
	// The desire was queued with 1000000; the AI ticks since have decayed
	// it.
	if got := antharasCasts(boss, roles); len(got) != 1 || !strings.HasPrefix(got[0], "4107@antharas=") {
		t.Fatalf("Antharas's cast desires after its sighting = %q, want 4107 on itself", got)
	}
}

// TestAntharasRanksItsAttackers ports the fight's ranking of plain hits and
// its skill choice (phase 4), every draw 0 (every even draw true), before
// its AI first runs. Restored fighting at 3 and idle, a hit of 7 from the
// character, 1000 due west of it, ranks it first with 7000; its cast then
// drops it to 500 and, the character in its narrow cone, wants 4107 on
// itself. A hit of 5 from a monster 500 due north takes the second slot
// with 5000, the best: dropped to 500, and out of the cone, the monster
// draws 4108 on it with its draw of 10 in 100.
func TestAntharasRanksItsAttackers(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	hitter := w.spawn(t, antharasHitterID, 1000, 500)
	roles := map[int32]string{boss.ObjectID(): "antharas", w.player.ObjectID(): "player", hitter.ObjectID(): "hitter"}
	if got := boss.AI().CurrentIntention(); got != actorai.IntentionIdle {
		t.Fatalf("Antharas's intention before its AI runs = %v, want idle", got)
	}

	w.onQueue(t, boss, func() { boss.TakeDamage(7, w.player) })
	if got := handle.Slot(script.SlotCreature2); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Antharas's first target = %v, want the character", got)
	}
	if got := handle.Int(script.SlotIntAI2); got != 500 {
		t.Fatalf("the character's points after its hit and the cast = %d, want 500", got)
	}
	if got, want := antharasCasts(boss, roles), []string{"4107@antharas=1e+06"}; !slices.Equal(got, want) {
		t.Fatalf("Antharas's cast desires after the character's hit = %q, want %q", got, want)
	}

	w.onQueue(t, boss, func() { boss.TakeDamage(5, hitter) })
	if got := handle.Slot(script.SlotCreature3); got == nil || got.ObjectID() != hitter.ObjectID() {
		t.Fatalf("Antharas's second target = %v, want the monster", got)
	}
	if got := handle.Int(script.SlotIntAI3); got != 500 {
		t.Fatalf("the monster's points after its hit and the cast = %d, want 500", got)
	}
	if got := handle.Int(script.SlotIntAI2); got != 500 {
		t.Fatalf("the character's points after the monster's hit = %d, want 500", got)
	}
	if got, want := antharasCasts(boss, roles), []string{"4107@antharas=1e+06", "4108@hitter=1e+06"}; !slices.Equal(got, want) {
		t.Fatalf("Antharas's cast desires after the monster's hit = %q, want %q", got, want)
	}
}

// TestAntharasCallsItsBehemothUnderHalfHP ports the Behemoth Dragon call
// (phase 4): restored fighting at 3 and under half its HP, its 2-minute
// check calls the Behemoth Dragon 29069 where it stands, facing 0, with its
// GM_ID 1 as the private's first parameter; the next check calls no other.
func TestAntharasCallsItsBehemothUnderHalfHP(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	w.onQueue(t, boss, func() { boss.SetCurrentHP(boss.MaxHP() * 4 / 10) })

	w.advance(t, 2*time.Minute-time.Millisecond)
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 0 {
		t.Fatalf("%d Behemoth Dragons before 2 minutes, want none", n)
	}
	w.advance(t, time.Millisecond)
	dragon := soleBaiumNPC(t, w, antharasBehemothID)
	if got := script.NPCOf(dragon).Int(script.SlotParam1); got != 1 {
		t.Fatalf("the Behemoth Dragon's first parameter = %d, want 1", got)
	}
	if dragon.Heading() != 0 {
		t.Fatalf("the Behemoth Dragon faces %d, want 0", dragon.Heading())
	}
	if got := script.NPCOf(boss).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("Antharas's call flag = %d, want 1", got)
	}
	w.advance(t, 2*time.Minute)
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 1 {
		t.Fatalf("%d Behemoth Dragons after the next check, want 1", n)
	}
}
