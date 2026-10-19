package npcs

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The second set of batch 1 makers (#212): the door-open makers, the wave
// series makers and the day/night maker, booted through the batch's
// catalog file.

const (
	openDoorLeaderID  = 21651
	openDoorArcherID  = 21652
	openDoorStrayID   = 21653
	openDoorSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="band_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="band_leader" territory="band_field" maximumNpcs="2">
		<ai type="undead_band_leader_maker"><set name="DoorName" val="MAKER_GATE"/></ai>
		<npc id="21651" total="3" pos="40;20;30;0" respawn="no"/>
	</npcmaker>
	<npcmaker name="band_archer" territory="band_field" maximumNpcs="1">
		<ai type="undead_band_archer_wazird_maker"><set name="DoorName" val="maker_gate"/></ai>
		<npc id="21652" total="1" pos="50;20;30;0" respawn="no"/>
	</npcmaker>
	<npcmaker name="band_stray" territory="band_field" maximumNpcs="1">
		<ai type="undead_band_lance_maker"/>
		<npc id="21653" total="1" pos="60;20;30;0" respawn="no"/>
	</npcmaker>
</list>`
)

// OpenDoorMaker (the three undead_band types): nothing spawns at start;
// each npcmaker registers on the door its DoorName memo names (ignoring
// case). The door opening spawns each group up to its maker's maximum (two
// of the three leaders, the archer) and the character is shown them; an
// NPC leaving arms no respawn. The door closing deletes the rest, and an
// NPC the default script event 1001 then brings is deleted as it appears.
// Opening again brings the groups back. An npcmaker with no DoorName never
// spawns.
func TestOpenDoorMakerFollowsItsDoor(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithDoors(makerGate()),
		gameservertest.WithNPCs(batch1Templates(openDoorLeaderID, openDoorArcherID, openDoorStrayID)),
		gameservertest.WithNpcSpawns(batch1Table(t, openDoorSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	world := func(step string, wantLeaders, wantArchers int) {
		t.Helper()
		if got := len(mw.inWorld(openDoorLeaderID)); got != wantLeaders {
			t.Fatalf("%s: leaders in the world = %d, want %d", step, got, wantLeaders)
		}
		if got := len(mw.inWorld(openDoorArcherID)); got != wantArchers {
			t.Fatalf("%s: archers in the world = %d, want %d", step, got, wantArchers)
		}
		if got := len(mw.inWorld(openDoorStrayID)); got != 0 {
			t.Fatalf("%s: stray NPCs in the world = %d, want 0", step, got)
		}
	}
	world("boot", 0, 0)
	drainFrames(t, w.c)

	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	w.srv.Settle(t)
	world("door opened", 2, 1)
	want := append(repeatID(openDoorLeaderID, 2), openDoorArcherID)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, want) {
		t.Fatalf("door opened: NPCs shown = %v, want %v", got, want)
	}

	leaders := mw.inWorld(openDoorLeaderID)
	leaders[0].DeleteMe()
	w.srv.Settle(t)
	for i := range 3 {
		if key := fmt.Sprintf("band_leader#0#%d", i); w.srv.NpcRespawns.Tracked(key) {
			t.Fatalf("slot %s armed a respawn", key)
		}
	}
	w.srv.Advance(t, time.Minute)
	world("a leader left", 1, 1)
	drainFrames(t, w.c)

	archer := mw.inWorld(openDoorArcherID)[0]
	w.srv.WorldObjects.SetDoorOpen(makerGateID, false)
	w.srv.Settle(t)
	world("door closed", 0, 0)
	wantGone := objectIDs([]*npc.Hostile{leaders[1], archer})
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, wantGone) {
		t.Fatalf("door closed: deleted objects = %v, want %v", got, wantGone)
	}

	if !w.srv.NpcSpawns.MakerEvent("band_leader", "1001", 0, 0) {
		t.Fatal("npcmaker band_leader not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	world("1001 while the door is closed", 0, 0)
	frames := drainFrames(t, w.c)
	if got, gone := shown(frames), deleted(frames); !slices.Equal(got, repeatID(openDoorLeaderID, 2)) || len(gone) != 2 {
		t.Fatalf("1001 while the door is closed: shown %v and deleted %v, want two leaders shown and both deleted", got, gone)
	}

	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	w.srv.Settle(t)
	world("door opened again", 2, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, want) {
		t.Fatalf("door opened again: NPCs shown = %v, want %v", got, want)
	}
}

const (
	waveFirstID   = 21661
	waveLastID    = 21662
	waveSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="wave_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="wave_first" territory="wave_field" maximumNpcs="1">
		<ai type="maker_instant_spawn_serial">
			<set name="on_start_spawn" val="1"/>
			<set name="loop_cnt" val="2"/>
			<set name="maker_name" val="WAVE_LAST"/>
		</ai>
		<npc id="21661" total="1" pos="40;20;30;0" respawn="30sec"/>
	</npcmaker>
	<npcmaker name="wave_last" territory="wave_field" maximumNpcs="1">
		<ai type="maker_instant_spawn_serial_last">
			<set name="on_start_spawn" val="0"/>
			<set name="loop_cnt" val="1"/>
			<set name="maker_name" val="wave_first"/>
		</ai>
		<npc id="21662" total="1" pos="50;20;30;0" respawn="20sec"/>
	</npcmaker>
</list>`
	waveFirstSlot = "wave_first#0#0"
)

// InstantSpawnSerialMaker and InstantSpawnSerialLastMaker, chained as the
// shipped godard14 pairs are: the series maker spawns at start and, each
// time its group's last NPC leaves, brings it back loop_cnt (2) more times
// through its own script event 1001 with that NPC's respawn delay; it
// ignores 1000. Its last NPC of the final wave instead sends 1001 to the
// last maker (named in the other case), whose group comes in after the
// 30 second delay. The last maker's group leaving sends 1001 with int2 1
// back, which restarts the series: two more waves before the last maker
// is woken again.
func TestInstantSpawnSerialMakersChainTheirWaves(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(waveFirstID, waveLastID)),
		gameservertest.WithNpcSpawns(batch1Table(t, waveSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, wantFirst, wantLast int) {
		t.Helper()
		if got := len(mw.inWorld(waveFirstID)); got != wantFirst {
			t.Fatalf("%s: series NPCs = %d, want %d", step, got, wantFirst)
		}
		if got := len(mw.inWorld(waveLastID)); got != wantLast {
			t.Fatalf("%s: last-maker NPCs = %d, want %d", step, got, wantLast)
		}
	}
	// nextWave kills the series NPC and expects the series to bring it
	// back through its own 1001: the NPC's respawn is armed and, fired,
	// shows it again.
	nextWave := func(step string) {
		t.Helper()
		first := mw.inWorld(waveFirstID)[0]
		first.DeleteMe()
		w.srv.Settle(t)
		count(step+": the wave left", 0, 0)
		if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{first.ObjectID()}) {
			t.Fatalf("%s: deleted objects = %v, want [%d]", step, got, first.ObjectID())
		}
		if !fireRespawn(w.srv, waveFirstSlot) {
			t.Fatalf("%s: the series armed no respawn of its NPC", step)
		}
		w.srv.Settle(t)
		count(step+": the next wave", 1, 0)
		if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{waveFirstID}) {
			t.Fatalf("%s: NPCs shown = %v, want [%d]", step, got, waveFirstID)
		}
	}
	count("boot", 1, 0)
	drainFrames(t, w.c)

	w.srv.NpcSpawns.MakerEvent("wave_first", "1000", 0, 0)
	w.srv.Settle(t)
	count("1000 to the series", 1, 0)

	nextWave("first loop")
	nextWave("second loop")

	final := mw.inWorld(waveFirstID)[0]
	final.DeleteMe()
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked(waveFirstSlot) {
		t.Fatal("the final wave armed a respawn of the series NPC")
	}
	w.srv.Advance(t, 29*time.Second)
	count("before the last maker's delay", 0, 0)
	drainFrames(t, w.c)
	w.srv.Advance(t, time.Second)
	count("after the last maker's delay", 0, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{waveLastID}) {
		t.Fatalf("last maker woken: NPCs shown = %v, want [%d]", got, waveLastID)
	}

	last := mw.inWorld(waveLastID)[0]
	last.DeleteMe()
	w.srv.Settle(t)
	if !fireRespawn(w.srv, waveFirstSlot) {
		t.Fatal("the last maker's group leaving did not bring the series back")
	}
	w.srv.Settle(t)
	count("series restarted", 1, 0)
	drainFrames(t, w.c)

	nextWave("restarted first loop")
	nextWave("restarted second loop")
	// The last maker keeps its NPC out of the world: this time 1001 arms
	// that NPC's respawn instead of placing a new one.
	mw.inWorld(waveFirstID)[0].DeleteMe()
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked(waveFirstSlot) || !fireRespawn(w.srv, "wave_last#0#0") {
		t.Fatal("the restarted series' final wave did not wake the last maker")
	}
	w.srv.Settle(t)
	count("restarted series done", 0, 1)
}

