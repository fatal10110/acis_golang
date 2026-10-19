package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The first set of batch 2 makers (#213): the Imperial Tomb alarm devices
// and hall keepers and the Rainbow Springs hall maker, booted through the
// batch's catalog file.

// batch2Makers is the batch's catalog file without the types not ported
// yet, as the composition root joins it.
func batch2Makers() script.MakerCatalog {
	out := script.MakerCatalog{}
	for typ, ctor := range catalog.Makers()["makers_2"] {
		if ctor != nil {
			out[typ] = ctor
		}
	}
	return out
}

// The devices carry one template each, so a receipt names its device:
// 22m1 runs alarm_device1_maker, 19m1 alarm_device2_maker, 20m1
// alarm_device3_maker and 21m1 alarm_device4_maker, as the shipped
// Imperial Tomb spawnlist binds them. Their respawn delay would arm a
// respawn under the default maker.
const (
	alarm1ID       = 21681
	alarm2ID       = 21682
	alarm3ID       = 21683
	alarm4ID       = 21684
	alarmSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="tomb_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="godard32_2515_19m1" territory="tomb_field" maximumNpcs="1">
		<ai type="alarm_device2_maker"/>
		<npc id="21682" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="godard32_2515_20m1" territory="tomb_field" maximumNpcs="1">
		<ai type="alarm_device3_maker"/>
		<npc id="21683" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="godard32_2515_21m1" territory="tomb_field" maximumNpcs="1">
		<ai type="alarm_device4_maker"/>
		<npc id="21684" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="godard32_2515_22m1" territory="tomb_field" maximumNpcs="1">
		<ai type="alarm_device1_maker"/>
		<npc id="21681" total="1" pos="70;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
	alarmListenerKey = "quest.AlarmListener"
)

// alarmReceipt is one script event a device NPC heard.
type alarmReceipt struct {
	npcID   int32
	decayed bool
	event   int32
	arg1    int32
}

func (r alarmReceipt) String() string {
	return fmt.Sprintf("%d:%d(%d) decayed=%v", r.npcID, r.event, r.arg1, r.decayed)
}

// alarmLog records the script events the device NPCs hear, in order.
type alarmLog struct {
	mu       sync.Mutex
	received []alarmReceipt
}

func (l *alarmLog) script() script.Script {
	ids := []int32{alarm1ID, alarm2ID, alarm3ID, alarm4ID}
	return script.Script{
		Bind: script.Bindings{script.EventScriptEvent: ids},
		Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				l.mu.Lock()
				l.received = append(l.received, alarmReceipt{npcID: e.NPC.NpcID(), decayed: e.NPC.Decayed(), event: e.EventID, arg1: e.Arg1})
				l.mu.Unlock()
			},
		},
	}
}

func (l *alarmLog) take() []alarmReceipt {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.received
	l.received = nil
	return out
}

