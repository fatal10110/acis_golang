package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The batch's NPCs: the raid bosses it binds, a private of theirs and a
// listener NPC that records the script events it hears.
const (
	rb5SummonID   = int32(29040) // RaidBossAloneSummonPrivate
	rb5RiftID     = int32(25333) // RaidBossForTeleportDungeon
	rb5SailrenID  = int32(29065) // Sailren
	rb5GolemID    = int32(25512) // DrChaosGiganticGolem
	rb5GuardID    = int32(29060) // FreyaRoyalGuard
	rb5Type2ID    = int32(25286) // RaidBossType2
	rb5Type2LowID = int32(25524) // RaidBossType2, below the character
	rb5PrivateID  = int32(29991) // no behavior
	rb5ListenerID = int32(29992)
	rb5RangeDD    = modelskill.ID(9945)
)

// rb5Batch is the raid chain in scripts.xml order with the batch's classes.
var rb5Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.Monster.MonsterAI", monster.New},
	{"script.ai.individual.Monster.RaidBoss.RaidBossStandard", raid.BossStandard},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossAloneSummonPrivate", raid.BossAloneSummonPrivate},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossForTeleportDungeon", raid.BossForTeleportDungeon},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1", raid.BossType1},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Sailren", raid.Sailren},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem", raid.DrChaosGiganticGolem},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.FreyaRoyalGuard", raid.FreyaRoyalGuard},
	{"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.RaidBossType1Aggressive", raid.BossType1Aggressive},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty", raid.BossParty},
	{"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2", raid.BossType2},
}

// rb5Makers records the maker script events its npcmakers run, as
// "<maker> <event>", and has each npcmaker place its NPCs as it starts.
type rb5Makers struct {
	mu  sync.Mutex
	log []string
}

func (m *rb5Makers) take() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.log
	m.log = nil
	return out
}

func (m *rb5Makers) catalog() script.MakerCatalog {
	return script.MakerCatalog{"rb5_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				for _, s := range e.Group.Spawns() {
					s.Spawn()
				}
			},
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.log = append(m.log, e.Group.Maker().Name+" "+e.Name)
			},
		}}
	}}
}

// rb5Spawnlist declares the npcmakers the batch's bosses talk to, every one
// a probe: the rift room that places the rift boss inside its territory,
// Sailren's and the Ice Queen's.
const rb5Spawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rb5_rift" minZ="-100" maxZ="200"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="dimension_gap_08" territory="rb5_rift" maximumNpcs="2">
		<ai type="rb5_probe"/>
		<npc id="25333" total="1" pos="200;200;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_01m1" territory="rb5_rift" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_13m1" territory="rb5_rift" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
	<npcmaker name="schuttgart13_mb2314_05m1" territory="rb5_rift" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
