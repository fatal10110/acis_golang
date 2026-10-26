package ai

import (
	"context"
	"fmt"
	"math"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/valakas"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Valakas, the Lavasaurus Elder it raises and its exit cube.
const (
	valakasID         = int32(29028)
	valakasPrivateID  = int32(29029)
	valakasCubeID     = int32(31759)
	valakasRow        = "valakas"
	valakasDirector   = "ValakasDirector"
	valakasWaitTime   = 20 * time.Minute // ValakasWaitTime when npcs.properties sets none
	valakasMemoKey    = "3"
	valakasSleepPlace = "-105200,-253104,-15264"
)

// valakasLair is where the fixture's spawn places Valakas, the point of its
// lair it appears at; the character enters the world 1000 due west of it,
// inside the lair's territory.
var (
	valakasLair      = location.Location{X: 212852, Y: -114842, Z: -1632}
	valakasCharacter = location.Location{X: 211852, Y: -114842, Z: -1632}
)

// valakasSpawnlist is the shipped lair territory and Valakas's two
// npcmakers: its database-tracked one, placing it at valakasLair (the
// shipped one places it at its sleeping place) and respawning it after the
// given time, and the exit cubes' one (one of its cubes), which spawns
// nothing until the first one's last NPC leaves.
const valakasSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="godard11_2614_01" minZ="-2572" maxZ="5428">
		<node x="196866" y="-130845"/>
		<node x="229289" y="-130860"/>
		<node x="229176" y="-98508"/>
		<node x="196880" y="-98498"/>
	</territory>
	<npcmaker name="boss_26_14" territory="godard11_2614_01" maximumNpcs="93">
		<ai type="default_use_db_maker">
			<set name="maker_name" val="valakascuve_2614"/>
		</ai>
		<npc id="29028" pos="212852;-114842;-1632;0" total="1" respawn="%s" respawnRand="0sec" dbName="valakas"/>
	</npcmaker>
	<territory name="godard11_2614_02" minZ="-2572" maxZ="5428">
		<node x="196866" y="-130845"/>
		<node x="229289" y="-130860"/>
		<node x="229176" y="-98508"/>
		<node x="196880" y="-98498"/>
	</territory>
	<npcmaker name="valakascuve_2614" territory="godard11_2614_02" maximumNpcs="30">
		<ai type="default_maker">
			<set name="on_start_spawn" val="0"/>
		</ai>
		<npc id="31759" pos="214880;-116144;-1644;0" total="1" respawn="no"/>
	</npcmaker>
</list>`

// valakasSkills are the skills Valakas casts, as fixture definitions it can
// always pay for: its regeneration in its four levels, the others at 1.
func valakasSkills() *modelskill.Table {
	var defs []modelskill.Definition
	add := func(id modelskill.ID, level int) {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		})
	}
	for _, id := range []modelskill.ID{4258, 4680, 4681, 4682, 4683, 4684, 4685, 4687, 4688, 4689, 4690} {
		add(id, 1)
	}
	for level := 1; level <= 4; level++ {
		add(4691, level)
	}
	return modelskill.NewTable(defs)
}

// valakasQuietDraws draws 2 for every percent draw and 0 for any other:
// no hit raises Lavasaurus Elders (2 is not under 2), and every other
// percent chance of 3 or more passes.
func valakasQuietDraws(n int) int {
	if n == 100 {
		return 2
	}
	return 0
}

// bootValakas boots Valakas's lair backed by the spawn_data table, after
// saving row as Valakas's live row when it is set, Valakas respawning after
// 1 hour, the script draws from rand, and the character entering the world
// at valakasCharacter.
func bootValakas(t *testing.T, row *spawn.State, rand func(int) int) *baiumWorld {
	t.Helper()
	if row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{valakasRow: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	w := &baiumWorld{start: time.Now()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "valakas.xml"), []byte(fmt.Sprintf(valakasSpawnlist, "1hour")), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := warriorTemplate(valakasID, nil)
	boss.Type, boss.Name, boss.Level, boss.HPMax = "GrandBoss", "Valakas", 85, 1000000
	boss.AggroRange, boss.CanSeeThrough = 2000, true
	boss.RunSpeed, boss.WalkSpeed = 1000, 1000
	lavasaurus := warriorTemplate(valakasPrivateID, nil)
	lavasaurus.Name = "Lavasaurus Elder"
	kinds := map[int32]script.NPCKind{valakasID: script.KindHostile, valakasPrivateID: script.KindHostile, valakasCubeID: script.KindFolk}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.boss.valakas.Valakas"}, {Path: "test." + valakasDirector}}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc": defaultnpc.New,
		"script.ai.boss.valakas.Valakas":  valakas.Valakas,
		"test." + valakasDirector:         func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{valakasCharacter}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{boss, lavasaurus, gameservertest.FolkTemplate("Folk", int(valakasCubeID))})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(valakasSkills()),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(rand),
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

// valakasLiveRow is a live spawn_data row of Valakas in its lair holding
// value, its HP full.
func valakasLiveRow(value int) *spawn.State {
	return &spawn.State{Name: valakasRow, Status: spawn.StatusAlive, CurrentHP: 1000000, CurrentMP: 1 << 20, Location: valakasLair, DBValue: value}
}

// valakasDBValue shuts the server down and returns the db value saved with
// Valakas's spawn_data row.
func valakasDBValue(t *testing.T, w *baiumWorld) int {
	t.Helper()
	w.srv.Shutdown(t)
	states, err := gamesql.NewSpawnStore(w.srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state, ok := states[valakasRow]
	if !ok {
		t.Fatalf("no spawn_data row %s", valakasRow)
	}
	return state.DBValue
}

// valakasMemo returns what global memo key "3" holds; -1 for nothing.
func valakasMemo(t *testing.T, w *baiumWorld) int {
	t.Helper()
	v := 0
	w.srv.RunScript(t, w.player.ObjectID(), valakasDirector, func(s *script.Script, _ *script.Player) { v = s.GlobalMemo().Int(valakasMemoKey, -1) })
	return v
}

// valakasEvent sends Valakas script event 0, as Heart of Volcano does, from
// the director on the character's queue.
func valakasEvent(t *testing.T, w *baiumWorld, h *npc.Hostile) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), valakasDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), 0, 0, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// valakasShot renders one of Valakas's camera shots as antharasFrames does.
func valakasShot(shot string) string { return "S SpecialCamera obj=valakas shot=" + shot }

// TestValakasSleepsAndDies ports the fresh spawn and the death (phases 1 and
// 5): spawned with no row it stores db value 0 and names itself under
// global memo key 3. Its dying hooks running 3 s after its death, it clears
// key 3, stores 0, plays B03_D and its camera 2000/130/-1; 0.5 s later its
// seven death shots follow, 3.5, 4.5, 0.5, 4.6, 0.75 and 2.5 s apart. The db value 0 is saved.
func TestValakasSleepsAndDies(t *testing.T) {
	w := bootValakas(t, nil, valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	if handle.DBLoaded() || handle.DBValue() != 0 {
		t.Fatalf("Valakas loaded=%v value=%d, want not loaded at 0", handle.DBLoaded(), handle.DBValue())
	}
	if got := valakasMemo(t, w); got != int(boss.ObjectID()) {
		t.Fatalf("memo key 3 = %d, want Valakas %d", got, boss.ObjectID())
	}
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	w.onQueue(t, boss, func() { handle.DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	x, y, z := boss.Position()
	wantAntharasFrames(t, "before the dying hooks", w.advance(t, 3*time.Second-time.Millisecond), roles)
	wantAntharasFrames(t, "dying hooks", w.advance(t, time.Millisecond), roles,
		fmt.Sprintf("S PlaySound type=1 file=B03_D bind=1 obj=valakas loc=%d,%d,%d delay=0", x, y, z),
		valakasShot("2000,130,-1,0,10000,0,0,1,1"))
	if got := valakasMemo(t, w); got != -1 {
		t.Fatalf("memo key 3 after the death = %d, want none (-1)", got)
	}
	for _, step := range []struct {
		after time.Duration
		shot  string
	}{
		{500 * time.Millisecond, "1100,210,-5,3000,10000,-13,0,1,1"},
		{3500 * time.Millisecond, "1300,200,-8,3000,10000,0,15,1,1"},
		{4500 * time.Millisecond, "1000,190,0,500,10000,0,10,1,1"},
		{500 * time.Millisecond, "1700,120,0,2500,10000,12,40,1,1"},
		{4600 * time.Millisecond, "1700,20,0,700,10000,10,10,1,1"},
		{750 * time.Millisecond, "1700,10,0,1000,10000,20,70,1,1"},
		{2500 * time.Millisecond, "1700,10,0,300,250,20,-20,1,1"},
	} {
		if step.after > time.Millisecond {
			wantAntharasFrames(t, "before "+step.shot, w.advance(t, step.after-time.Millisecond), roles)
			step.after = time.Millisecond
		}
		wantAntharasFrames(t, step.shot, w.advance(t, step.after), roles, valakasShot(step.shot))
	}
	wantAntharasFrames(t, "after the last shot", w.advance(t, time.Minute), roles)
	if got := valakasDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestValakasResumesItsWait ports the restore of db value 1 (phase 1):
// restored from its live row, 10 minutes later it stores 2, teleports to
// 212852,-114842,-1632 and plays B03_A there, and 2 s later its first
// camera shot plays. The db value 2 is saved.
func TestValakasResumesItsWait(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(1), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	if !handle.DBLoaded() || handle.DBValue() != 1 {
		t.Fatalf("Valakas loaded=%v value=%d, want loaded at 1", handle.DBLoaded(), handle.DBValue())
	}
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	wantAntharasFrames(t, "10 minutes less 1 ms", w.advance(t, 10*time.Minute-time.Millisecond), roles)
	wantAntharasFrames(t, "10 minutes", w.advance(t, time.Millisecond), roles,
		"S TeleportToLocation obj=valakas loc=212852,-114842,-1632 fast=0",
		"S PlaySound type=1 file=B03_A bind=1 obj=valakas loc=212852,-114842,-1632 delay=0")
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value after the wait = %d, want 2", got)
	}
	wantAntharasFrames(t, "+2 s", w.advance(t, 2*time.Second), roles, valakasShot("1800,180,-1,1500,10000,0,0,1,0"))
	if got := valakasDBValue(t, w); got != 2 {
		t.Fatalf("saved db value = %d, want 2", got)
	}
}

// TestValakasResumesItsFight ports the restore of db value 3 (phase 1):
// restored from its live row it wants to wander, and a minute later, at
// full HP, wants its regeneration 4691 level 1 on itself with 4000000.
func TestValakasResumesItsFight(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(3), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}
	if _, ok := desire(boss, actorai.IntentionWander); !ok {
		t.Fatal("Valakas restored fighting does not want to wander")
	}
	if got := antharasCasts(boss, roles); slices.Contains(got, "4691@valakas=4e+06") {
		t.Fatalf("Valakas's cast desires before its first minute = %q, want no regeneration yet", got)
	}
	w.advance(t, time.Minute)
	if !slices.Contains(valakasCastLevels(boss, roles), "4691-1@valakas=4e+06") {
		t.Fatalf("Valakas's cast desires after a minute = %q, want 4691 level 1 on itself with 4000000", valakasCastLevels(boss, roles))
	}
}

// valakasCastLevels renders h's queued cast desires as
// "skill-level@target=weight", sorted, roles naming the targets.
func valakasCastLevels(h *npc.Hostile, roles map[int32]string) []string {
	var out []string
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind != actorai.IntentionCast || d.FinalTarget == nil {
			continue
		}
		out = append(out, fmt.Sprintf("%d-%d@%s=%v", d.Skill.ID, d.Skill.Level, roles[d.FinalTarget.ObjectID()], d.Weight))
	}
	slices.Sort(out)
	return out
}

// TestValakasAwakens ports the entry and the awakening (phases 2 and 3):
// asleep at 0, a script event stores 1 and a second one changes nothing;
// once ValakasWaitTime (20 minutes) has passed it stores 2, teleports to
// 212852,-114842,-1632 and plays B03_A, and 2 s later its nine camera shots
// play, 1.5, 3.3, 2.9, 2.7, 0.001, 3.2, 1.4 and 6.7 s apart; 5.7 s after
// the last one it stores 3 and wants its regeneration 4691 level 1 on
// itself with 4000000. With a hero within 4000 its screen message 1000519
// shows for 3 s then; with none, nothing does. The db value 3 is saved.
func TestValakasAwakens(t *testing.T) {
	for _, hero := range []bool{false, true} {
		t.Run(fmt.Sprintf("hero=%v", hero), func(t *testing.T) { valakasAwakens(t, hero) })
	}
}

// valakasAwakens runs TestValakasAwakens with the character a hero or not.
func valakasAwakens(t *testing.T, hero bool) {
	w := bootValakas(t, nil, valakasQuietDraws)
	if hero {
		w.srv.RunQuest(t, w.player.ObjectID(), valakasDirector, func(_ *script.Quests, c *player.Character, _ *script.Script) { c.SetHero(true) })
		w.srv.ReadQueued(t, w.srv.Client)
	}
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	valakasEvent(t, w, boss)
	if got := handle.DBValue(); got != 1 {
		t.Fatalf("db value after the event = %d, want 1", got)
	}
	valakasEvent(t, w, boss)
	w.srv.ReadQueued(t, w.srv.Client)
	wantAntharasFrames(t, "wait time less 1 ms", w.advance(t, valakasWaitTime-time.Millisecond), roles)
	wantAntharasFrames(t, "wait time", w.advance(t, time.Millisecond), roles,
		"S TeleportToLocation obj=valakas loc=212852,-114842,-1632 fast=0",
		"S PlaySound type=1 file=B03_A bind=1 obj=valakas loc=212852,-114842,-1632 delay=0")
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value after the wait = %d, want 2", got)
	}

	for _, step := range []struct {
		after time.Duration
		shot  string
	}{
		{2000 * time.Millisecond, "1800,180,-1,1500,10000,0,0,1,0"},
		{1500 * time.Millisecond, "1300,180,-5,3000,10000,0,-5,1,0"},
		{3300 * time.Millisecond, "500,180,-8,600,10000,0,60,1,0"},
		{2900 * time.Millisecond, "800,180,-8,2700,10000,0,30,1,0"},
		{2700 * time.Millisecond, "200,250,70,0,10000,30,80,1,0"},
		{time.Millisecond, "1100,250,70,2500,10000,30,80,1,0"},
		{3200 * time.Millisecond, "700,150,30,0,10000,-10,60,1,0"},
		{1400 * time.Millisecond, "1200,150,20,2900,10000,-10,30,1,0"},
		{6700 * time.Millisecond, "750,170,-10,3400,4000,10,-15,1,0"},
	} {
		if step.after > time.Millisecond {
			wantAntharasFrames(t, "before "+step.shot, w.advance(t, step.after-time.Millisecond), roles)
			step.after = time.Millisecond
		}
		wantAntharasFrames(t, step.shot, w.advance(t, step.after), roles, valakasShot(step.shot))
	}
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value before the end of the cameras = %d, want 2", got)
	}

	wantAntharasFrames(t, "5.7 s less 1 ms", w.advance(t, 5700*time.Millisecond-time.Millisecond), roles)
	var last []string
	if hero {
		text, _ := npcstring.Text(1000519)
		last = append(last, "S ExShowScreenMessage "+text+"@3000")
	}
	wantAntharasFrames(t, "5.7 s", w.advance(t, time.Millisecond), roles, last...)
	if got := handle.DBValue(); got != 3 {
		t.Fatalf("db value after the cameras = %d, want 3", got)
	}
	if !slices.Contains(valakasCastLevels(boss, roles), "4691-1@valakas=4e+06") {
		t.Fatalf("Valakas's cast desires = %q, want 4691 level 1 on itself with 4000000", valakasCastLevels(boss, roles))
	}
	if got := valakasDBValue(t, w); got != 3 {
		t.Fatalf("saved db value = %d, want 3", got)
	}
}

// TestValakasSendsItsAttackerAwayAsleep ports a hit while it sleeps (phase
// 4): at db value 0 the player who hits it is sent to 150037,-57255,-2976,
// with no scatter.
func TestValakasSendsItsAttackerAwayAsleep(t *testing.T) {
	w := bootValakas(t, nil, valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	w.hit(t, boss, 1)
	w.srv.Settle(t)
	if x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID()); x != 150037 || y != -57255 || z != -2976 {
		t.Fatalf("the character stands at %d,%d,%d, want 150037,-57255,-2976", x, y, z)
	}
}

// TestValakasResetsAfterFifteenQuietMinutes ports the reset (phase 4):
// restored fighting at 3 it checks every minute since its last hit; at 15
// minutes it goes on, at 16 it stores 0, sends the players of its lair to
// 150037,-57255,-2976 (up to 150 off on each axis) and teleports to its
// sleeping place -105200,-253104,-15264. The db value 0 is saved.
func TestValakasResetsAfterFifteenQuietMinutes(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(3), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	w.advance(t, 15*time.Minute)
	if got := handle.DBValue(); got != 3 {
		t.Fatalf("db value at 15 minutes = %d, want 3", got)
	}
	frames := w.advance(t, time.Minute)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if x < 150037-150 || x > 150037+150 || y < -57255-150 || y > -57255+150 || z != -2976 {
		t.Fatalf("the character stands at %d,%d,%d, want within 150 of 150037,-57255,-2976", x, y, z)
	}
	var teleports []string
	for _, line := range antharasFrames(t, frames, roles) {
		if strings.HasPrefix(line, "S TeleportToLocation") {
			teleports = append(teleports, line)
		}
	}
	if want := []string{
		"S TeleportToLocation obj=valakas loc=" + valakasSleepPlace + " fast=0",
		fmt.Sprintf("S TeleportToLocation obj=player loc=%d,%d,%d fast=0", x, y, z),
	}; !slices.Equal(teleports, want) {
		t.Fatalf("16 minutes teleports:\n got %q\nwant %q", teleports, want)
	}
	if got := handle.DBValue(); got != 0 {
		t.Fatalf("db value after the reset = %d, want 0", got)
	}
	if got := valakasDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestValakasRanksAndCastsAtItsAttacker ports the fight's ranking of a
// plain hit and its skill choice (phase 4), before its AI first runs:
// restored fighting at 3 and idle, a hit of 7 from the character, on its
// ground 1000 due west, ranks it first with 7000 and, the percent draws 2,
// its cast drops it to 500 and, above 3/4 of its HP, wants its fear 4689 at
// the character with 1000000 (its meteor chance there is 0).
func TestValakasRanksAndCastsAtItsAttacker(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(3), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}
	if got := boss.AI().CurrentIntention(); got != actorai.IntentionIdle {
		t.Fatalf("Valakas's intention before its AI runs = %v, want idle", got)
	}

	w.onQueue(t, boss, func() { boss.TakeDamage(7, w.player) })
	if got := handle.Slot(npc.CreatureQuest2); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Valakas's first target = %v, want the character", got)
	}
	if got := handle.Int(npc.IntQuest2); got != 500 {
		t.Fatalf("the character's points after its hit and the cast = %d, want 500", got)
	}
	if got, want := antharasCasts(boss, roles), []string{"4689@player=1e+06"}; !slices.Equal(got, want) {
		t.Fatalf("Valakas's cast desires after the hit = %q, want %q", got, want)
	}
	if n := len(baiumNPCs(w, valakasPrivateID)); n != 0 {
		t.Fatalf("%d Lavasaurus Elders after a hit with a percent draw of 2, want none", n)
	}
}

// TestValakasFliesAndReadiesItsRearAttack ports its flight and rear attack
// (phase 4): restored fighting at 3, the damage it takes from its ground
// counts in whole shares of its max HP, so two hits of half its HP from the
// character, 1000 due west of it and so behind it, healed between, make it
// fly: it drops its attack desires and wants to move to its nearest flee
// point 212896,-114592,-1636 with 100000000, and wants its rear attack
// readiness 4687 on itself with 1000000. Once there it wants its power up
// 4680 on itself with 1000000.
func TestValakasFliesAndReadiesItsRearAttack(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(3), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	half := (boss.MaxHP() + 1) / 2
	w.onQueue(t, boss, func() {
		boss.SetCurrentHP(boss.MaxHP())
		boss.TakeDamage(half, w.player)
	})
	if d, ok := desire(boss, actorai.IntentionMoveTo); ok {
		t.Fatalf("Valakas wants to move after half its HP: %+v", d)
	}
	if got := antharasCasts(boss, roles); slices.Contains(got, "4687@valakas=1e+06") {
		t.Fatalf("Valakas's cast desires after half its HP = %q, want no rear attack readiness", got)
	}
	w.onQueue(t, boss, func() {
		boss.SetCurrentHP(boss.MaxHP())
		boss.TakeDamage(half, w.player)
	})
	flee := location.Location{X: 212896, Y: -114592, Z: -1636}
	d, ok := desire(boss, actorai.IntentionMoveTo)
	if !ok || d.Weight != 100000000 || d.Location != flee {
		t.Fatalf("Valakas's move desire = %+v (%v), want %v with 100000000", d, ok, flee)
	}
	if _, ok := desire(boss, actorai.IntentionAttack); ok {
		t.Fatal("Valakas still wants to attack once it flies")
	}
	if got := antharasCasts(boss, roles); !slices.Contains(got, "4687@valakas=1e+06") {
		t.Fatalf("Valakas's cast desires = %q, want 4687 on itself with 1000000", got)
	}

	w.ticks(t, 2)
	for i := 0; boss.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("Valakas's flight never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	if x, y, _ := boss.Position(); x != flee.X || y != flee.Y {
		t.Fatalf("Valakas stopped at %d,%d, want its flee point %d,%d", x, y, flee.X, flee.Y)
	}
	if got := antharasCasts(boss, roles); !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "4680@valakas=") }) {
		t.Fatalf("Valakas's cast desires at its flee point = %q, want 4680 on itself", got)
	}
}

// TestValakasRaisesLavasaurusElders ports its privates (phase 4), every
// draw 0: restored fighting at 3, a hit from the character raises four
// Lavasaurus Elders, at the two spots of each of its spot lists farthest
// from the character, each started with the character's object id as its
// first parameter.
func TestValakasRaisesLavasaurusElders(t *testing.T) {
	w := bootValakas(t, valakasLiveRow(3), func(int) int { return 0 })
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	w.hit(t, boss, 1)

	var want []string
	for _, spots := range [][][3]int32{valakasFirstSpots, valakasSecondSpots} {
		want = append(want, valakasFarthestTwo(w.at, spots)...)
	}
	slices.Sort(want)
	var got []string
	for _, c := range baiumNPCs(w, valakasPrivateID) {
		x, y, _ := c.Position()
		got = append(got, fmt.Sprintf("%d,%d", x, y))
		if p := script.NPCOf(c.(*npc.Hostile)).Int(script.SlotParam1); p != w.player.ObjectID() {
			t.Errorf("the Lavasaurus Elder at %d,%d has first parameter %d, want the character %d", x, y, p, w.player.ObjectID())
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("Lavasaurus Elders at %q, want %q", got, want)
	}
}

// valakasFarthestTwo returns, as "x,y", the two of spots farthest from at
// in 3D.
func valakasFarthestTwo(at location.Location, spots [][3]int32) []string {
	dist := func(s [3]int32) float64 {
		return math.Sqrt(math.Pow(float64(int(s[0])-at.X), 2) + math.Pow(float64(int(s[1])-at.Y), 2) + math.Pow(float64(int(s[2])-at.Z), 2))
	}
	first, second := -1, -1
	for i, s := range spots {
		switch {
		case first < 0 || dist(s) > dist(spots[first]):
			first, second = i, first
		case second < 0 || dist(s) > dist(spots[second]):
			second = i
		}
	}
	return []string{fmt.Sprintf("%d,%d", spots[first][0], spots[first][1]), fmt.Sprintf("%d,%d", spots[second][0], spots[second][1])}
}