// AlarmDeviceMaker1-4: no device spawns at start, and the timer three
// seconds after start reaches no NPC. A 1001 brings a device's NPC and,
// three seconds later, it hears 10025 with 1. The last NPC of a device
// leaving arms no respawn: the device raises 10008 on itself, which adds
// one to its count and tells its NPC (out of the world, as its last life),
// devices 2 and 3 tell it a second time, and then the device raises 10008
// on the other three by name, in its own order, each telling its NPC its
// new count; the lost device's count drops to 0. A 1001 to a lost device
// sets its count back to 1, arms its NPC's respawn after int1 seconds and
// tells the NPC 10025 with 1 three seconds later.
func TestAlarmDeviceMakersRaiseEachOther(t *testing.T) {
	t.Parallel()
	log := &alarmLog{}
	kinds := map[int32]script.NPCKind{alarm1ID: script.KindHostile, alarm2ID: script.KindHostile, alarm3ID: script.KindHostile, alarm4ID: script.KindHostile}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(alarm1ID, alarm2ID, alarm3ID, alarm4ID)),
		gameservertest.WithNpcSpawns(batch1Table(t, alarmSpawnlist)),
		gameservertest.WithMakers(batch2Makers()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: alarmListenerKey}}, script.Catalog{alarmListenerKey: log.script}),
	)
	mw := &makerWorld{srv: w.srv}
	devices := []struct {
		maker string
		id    int
	}{
		{"godard32_2515_22m1", alarm1ID},
		{"godard32_2515_19m1", alarm2ID},
		{"godard32_2515_20m1", alarm3ID},
		{"godard32_2515_21m1", alarm4ID},
	}
	alive := func(step string, want ...int) {
		t.Helper()
		var got []int
		for _, d := range devices {
			if len(mw.inWorld(d.id)) == 1 {
				got = append(got, d.id)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: devices in the world = %v, want %v", step, got, want)
		}
	}
	expect := func(step string, want ...alarmReceipt) {
		t.Helper()
		if got := log.take(); !slices.Equal(got, want) {
			t.Fatalf("%s: receipts = %v, want %v", step, got, want)
		}
	}
	heard := func(id int, arg1 int32, decayed bool) alarmReceipt {
		return alarmReceipt{npcID: int32(id), decayed: decayed, event: 10025, arg1: arg1}
	}
	alive("boot")
	w.srv.Advance(t, 3*time.Second)
	expect("start timers")
	drainFrames(t, w.c)

	for _, d := range devices {
		if !w.srv.NpcSpawns.MakerEvent(d.maker, "1001", 0, 0) {
			t.Fatalf("npcmaker %s not found by name", d.maker)
		}
	}
	w.srv.Advance(t, time.Millisecond)
	alive("1001 to every device", alarm1ID, alarm2ID, alarm3ID, alarm4ID)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{alarm1ID, alarm2ID, alarm3ID, alarm4ID}) {
		t.Fatalf("1001 to every device: NPCs shown = %v", got)
	}
	expect("before the 1001 timers")
	w.srv.Advance(t, 3*time.Second-time.Millisecond)
	got := log.take()
	slices.SortFunc(got, func(a, b alarmReceipt) int { return int(a.npcID - b.npcID) })
	if want := []alarmReceipt{heard(alarm1ID, 1, false), heard(alarm2ID, 1, false), heard(alarm3ID, 1, false), heard(alarm4ID, 1, false)}; !slices.Equal(got, want) {
		t.Fatalf("1001 timers: receipts = %v, want %v", got, want)
	}

	lose := func(step string, d int) {
		t.Helper()
		gone := mw.inWorld(devices[d].id)[0]
		gone.DeleteMe()
		w.srv.Settle(t)
		if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{gone.ObjectID()}) {
			t.Fatalf("%s: deleted objects = %v, want [%d]", step, got, gone.ObjectID())
		}
		if slot := devices[d].maker + "#0#0"; w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("%s: slot %s armed a respawn", step, slot)
		}
	}

	lose("device 1 lost", 0)
	alive("device 1 lost", alarm2ID, alarm3ID, alarm4ID)
	expect("device 1 lost",
		heard(alarm1ID, 2, true),
		heard(alarm2ID, 2, false), heard(alarm3ID, 2, false), heard(alarm4ID, 2, false))

	lose("device 2 lost", 1)
	alive("device 2 lost", alarm3ID, alarm4ID)
	expect("device 2 lost",
		heard(alarm2ID, 3, true), heard(alarm2ID, 3, true),
		heard(alarm1ID, 1, true), heard(alarm3ID, 3, false), heard(alarm4ID, 3, false))

	lose("device 3 lost", 2)
	alive("device 3 lost", alarm4ID)
	expect("device 3 lost",
		heard(alarm3ID, 4, true), heard(alarm3ID, 4, true),
		heard(alarm2ID, 1, true), heard(alarm1ID, 2, true), heard(alarm4ID, 4, false))

	lose("device 4 lost", 3)
	alive("device 4 lost")
	expect("device 4 lost",
		heard(alarm4ID, 5, true),
		heard(alarm2ID, 2, true), heard(alarm3ID, 1, true), heard(alarm1ID, 3, true))

	w.srv.NpcSpawns.MakerEvent("godard32_2515_22m1", "1001", 5, 0)
	w.srv.Settle(t)
	if !fireRespawn(w.srv, "godard32_2515_22m1#0#0") {
		t.Fatal("1001 to the lost device 1 armed no respawn of its NPC")
	}
	w.srv.Settle(t)
	alive("device 1 back", alarm1ID)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{alarm1ID}) {
		t.Fatalf("device 1 back: NPCs shown = %v, want [%d]", got, alarm1ID)
	}
	w.srv.Advance(t, 3*time.Second)
	expect("device 1 back, its timer", heard(alarm1ID, 1, false))
}