</list>`

// rb5World is ind7World with the probe npcmakers.
type rb5World struct {
	*ind7World
	makers *rb5Makers
}

// bootRaid5 boots the batch beside templates and a listener, every NPC
// casting the raid boss fixture skills, every draw 0 until a test queues
// others, with the probe npcmakers of rb5Spawnlist.
func bootRaid5(t *testing.T, extra raidBoot, templates ...*npc.Template) *rb5World {
	t.Helper()
	var list []script.Listing
	catalog := script.Catalog{}
	for _, c := range rb5Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Rb5Listener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{rb5ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(rb5ListenerID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rb5.xml"), []byte(rb5Spawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	makers := &rb5Makers{}
	draws := &ind1Draws{}
	skills := append(rbSkills(), ind1Skill(rb5RangeDD, 600, false))
	w := bootRaid(t, raidBoot{ready: extra.ready, opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(append(skills, raidCurseDefinition()))),
		gameservertest.WithScriptSkills(skills...),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(makers.catalog()),
	}, extra.opts...)}, templates...)
	return &rb5World{
		ind7World: &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: heard}, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())},
		makers:    makers,
	}
}

// rb5Boss is a raid boss fixture of id at level 20 that does not walk and
// sees 1000 around itself.
func rb5Boss(id int32, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := rbTemplate(id, 20, nil, skills)
	tmpl.CanMove, tmpl.AggroRange = false, 1000
	return tmpl
}

// rb5Seer is rb5Boss as a plain monster, which tells its see-creature
// hooks about a playable once as it comes in sight: a raid boss tells them
// only on a tick that it or the playable moves.
func rb5Seer(id int32) *npc.Template {
	tmpl := rb5Boss(id, nil)
	tmpl.Type = "Monster"
	return tmpl
}

// rb5Leader is rb5Boss leading two privates that come back 240 s after
// they die when asked.
func rb5Leader(id int32, level int, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := rb5Boss(id, skills)
	tmpl.Level = level
	tmpl.Privates = []npc.PrivateEntry{
		{NpcID: int(rb5PrivateID), Weight: 1, RespawnDelay: 240 * time.Second},
		{NpcID: int(rb5PrivateID), Weight: 1, RespawnDelay: 240 * time.Second},
	}
	return tmpl
}

// rb5PrivateTemplate is the leaders' private, which does not walk.
func rb5PrivateTemplate() *npc.Template {
	tmpl := warriorTemplate(rb5PrivateID, nil)
	tmpl.CanMove = false
	return tmpl
}

// sees drops h's desires, moves the character out of its sight for one AI
// tick, then 100 in front of it for another, whose see-creature scan tells
// h about it, and returns h's attack desire on the character.
func (w *rb5World) sees(t *testing.T, h *npc.Hostile) float64 {
	t.Helper()
	w.clearDesires(t, h)
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x+700, y, z); err != nil {
		t.Fatalf("move the character away: %v", err)
	}
	w.ticks(t, 1)
	w.lookAt(t, h)
	d, _ := attackOn(h, w.player)
	return d.Weight
}

// TestRaidBossType1AggressiveAttacksWhatItSees ports RaidBossType1Aggressive's
// see-creature hook on FreyaRoyalGuard: seeing the character while it stands
// in its territory, the guard wants to attack it at 200; moved out of its
// territory, it wants nothing of the character it sees.
func TestRaidBossType1AggressiveAttacksWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Seer(rb5GuardID))
	w.draws.set(1)
	guard := w.spawn(t, rb5GuardID, 400, 0)
	if got := w.sees(t, guard); got != 200 {
		t.Fatalf("attack desire on the character seen in territory = %v, want 200", got)
	}
	w.onQueue(t, guard, func() {
		script.NPCOf(guard).InstantTeleportTo(script.Loc{X: w.at.X + 3000, Y: w.at.Y, Z: w.at.Z})
	})
	if script.NPCOf(guard).InMyTerritory() {
		t.Fatal("setup: the guard is still in its territory")
	}
	if got := w.sees(t, guard); got != 0 {
		t.Fatalf("attack desire on the character seen out of territory = %v, want none", got)
	}
}

// TestFreyaRoyalGuardTellsTheIceQueenAsItDies ports FreyaRoyalGuard's dying
// hook: it runs maker script event 10005 on schuttgart13_mb2314_05m1 and
// tells the NPCs within 7000 event 11036 with 3; it calls no parent.
func TestFreyaRoyalGuardTellsTheIceQueenAsItDies(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Boss(rb5GuardID, nil))
	w.draws.set(1)
	guard := w.spawn(t, rb5GuardID, 400, 0)
	w.spawn(t, rb5ListenerID, -400, 0)
	w.makers.take()
	w.die(t, guard)
	if got, want := w.makers.take(), []string{"schuttgart13_mb2314_05m1 10005"}; !slices.Equal(got, want) {
		t.Fatalf("maker events = %q, want %q", got, want)
	}
	if got, want := w.heard.take(), []string{"event=11036 arg1=3 arg2=0"}; !slices.Equal(got, want) {
		t.Fatalf("events heard = %q, want %q", got, want)
	}
}

// TestDrChaosGiganticGolemWakesUp ports DrChaosGiganticGolem's created,
// timer and see-creature hooks. 1 s after it appears it plays its camera
// shot (distance 30, yaw -200, pitch 20, time 6000, duration 8000) on
// itself; until 10 s later only its parent's 200 answers the character it
// sees, then its own 200 adds to it.
func TestDrChaosGiganticGolemWakesUp(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Seer(rb5GolemID))
	w.draws.set(1)
	golem := w.spawn(t, rb5GolemID, 400, 0)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := w.sees(t, golem); got != 200 {
		t.Fatalf("attack desire on the character seen asleep = %v, want 200", got)
	}
	frames := w.advanceFrames(t, time.Second)
	want := ind8CameraPayload(golem.ObjectID(), 30, -200, 20, 6000, 8000, 0, 0, 0, 0)
	if got := ind8Pick(frames, serverpackets.OpcodeSpecialCamera); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("SpecialCamera frames 1 s after the golem appeared = %x, want %x", got, want)
	}
	w.srv.Advance(t, 9*time.Second)
	if got := ind1Slot(golem, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 9 s after the camera = %d, want 0", got)
	}
	w.srv.Advance(t, time.Second)
	if got := w.sees(t, golem); got != 400 {
		t.Fatalf("attack desire on the character seen awake = %v, want 400", got)
	}
}

// TestDrChaosGiganticGolemLeavesItsMaster ports DrChaosGiganticGolem's
// no-desire and dying hooks, on golems made the listener's privates. Left
// with nothing to do for more than 1200 s, a golem says its farewell
// (NpcString 1010582), tells its master event 10029 and leaves the world;
// dying, one says its last line (1010583) and tells its master 10029.
func TestDrChaosGiganticGolemLeavesItsMaster(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Boss(rb5GolemID, nil))
	w.draws.set(1)
	master := w.spawn(t, rb5ListenerID, -400, 0)
	idle := w.raise(t, master, rb5GolemID, 400, 0, script.Params{})
	w.srv.ReadQueued(t, w.srv.Client)
	w.ticks(t, 2)
	if !w.inWorld(idle) {
		t.Fatal("the golem left the world at once")
	}
	// The script clock is the server's uptime: the test moves the golem's
	// last waking tick 1201 s back instead.
	w.onQueue(t, idle, func() {
		n := script.NPCOf(idle)
		n.SetInt(script.SlotIntAI1, n.Int(script.SlotIntAI1)-1201)
	})
	frames := w.ticks(t, 2)
	if w.inWorld(idle) {
		t.Fatal("the golem stays in the world 1201 s after it last woke")
	}
	if got := says(frames, idle.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010582)}) {
		t.Fatalf("golem lines = %q, want its farewell", got)
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10029 arg1=0 arg2=0"}) {
		t.Fatalf("master heard %q, want event 10029", got)
	}

	dying := w.raise(t, master, rb5GolemID, 400, 0, script.Params{})
	w.srv.ReadQueued(t, w.srv.Client)
	w.kill(t, dying)
	if got := says(w.advanceFrames(t, 3500*time.Millisecond), dying.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010583)}) {
		t.Fatalf("dying golem lines = %q, want its last line", got)
	}
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10029 arg1=0 arg2=0"}) {
		t.Fatalf("master heard %q after the golem died, want event 10029", got)
	}
}

// TestRaidBossForTeleportDungeonTellsItsRoom ports RaidBossForTeleportDungeon,
// placed by the rift room's npcmaker: it tells the room 2 as it appears. A
// hit inside its territory wants the attacker; one out of it is not
// answered and leaves its last-hit tick. 180 s after it appeared it tells
// the room 2, the hit no more than 180 s old; a minute later, the hit more
// than 180 s old, 3.
// Dying, it tells the room 3.
func TestRaidBossForTeleportDungeonTellsItsRoom(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Boss(rb5RiftID, nil))
	w.draws.set(1)
	bosses := w.liveNPCs(rb5RiftID)
	if len(bosses) != 1 {
		t.Fatalf("rift bosses = %d, want 1", len(bosses))
	}
	boss := bosses[0]
	if got, want := w.makers.take(), []string{"dimension_gap_08 2"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as the boss appeared = %q, want %q", got, want)
	}
	w.hit(t, boss, 10)
	if got, want := w.attackWeight(boss), rbHate(10, 20); got != want {
		t.Fatalf("attack desire after a hit in territory = %v, want %v", got, want)
	}
	tick := ind1Slot(boss, script.SlotIntAI0)

	home := positionOf(boss)
	w.onQueue(t, boss, func() {
		script.NPCOf(boss).InstantTeleportTo(script.Loc{X: home.X + 3000, Y: home.Y, Z: home.Z})
	})
	w.srv.Advance(t, 5*time.Second)
	w.hit(t, boss, 10)
	if got, want := w.attackWeight(boss), rbHate(10, 20); got != want {
		t.Fatalf("attack desire after a hit out of territory = %v, want %v (unchanged)", got, want)
	}
	if got := ind1Slot(boss, script.SlotIntAI0); got != tick {
		t.Fatalf("last-hit tick after a hit out of territory = %d, want %d", got, tick)
	}
	w.onQueue(t, boss, func() { script.NPCOf(boss).InstantTeleportTo(script.Loc{X: home.X, Y: home.Y, Z: home.Z}) })

	w.srv.Advance(t, 175*time.Second)
	if got, want := w.makers.take(), []string{"dimension_gap_08 2"}; !slices.Equal(got, want) {
		t.Fatalf("maker events 180 s after the boss appeared = %q, want %q", got, want)
	}
	// The script clock is the server's uptime: the test moves the last
	// hit's tick 181 s back instead.
	w.onQueue(t, boss, func() { script.NPCOf(boss).SetInt(script.SlotIntAI0, tick-181) })
	w.srv.Advance(t, time.Minute)
	if got, want := w.makers.take(), []string{"dimension_gap_08 3"}; !slices.Equal(got, want) {
		t.Fatalf("maker events with the hit 181 s old = %q, want %q", got, want)
	}
	w.die(t, boss)
	if got, want := w.makers.take(), []string{"dimension_gap_08 3"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as the boss died = %q, want %q", got, want)
	}
}

// TestSailrenComesToItsLair ports Sailren's created, script event and timer
// hooks. Appearing, it runs 11041 on rune16_npc2017_01m1, and 1 s later its
// minute check, every draw 0, wants its self buff. Event 11042 jumps it into
// its lair, and 18 s later it wants social animation 2.
func TestSailrenComesToItsLair(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Boss(rb5SailrenID, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuffA: rbSelfBuff}))
	sailren := w.spawn(t, rb5SailrenID, 400, 0)
	if got, want := w.makers.take(), []string{"rune16_npc2017_01m1 11041"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as Sailren appeared = %q, want %q", got, want)
	}
	w.srv.Advance(t, time.Second)
	w.wantCasts(t, "after the first check", sailren, map[modelskill.ID]ind1Cast{rbSelfBuff: {sailren.ObjectID(), 1000000}})

	w.clearDesires(t, sailren)
	w.sendEvent(t, sailren, 11042, 1, 0)
	if got, want := positionOf(sailren), (location.Location{X: 27549, Y: -6638, Z: -2008}); got != want {
		t.Fatalf("Sailren stands at %v after event 11042, want %v", got, want)
	}
	w.srv.Advance(t, 17*time.Second)
	if _, ok := desire(sailren, actorai.IntentionSocial); ok {
		t.Fatal("social desire 17 s after event 11042, want none yet")
	}
	w.srv.Advance(t, time.Second)
	if d, ok := desire(sailren, actorai.IntentionSocial); !ok || d.Weight != 50 {
		t.Fatalf("social desire 18 s after event 11042 = %+v (%v), want weight 50", d, ok)
	}
}

// TestSailrenTurnsOnTheParty ports Sailren's event 11055: it wants to
// attack, at 200, each member of the named player's party standing in its
// spawn territory within 3000, the character and its mate, and from then on
// a creature it sees. Dying, it tells the NPCs within 8000 event 11054.
func TestSailrenTurnsOnTheParty(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Seer(rb5SailrenID))
	w.draws.set(1)
	sailren := w.spawn(t, rb5SailrenID, 100, 0)
	w.joinMate(t)
	members := script.PlayerOf(w.player).PartyMembers()
	if len(members) != 2 {
		t.Fatalf("setup: party members = %d, want 2", len(members))
	}
	w.sendEvent(t, sailren, 11055, w.player.ObjectID(), 0)
	for _, m := range members {
		obj, found := w.srv.State.Object(m.ObjectID())
		if !found {
			t.Fatalf("%s is not in the world", m.Name())
		}
		if d, ok := attackOn(sailren, obj.(attackable.Combatant)); !ok || d.Weight != 200 {
			t.Fatalf("attack desire on %s = %+v (%v), want 200", m.Name(), d, ok)
		}
	}
	if got := ind1Slot(sailren, script.SlotIntAI3); got != 1 {
		t.Fatalf("i_ai3 after event 11055 = %d, want 1", got)
	}
	if got := w.sees(t, sailren); got != 200 {
		t.Fatalf("attack desire on the character seen = %v, want 200", got)
	}

	w.spawn(t, rb5ListenerID, -400, 0)
	w.die(t, sailren)
	if got, want := w.heard.take(), []string{"event=11054 arg1=0 arg2=0"}; !slices.Equal(got, want) {
		t.Fatalf("events heard after Sailren died = %q, want %q", got, want)
	}
}

// TestSailrenGoesHomeWhenLeftAlone ports Sailren's 5 minute check: the
// 300 s check, with it touched just now, leaves it; the 600 s one, in its
// territory past 7 AI cycles and untouched for more than 600 s, runs 11046
// on rune16_npc2017_13m1, jumps it back to its waiting point and runs 11043
// on rune16_npc2017_01m1.
func TestSailrenGoesHomeWhenLeftAlone(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Boss(rb5SailrenID, nil))
	w.draws.set(1)
	sailren := w.spawn(t, rb5SailrenID, 400, 0)
	w.makers.take()
	w.ticks(t, 8)
	if got := script.NPCOf(sailren).LifeTime(); got <= 7 {
		t.Fatalf("setup: life time after 8 AI ticks = %d, want more than 7", got)
	}
	w.srv.Advance(t, 300*time.Second)
	if got := w.makers.take(); len(got) != 0 {
		t.Fatalf("maker events at 300 s = %q, want none", got)
	}
	// The script clock is the server's uptime: the test moves Sailren's
	// last touch 601 s back instead.
	w.onQueue(t, sailren, func() {
		n := script.NPCOf(sailren)
		n.SetInt(script.SlotIntAI1, n.Int(script.SlotIntAI1)-601)
	})
	w.srv.Advance(t, 300*time.Second)
	if got, want := w.makers.take(), []string{"rune16_npc2017_13m1 11046", "rune16_npc2017_01m1 11043"}; !slices.Equal(got, want) {
		t.Fatalf("maker events at 600 s = %q, want %q", got, want)
	}
	if got, want := positionOf(sailren), (location.Location{X: -113091, Y: -243942, Z: -15536}); got != want {
		t.Fatalf("Sailren stands at %v, want %v", got, want)
	}
}

// TestSailrenMutesAnOverLevelledCaster ports Sailren's see-spell hook: the
// level 20 character, more than 8 levels above a level 11 Sailren, casting
// in its sight makes it want its raid mute on the character, and nothing
// else.
func TestSailrenMutesAnOverLevelledCaster(t *testing.T) {
	t.Parallel()
	tmpl := rb5Boss(rb5SailrenID, nil)
	tmpl.Level = 11
	w := bootRaid5(t, raidSpellBoot(t), tmpl)
	w.draws.set(1)
	sailren := w.spawn(t, rb5SailrenID, 400, 0)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the mute wanted", func() bool {
		_, ok := desire(sailren, actorai.IntentionCast)
		return ok
	})
	w.wantCasts(t, "after the cast", sailren, map[modelskill.ID]ind1Cast{rbMuteID: {w.player.ObjectID(), 1000000}})
	if d, ok := attackOn(sailren, w.player); ok {
		t.Fatalf("attack desire on the caster = %+v, want none", d)
	}
}

// TestRaidBossAloneSummonPrivateBringsItsPrivatesBack ports
// RaidBossAloneSummonPrivate: it appears with its two privates, a weight
// point of 10 and i_ai0 at 0. A hit of 10 on a private makes it want the
// character at int(10 / (20 + 7) * 100) = 37. A dying private arms its
// respawn, counted in i_ai0, while i_ai0 is under 32; at 32 none does.
func TestRaidBossAloneSummonPrivateBringsItsPrivatesBack(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Leader(rb5SummonID, 20, nil), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb5SummonID, 400, 0)
	privates := privatesOf(t, boss, 2)
	if got := ind1Slot(boss, script.SlotWeightPoint); got != 10 {
		t.Fatalf("weight point = %d, want 10", got)
	}
	w.hit(t, privates[0], 10)
	if got := w.attackWeight(boss); got != 37 {
		t.Fatalf("attack desire after a hit of 10 on a private = %v, want 37", got)
	}

	w.kill(t, privates[0])
	if got := ind1Slot(boss, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after a private died = %d, want 1", got)
	}
	armed := w.armedPrivateRespawn(t)

	w.onQueue(t, boss, func() { script.NPCOf(boss).SetInt(script.SlotIntAI0, 32) })
	w.kill(t, privates[1])
	if got := w.armedPrivateRespawn(t); got != armed {
		t.Fatalf("respawn armed for %s after the 33rd death, want only the first private's %s", got, armed)
	}
	if got := ind1Slot(boss, script.SlotIntAI0); got != 32 {
		t.Fatalf("i_ai0 after the 33rd death = %d, want 32", got)
	}
}

// TestRaidBossPartyGuardsItsParty ports RaidBossParty's created and
// party-attacked hooks through RaidBossType2. A party boss appears a master
// of its two privates with a weight point of 10. A hit of 10 on a private
// by the character, at most 8 levels above the boss, makes it want the
// character at int(10 / 27 * 20000) = 7407; a boss 9 levels below wants
// nothing. On a draw of 0 in 150 a hit also wants its ranged DD magic on
// the character, as does a spell it sees.
func TestRaidBossPartyGuardsItsParty(t *testing.T) {
	t.Parallel()
	skills := map[npc.SkillType]modelskill.ID{npc.SkillTypeRangeDDMagicA: rb5RangeDD}
	w := bootRaid5(t, raidSpellBoot(t), rb5Leader(rb5Type2ID, 20, skills), rb5Leader(rb5Type2LowID, 11, nil), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb5Type2ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	if !script.NPCOf(boss).IsMaster() || ind1Slot(boss, script.SlotWeightPoint) != 10 {
		t.Fatalf("master %v, weight point %d, want a master of weight point 10", script.NPCOf(boss).IsMaster(), ind1Slot(boss, script.SlotWeightPoint))
	}
	w.hit(t, privates[0], 10)
	if got := w.attackWeight(boss); got != 7407 {
		t.Fatalf("attack desire after a hit of 10 on a private = %v, want 7407", got)
	}
	if _, ok := desire(boss, actorai.IntentionCast); ok {
		t.Fatal("cast desire on a draw of 1 in 150, want none")
	}

	low := w.spawn(t, rb5Type2LowID, -400, 0)
	w.hit(t, privatesOf(t, low, 2)[0], 10)
	if d, ok := attackOn(low, w.player); ok {
		t.Fatalf("attack desire of a boss 9 levels below = %+v, want none", d)
	}

	w.clearDesires(t, boss)
	w.draws.set(1, 0)
	w.hit(t, privates[1], 10)
	w.wantCasts(t, "after a hit on a draw of 0", boss, map[modelskill.ID]ind1Cast{rb5RangeDD: {w.player.ObjectID(), 1000000}})

	w.clearDesires(t, boss)
	w.draws.set(0)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the ranged DD magic wanted", func() bool {
		_, ok := desire(boss, actorai.IntentionCast)
		return ok
	})
	w.wantCasts(t, "after the spell seen", boss, map[modelskill.ID]ind1Cast{rb5RangeDD: {w.player.ObjectID(), 1000000}})
}

// TestRaidBossPartyJumpsWhenLost ports RaidBossParty's path-failure branch:
// after more than 8 + 12 failed paths (a draw of 12 in 13), a hit on a
// private makes the boss jump onto its top desire target within 1000, the
// character. Every other draw is 1.
func TestRaidBossPartyJumpsWhenLost(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Leader(rb5Type2ID, 20, nil), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb5Type2ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	w.hit(t, privates[0], 10)
	w.setTop(t, boss, script.PlayerOf(w.player))
	w.onQueue(t, boss, func() {
		for range 21 {
			boss.AddGeoPathFailCount()
		}
	})
	w.draws.set(1, 1, 12)
	w.hit(t, privates[0], 10)
	w.srv.AdvanceUntil(t, "the jump onto the character", func() bool { return w.onPlayer(t, boss) })
}

// TestRaidBossPartyHitInPeaceGoesHome ports RaidBossParty's peace zone
// branch: a hit on its party while the boss stands inside a peace zone
// sends it back to its spawn point with no attack desire left.
func TestRaidBossPartyHitInPeaceGoesHome(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	w := bootRaid5(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}}, rb5Leader(rb5Type2ID, 20, nil), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb5Type2ID, 400, 0)
	home := positionOf(boss)
	privates := privatesOf(t, boss, 2)
	w.teleport(t, boss, 600, 0)
	if !boss.InsideZone(zone.FlagPeace) || positionOf(boss) == home {
		t.Fatal("setup: the boss is not in the peace zone, off its spawn point")
	}
	w.hit(t, privates[0], 10)
	w.srv.AdvanceUntil(t, "the boss back home", func() bool { return positionOf(boss) == home })
	if d, ok := desire(boss, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after a hit in peace = %+v, want none", d)
	}
}

// TestRaidBossPartyBringsItsPrivatesBack ports RaidBossParty's party-died
// hook: a dying private of a live boss arms its respawn, which brings it
// back.
func TestRaidBossPartyBringsItsPrivatesBack(t *testing.T) {
	t.Parallel()
	w := bootRaid5(t, raidBoot{}, rb5Leader(rb5Type2ID, 20, nil), rb5PrivateTemplate())
	w.draws.set(1)
	boss := w.spawn(t, rb5Type2ID, 400, 0)
	privates := privatesOf(t, boss, 2)
	w.kill(t, privates[0])
	w.bringBack(t, privates[0])
	livePrivates(t, boss, 2)
}

// bringBack decays the dead private, whose spawn must have a respawn
// armed, and runs that respawn.
func (w *rb5World) bringBack(t *testing.T, private *npc.Hostile) {
	t.Helper()
	key := w.armedPrivateRespawn(t)
	w.onQueue(t, private, func() { private.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(private.ObjectID())) })
	w.srv.NpcSpawns.Respawn(key)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}