const (
	nightOnlyID     = 21671
	dayOnlyID       = 21672
	dayNightSpawnls = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="dusk_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="night_m" territory="dusk_field" maximumNpcs="1">
		<ai type="on_day_night_spawn"><set name="IsNight" val="1"/></ai>
		<npc id="21671" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="day_m" territory="dusk_field" maximumNpcs="1">
		<ai type="on_day_night_spawn"><set name="IsNight" val="0"/></ai>
		<npc id="21672" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// OnDayNightSpawnMaker: at start only the npcmaker whose IsNight memo fits
// the time of day spawns. Once the time of day turns, the check one
// second after start deletes that group (1000) and brings the other in
// (1001); later checks, every minute, change nothing while the time of day
// holds. An NPC leaving while its time fits respawns; an NPC the other
// maker gets from an outside 1001 is deleted as it appears, and arms no
// respawn.
func TestOnDayNightSpawnMakerFollowsTheTimeOfDay(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(nightOnlyID, dayOnlyID)),
		gameservertest.WithNpcSpawns(batch1Table(t, dayNightSpawnls)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	night := len(mw.inWorld(nightOnlyID)) == 1
	// inID and outID are the templates of the maker in and out of the
	// world for the current time of day; inMaker and outMaker their names.
	inID, outID, inMaker, outMaker := dayOnlyID, nightOnlyID, "day_m", "night_m"
	if night {
		inID, outID, inMaker, outMaker = outID, inID, outMaker, inMaker
	}
	count := func(step string, wantIn, wantOut int) {
		t.Helper()
		if got := len(mw.inWorld(inID)); got != wantIn {
			t.Fatalf("%s: NPCs of %s = %d, want %d", step, inMaker, got, wantIn)
		}
		if got := len(mw.inWorld(outID)); got != wantOut {
			t.Fatalf("%s: NPCs of %s = %d, want %d", step, outMaker, got, wantOut)
		}
	}
	count("boot (night "+map[bool]string{true: "yes", false: "no"}[night]+")", 1, 0)
	drainFrames(t, w.c)

	if fell := w.srv.CrossDayNight(t); fell == night {
		t.Fatalf("CrossDayNight() = %v from night %v", fell, night)
	}
	w.srv.Settle(t)
	count("time of day turned, before the check", 1, 0)
	gone := mw.inWorld(inID)[0]
	w.srv.Advance(t, time.Second)
	count("first check", 0, 1)
	frames := drainFrames(t, w.c)
	if got := deleted(frames); !slices.Equal(got, []int32{gone.ObjectID()}) {
		t.Fatalf("first check: deleted objects = %v, want [%d]", got, gone.ObjectID())
	}
	if got := shown(frames); !slices.Equal(got, []int{outID}) {
		t.Fatalf("first check: NPCs shown = %v, want [%d]", got, outID)
	}
	inID, outID, inMaker, outMaker = outID, inID, outMaker, inMaker

	in := mw.inWorld(inID)[0]
	in.DeleteMe()
	w.srv.Settle(t)
	slot := inMaker + "#0#0"
	if !fireRespawn(w.srv, slot) {
		t.Fatalf("%s's NPC left in its time and armed no respawn", inMaker)
	}
	w.srv.Settle(t)
	count("respawn in its time", 1, 0)
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Minute)
	count("next check", 1, 0)
	if frames := drainFrames(t, w.c); len(shown(frames)) != 0 || len(deleted(frames)) != 0 {
		t.Fatalf("next check: shown %v and deleted %v, want none", shown(frames), deleted(frames))
	}

	if !w.srv.NpcSpawns.MakerEvent(outMaker, "1001", 0, 0) {
		t.Fatalf("npcmaker %s not found by name", outMaker)
	}
	w.srv.Advance(t, time.Millisecond)
	count("outside 1001 out of its time", 1, 0)
	frames = drainFrames(t, w.c)
	if got, gone := shown(frames), deleted(frames); !slices.Equal(got, []int{outID}) || len(gone) != 1 {
		t.Fatalf("outside 1001 out of its time: shown %v and deleted %v, want one NPC of %d shown and deleted", got, gone, outID)
	}
	if w.srv.NpcRespawns.Tracked(outMaker + "#0#0") {
		t.Fatalf("%s's NPC deleted out of its time armed a respawn", outMaker)
	}
}

// fireRespawn fires the respawn armed for slot, as the respawn ticker
// would once it came due, and reports whether one was armed. The fixture's
// respawn timers never fire on their own.
func fireRespawn(srv *gameservertest.Server, slot string) bool {
	if !srv.NpcRespawns.Cancel(slot) {
		return false
	}
	srv.NpcSpawns.Respawn(slot)
	return true
}