const (
	hallKeeperID        = 21691
	hallKeeperSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="hall_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="hall_wizards" territory="hall_field" maximumNpcs="2">
		<ai type="hall_keeper_wizard_maker"><set name="on_start_spawn" val="1"/></ai>
		<npc id="21691" total="2" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// HallKeeperDefaultMaker: nothing spawns at start, its on_start_spawn
// memo notwithstanding. The default maker's 1001 brings the group up to
// its maximum, an NPC leaving arms its respawn as the default maker does,
// and 1000 deletes the group.
func TestHallKeeperMakerSpawnsOnlyOnEvent(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(hallKeeperID)),
		gameservertest.WithNpcSpawns(batch1Table(t, hallKeeperSpawnlist)),
		gameservertest.WithMakers(batch2Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(hallKeeperID)); got != want {
			t.Fatalf("%s: hall keepers in the world = %d, want %d", step, got, want)
		}
	}
	count("boot", 0)
	drainFrames(t, w.c)

	if !w.srv.NpcSpawns.MakerEvent("hall_wizards", "1001", 0, 0) {
		t.Fatal("npcmaker hall_wizards not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	count("1001", 2)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(hallKeeperID, 2)) {
		t.Fatalf("1001: NPCs shown = %v", got)
	}

	keepers := mw.inWorld(hallKeeperID)
	keepers[0].DeleteMe()
	w.srv.Settle(t)
	count("a keeper left", 1)
	armed := 0
	for i := range 2 {
		if fireRespawn(w.srv, fmt.Sprintf("hall_wizards#0#%d", i)) {
			armed++
		}
	}
	if armed != 1 {
		t.Fatalf("a keeper left: respawns armed = %d, want 1", armed)
	}
	w.srv.Settle(t)
	count("respawn", 2)
	drainFrames(t, w.c)

	w.srv.NpcSpawns.MakerEvent("hall_wizards", "1000", 0, 0)
	w.srv.Settle(t)
	count("1000", 0)
	if got := deleted(drainFrames(t, w.c)); len(got) != 2 {
		t.Fatalf("1000: deleted objects = %v, want two", got)
	}
}

const (
	rainbowMessengerID = 21695
	rainbowCanID       = 21696
	rainbowJarID       = 21697
	rainbowSpawnlist   = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rainbow_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rainbow_hall" territory="rainbow_field" maximumNpcs="5">
		<ai type="rainbow_maker"><set name="CastleID" val="62"/></ai>
		<npc id="21695" total="1" pos="40;20;30;0" respawn="60sec"/>
		<npc id="21696" total="2" pos="50;20;30;0" respawn="60sec"/>
		<npc id="21697" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// RainbowMaker: at start only the first spawn places its NPC. The default
// maker's 1001 and 1000 do nothing; onSiegeEnd places one NPC of each
// other spawn, whatever its total. An NPC leaving arms no respawn.
func TestRainbowMakerSpawnsTheRestOnSiegeEnd(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(rainbowMessengerID, rainbowCanID, rainbowJarID)),
		gameservertest.WithNpcSpawns(batch1Table(t, rainbowSpawnlist)),
		gameservertest.WithMakers(batch2Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, messengers, cans, jars int) {
		t.Helper()
		got := [3]int{len(mw.inWorld(rainbowMessengerID)), len(mw.inWorld(rainbowCanID)), len(mw.inWorld(rainbowJarID))}
		if want := [3]int{messengers, cans, jars}; got != want {
			t.Fatalf("%s: NPCs in the world (messenger, can, jar) = %v, want %v", step, got, want)
		}
	}
	count("boot", 1, 0, 0)
	drainFrames(t, w.c)

	w.srv.NpcSpawns.MakerEvent("rainbow_hall", "1000", 0, 0)
	w.srv.NpcSpawns.MakerEvent("rainbow_hall", "1001", 0, 0)
	w.srv.Advance(t, time.Millisecond)
	count("1000 and 1001", 1, 0, 0)
	if frames := drainFrames(t, w.c); len(shown(frames)) != 0 || len(deleted(frames)) != 0 {
		t.Fatalf("1000 and 1001: shown %v and deleted %v, want none", shown(frames), deleted(frames))
	}

	if !w.srv.NpcSpawns.MakerEvent("rainbow_hall", "onSiegeEnd", 0, 0) {
		t.Fatal("npcmaker rainbow_hall not found by name")
	}
	w.srv.Settle(t)
	count("onSiegeEnd", 1, 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{rainbowCanID, rainbowJarID}) {
		t.Fatalf("onSiegeEnd: NPCs shown = %v, want [%d %d]", got, rainbowCanID, rainbowJarID)
	}

	messenger := mw.inWorld(rainbowMessengerID)[0]
	messenger.DeleteMe()
	w.srv.Settle(t)
	count("the messenger left", 0, 1, 1)
	if w.srv.NpcRespawns.Tracked("rainbow_hall#0#0") {
		t.Fatal("the messenger leaving armed a respawn")
	}
}
