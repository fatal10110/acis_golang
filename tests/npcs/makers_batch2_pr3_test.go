package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The last makers of batch 2 (#213): the dimensional rift rooms and the
// Royal Rush makers of the Four Sepulchers, booted through the batch's
// catalog file.

const (
	riftInvaderAID = 21659
	riftInvaderBID = 21661
	riftManagerID  = 31865
	riftListenKey  = "quest.RiftListener"
	// The room's memo names its manager in another case than the template
	// alias; the manager spawn's total of 3 shows the start loop's
	// shrinking bound. The manager comes first so the default 1001 tops it
	// up before the invaders take the room's maximum.
	riftSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rift_room" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="dimension_gap_00" territory="rift_room" maximumNpcs="6">
		<ai type="manage_teleport_dungeon">
			<set name="manager_npc_name" val="R00_INVADER_TELEPORT"/>
			<set name="room_index" val="00"/>
		</ai>
		<npc id="31865" total="3" pos="60;20;30;0" respawn="1min"/>
		<npc id="21659" total="2" pos="40;20;30;0" respawn="90sec"/>
		<npc id="21661" total="2" pos="50;20;30;0" respawn="80sec"/>
	</npcmaker>
</list>`
)

// riftLog records the script events the room's managers hear, in order.
type riftLog struct {
	mu       sync.Mutex
	received []alarmReceipt
}

func (l *riftLog) script() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventScriptEvent: {riftManagerID}},
		Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				l.mu.Lock()
				l.received = append(l.received, alarmReceipt{npcID: e.NPC.NpcID(), decayed: e.NPC.Decayed(), event: e.EventID, arg1: e.Arg1})
				l.mu.Unlock()
			},
		},
	}
}

func (l *riftLog) take() []alarmReceipt {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.received
	l.received = nil
	return out
}

// ManageTeleportDungeonMaker: at start the room places its manager, as many
// as the shrinking bound lets (2 of 3), and no invader. While the room is
// closed an invader entering the world leaves at once, so the default
// 1001 tops up only the manager. Event 1 opens the room and brings each
// invader spawn whose total still fits the maximum (the first, not the
// second); a second 1 does nothing. While open an invader leaving arms its
// respawn. Event 2 tells the managers 2. Event 0 closes the room and
// deletes the invaders; a manager leaving then arms no respawn. Event 3
// closes the room again, tells every manager kept 3, the one out of the
// world included, and deletes the invaders.
func TestManageTeleportDungeonMakerOpensAndClosesTheRoom(t *testing.T) {
	t.Parallel()
	log := &riftLog{}
	manager := spawnMonster(riftManagerID, "Guardian of Border")
	manager.Alias = "r00_invader_teleport"
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			spawnMonster(riftInvaderAID, "Invader A"), spawnMonster(riftInvaderBID, "Invader B"), manager,
		})),
		gameservertest.WithNpcSpawns(batch1Table(t, riftSpawnlist)),
		gameservertest.WithMakers(batch2Makers()),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{riftManagerID: script.KindHostile},
			[]script.Listing{{Path: riftListenKey}}, script.Catalog{riftListenKey: log.script}),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, invadersA, invadersB, managers int) {
		t.Helper()
		got := [3]int{len(mw.inWorld(riftInvaderAID)), len(mw.inWorld(riftInvaderBID)), len(mw.inWorld(riftManagerID))}
		if want := [3]int{invadersA, invadersB, managers}; got != want {
			t.Fatalf("%s: NPCs in the world (invader A, invader B, manager) = %v, want %v", step, got, want)
		}
	}
	event := func(name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("dimension_gap_00", name, 0, 0) {
			t.Fatal("npcmaker dimension_gap_00 not found by name")
		}
		w.srv.Advance(t, time.Millisecond)
	}
	armed := func(entry, n int) int {
		t.Helper()
		out := 0
		for i := range n {
			if w.srv.NpcRespawns.Tracked(fmt.Sprintf("dimension_gap_00#%d#%d", entry, i)) {
				out++
			}
		}
		return out
	}
	heard := func(ev int32, decayed bool) alarmReceipt {
		return alarmReceipt{npcID: riftManagerID, decayed: decayed, event: ev}
	}

	count("boot", 0, 0, 2)
	drainFrames(t, w.c)

	event("1001")
	count("1001 while closed", 0, 0, 3)
	drainFrames(t, w.c)

	event("1")
	count("1", 2, 0, 3)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(riftInvaderAID, 2)) {
		t.Fatalf("1: NPCs shown = %v, want two of invader A", got)
	}
	event("1")
	count("1 again", 2, 0, 3)

	mw.inWorld(riftInvaderAID)[0].DeleteMe()
	w.srv.Settle(t)
	count("an invader left", 1, 0, 3)
	if n := armed(1, 2); n != 1 {
		t.Fatalf("an invader left: respawns armed = %d, want 1", n)
	}
	for i := range 2 {
		fireRespawn(w.srv, fmt.Sprintf("dimension_gap_00#1#%d", i))
	}
	w.srv.Settle(t)
	count("invader respawn", 2, 0, 3)
	drainFrames(t, w.c)

	event("2")
	if got, want := log.take(), []alarmReceipt{heard(2, false), heard(2, false), heard(2, false)}; !slices.Equal(got, want) {
		t.Fatalf("2: receipts = %v, want %v", got, want)
	}
	count("2", 2, 0, 3)

	event("0")
	count("0", 0, 0, 3)
	if got := deleted(drainFrames(t, w.c)); len(got) != 2 {
		t.Fatalf("0: deleted objects = %v, want the two invaders", got)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("0: receipts = %v, want none", got)
	}

	mw.inWorld(riftManagerID)[0].DeleteMe()
	w.srv.Settle(t)
	count("a manager left while closed", 0, 0, 2)
	if n := armed(0, 3); n != 0 {
		t.Fatalf("a manager left while closed: respawns armed = %d, want 0", n)
	}
	drainFrames(t, w.c)

	event("1")
	count("1 reopens", 2, 2, 2)
	drainFrames(t, w.c)

	event("3")
	count("3", 0, 0, 2)
	got := log.take()
	slices.SortFunc(got, func(a, b alarmReceipt) int {
		if a.decayed == b.decayed {
			return 0
		}
		if a.decayed {
			return 1
		}
		return -1
	})
	if want := []alarmReceipt{heard(3, false), heard(3, false), heard(3, true)}; !slices.Equal(got, want) {
		t.Fatalf("3: receipts = %v, want %v", got, want)
	}
	if got := deleted(drainFrames(t, w.c)); len(got) != 4 {
		t.Fatalf("3: deleted objects = %v, want the four invaders", got)
	}
	if n := armed(1, 2) + armed(2, 2); n != 0 {
		t.Fatalf("3: invader respawns armed = %d, want 0", n)
	}
}

// royalClock is a probe npcmaker that reads the clock the makers run on,
// started with them.
type royalClock struct {
	mu    sync.Mutex
	maker *script.Maker
	start time.Time
}

func (c *royalClock) catalog() script.MakerCatalog {
	out := batch2Makers()
	out["royal_clock_probe"] = func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{OnStart: func(m *script.Maker, _ script.MakerStart) {
			c.mu.Lock()
			c.maker, c.start = m, m.Now()
			c.mu.Unlock()
		}}}
	}
	return out
}

func (c *royalClock) now(t *testing.T) (start, now time.Time) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maker == nil {
		t.Fatal("the clock probe never started")
	}
	return c.start, c.maker.Now()
}

// toTick advances the clock to the first run of the makers' one-second
// timers that falls in second sec of minute minute.
func (c *royalClock) toTick(t *testing.T, srv *gameservertest.Server, minute, sec int) {
	t.Helper()
	start, now := c.now(t)
	at := now.Truncate(time.Hour).Add(time.Duration(minute)*time.Minute + time.Duration(sec)*time.Second)
	if !at.After(now) {
		at = at.Add(time.Hour)
	}
	tick := start.Add((at.Sub(start) + time.Second - 1) / time.Second * time.Second)
	if tick.Minute() != minute || tick.Second() != sec {
		t.Fatalf("tick %v of the timers started %v is not in %02d:%02d", tick, start, minute, sec)
	}
	srv.Advance(t, tick.Sub(now))
}

const (
	royalGuardID = 18146
	royalOnceID  = 18148
	royalRoomLst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="royal_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="royal_clock" territory="royal_hall" maximumNpcs="1">
		<ai type="royal_clock_probe"/>
	</npcmaker>
	<npcmaker name="royal_room" territory="royal_hall" maximumNpcs="3">
		<ai type="royal_rush_maker"/>
		<npc id="18146" total="2" pos="40;20;30;0" respawn="60sec"/>
		<npc id="18148" total="1" pos="50;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// RoyalRushMaker: nothing spawns at start. The default 1001 brings the
// group; in minute 5 a leaving NPC respawns as the default maker has it,
// unless its spawn's respawn delay is 0. In minute 52 a leaving NPC arms no
// respawn and an NPC entering the world leaves at once. Timer 3000 at
// minute 54 second 0 deletes the rest; at minute 0 the default 1001 brings
// the group again.
func TestRoyalRushMakerKeepsTheHallOnTheClock(t *testing.T) {
	t.Parallel()
	clock := &royalClock{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(royalGuardID, royalOnceID)),
		gameservertest.WithNpcSpawns(batch1Table(t, royalRoomLst)),
		gameservertest.WithMakers(clock.catalog()),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the wall clock cannot be moved to a given minute on the real pool")
	}
	mw := &makerWorld{srv: w.srv}
	count := func(step string, guards, once int) {
		t.Helper()
		got := [2]int{len(mw.inWorld(royalGuardID)), len(mw.inWorld(royalOnceID))}
		if want := [2]int{guards, once}; got != want {
			t.Fatalf("%s: NPCs in the world (guard, once) = %v, want %v", step, got, want)
		}
	}
	bring := func() {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("royal_room", "1001", 0, 0) {
			t.Fatal("npcmaker royal_room not found by name")
		}
		w.srv.Advance(t, time.Millisecond)
	}
	guardsArmed := func() int {
		n := 0
		for i := range 2 {
			if w.srv.NpcRespawns.Tracked(fmt.Sprintf("royal_room#0#%d", i)) {
				n++
			}
		}
		return n
	}

	count("boot", 0, 0)
	clock.toTick(t, w.srv, 5, 0)
	drainFrames(t, w.c)
	bring()
	count("1001 at minute 5", 2, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{royalGuardID, royalGuardID, royalOnceID}) {
		t.Fatalf("1001 at minute 5: NPCs shown = %v", got)
	}

	mw.inWorld(royalGuardID)[0].DeleteMe()
	mw.inWorld(royalOnceID)[0].DeleteMe()
	w.srv.Settle(t)
	if n := guardsArmed(); n != 1 {
		t.Fatalf("a guard left at minute 5: respawns armed = %d, want 1", n)
	}
	if w.srv.NpcRespawns.Tracked("royal_room#1#0") {
		t.Fatal("an NPC of a spawn with no respawn delay armed a respawn")
	}
	for i := range 2 {
		fireRespawn(w.srv, fmt.Sprintf("royal_room#0#%d", i))
	}
	w.srv.Settle(t)
	count("guard respawn", 2, 0)

	clock.toTick(t, w.srv, 52, 0)
	drainFrames(t, w.c)
	mw.inWorld(royalGuardID)[0].DeleteMe()
	w.srv.Settle(t)
	if n := guardsArmed(); n != 0 {
		t.Fatalf("a guard left at minute 52: respawns armed = %d, want 0", n)
	}
	drainFrames(t, w.c)
	bring()
	count("1001 at minute 52", 1, 0)
	frames := drainFrames(t, w.c)
	if got := shown(frames); !slices.Equal(got, []int{royalOnceID}) {
		t.Fatalf("1001 at minute 52: NPCs shown = %v, want the one that leaves", got)
	}
	if got := deleted(frames); len(got) != 1 {
		t.Fatalf("1001 at minute 52: deleted objects = %v, want the one shown", got)
	}

	clock.toTick(t, w.srv, 53, 59)
	count("minute 53 second 59", 1, 0)
	drainFrames(t, w.c)
	clock.toTick(t, w.srv, 54, 0)
	count("minute 54 second 0", 0, 0)
	if got := deleted(drainFrames(t, w.c)); len(got) != 1 {
		t.Fatalf("minute 54 second 0: deleted objects = %v, want the last guard", got)
	}

	clock.toTick(t, w.srv, 0, 0)
	bring()
	count("1001 at minute 0", 2, 1)
}

const (
	royalBossID   = 25339
	royalEscortID = 18224
	royalChestID  = 18256
	royalChainLst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="royal_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="royal_clock" territory="royal_hall" maximumNpcs="1">
		<ai type="royal_clock_probe"/>
	</npcmaker>
	<npcmaker name="1st_type1_b1" territory="royal_hall" maximumNpcs="2">
		<ai type="royal_req_next_maker">
			<set name="req_count" val="0"/>
			<set name="next_maker_name" val="1ST_BOSS_TYPE1"/>
		</ai>
		<npc id="18146" total="2" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="1st_boss_type1" territory="royal_hall" maximumNpcs="10">
		<ai type="royal_req_next_maker">
			<set name="next_maker_name" val="1st_boss_treasure"/>
			<set name="BossMaker" val="1"/>
			<set name="req_count" val="0"/>
		</ai>
		<npc id="25339" total="1" pos="60;20;30;0" respawn="0sec"/>
		<npc id="18224" total="1" pos="70;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="1st_boss_treasure" territory="royal_hall" maximumNpcs="200">
		<ai type="royal_spawn_treasurebox"/>
		<npc id="18256" total="200" pos="80;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// RoyalReqNextMaker and RoyalSpawnTreasureBoxMaker, with the random draw
// pinned to 9: the room's last guard leaving in minute 10 (its req_count
// of 0 reached) runs 1001 on the boss npcmaker its memo names, whatever the
// case. The boss maker's escort leaving arms no respawn and calls nothing;
// the boss leaving runs 1002 on the treasure npcmaker, which brings 9 + 157
// chests. At minute 48 a 1002 brings 9 + 10 more, and the next one none
// (they would not fit 200). The room's last guard leaving in minute 51
// calls nothing, and at minute 54 the chests stay: the treasure maker
// answers no 1000.
func TestRoyalReqNextMakersChainToTheTreasure(t *testing.T) {
	t.Parallel()
	clock := &royalClock{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(royalGuardID, royalBossID, royalEscortID, royalChestID)),
		gameservertest.WithNpcSpawns(batch1Table(t, royalChainLst)),
		gameservertest.WithMakers(clock.catalog()),
		gameservertest.WithScriptRand(func(n int) int { return n - 1 }),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the wall clock cannot be moved to a given minute on the real pool")
	}
	mw := &makerWorld{srv: w.srv}
	count := func(step string, guards, bosses, escorts, chests int) {
		t.Helper()
		got := [4]int{len(mw.inWorld(royalGuardID)), len(mw.inWorld(royalBossID)), len(mw.inWorld(royalEscortID)), len(mw.inWorld(royalChestID))}
		if want := [4]int{guards, bosses, escorts, chests}; got != want {
			t.Fatalf("%s: NPCs in the world (guard, boss, escort, chest) = %v, want %v", step, got, want)
		}
	}
	event := func(maker, name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent(maker, name, 0, 0) {
			t.Fatalf("npcmaker %s not found by name", maker)
		}
		w.srv.Advance(t, time.Millisecond)
	}
	leave := func(id int) {
		t.Helper()
		mw.inWorld(id)[0].DeleteMe()
		w.srv.Settle(t)
	}

	count("boot", 0, 0, 0, 0)
	clock.toTick(t, w.srv, 10, 0)
	event("1st_type1_b1", "1001")
	count("1001 at minute 10", 2, 0, 0, 0)
	leave(royalGuardID)
	count("a guard left", 1, 0, 0, 0)
	leave(royalGuardID)
	w.srv.Advance(t, time.Millisecond)
	count("the last guard left at minute 10", 0, 1, 1, 0)
	drainFrames(t, w.c)

	leave(royalEscortID)
	count("the escort left", 0, 1, 0, 0)
	if w.srv.NpcRespawns.Tracked("1st_boss_type1#1#0") {
		t.Fatal("the escort leaving armed a respawn")
	}
	leave(royalBossID)
	count("the boss left", 0, 0, 0, 166)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(royalChestID, 166)) {
		t.Fatalf("the boss left: %d NPCs shown, want 166 chests", len(got))
	}

	clock.toTick(t, w.srv, 48, 0)
	event("1st_boss_treasure", "1002")
	count("1002 at minute 48", 0, 0, 0, 185)
	event("1st_boss_treasure", "1002")
	count("1002 that does not fit", 0, 0, 0, 185)

	event("1st_type1_b1", "1001")
	count("1001 at minute 48", 2, 0, 0, 185)
	clock.toTick(t, w.srv, 51, 0)
	drainFrames(t, w.c)
	leave(royalGuardID)
	leave(royalGuardID)
	w.srv.Advance(t, time.Millisecond)
	count("the last guard left at minute 51", 0, 0, 0, 185)
	if got := shown(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("the last guard left at minute 51: NPCs shown = %v, want none", got)
	}

	clock.toTick(t, w.srv, 54, 2)
	count("minute 54", 0, 0, 0, 185)
}
