package npcs

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The third set of batch 1 makers (#212): the unique-NPC makers, which
// read the template alias of their NPCs, and the random makers, which draw
// from the scripts' random source, booted through the batch's catalog
// file.

// aliasTemplates is batch1Templates with the template aliases alias gives.
func aliasTemplates(alias map[int]string, ids ...int) *npc.Table {
	var templates []*npc.Template
	for _, id := range ids {
		tmpl := spawnMonster(id, fmt.Sprintf("Batch %d", id))
		tmpl.Alias = alias[id]
		templates = append(templates, tmpl)
	}
	return npc.NewTable(templates)
}

// scriptDraws pins the scripts' random source: each draw returns the next
// queued answer and records the bound it was asked for.
type scriptDraws struct {
	mu      sync.Mutex
	answers []int
	asked   []int
}

func (d *scriptDraws) queue(answers ...int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.answers = append(d.answers, answers...)
}

func (d *scriptDraws) draw(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.asked = append(d.asked, n)
	if len(d.answers) == 0 {
		panic(fmt.Sprintf("unexpected script draw Rnd(%d)", n))
	}
	v := d.answers[0]
	d.answers = d.answers[1:]
	return v
}

func (d *scriptDraws) takeAsked() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.asked
	d.asked = nil
	return out
}

const (
	exclLeaderID  = 21960
	exclMinionID  = 21961
	exclOtherID   = 21962
	exclSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="excl_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="excl_a_other" territory="excl_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21962" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="excl_m" territory="excl_field" maximumNpcs="3">
		<ai type="exclusive_spawn_normal">
			<set name="unique_npc" val="excl_leader"/>
			<set name="maker_name" val="EXCL_A_OTHER"/>
		</ai>
		<npc id="21960" total="1" pos="40;20;30;0" respawn="60sec"/>
		<npc id="21961" total="2" pos="60;20;30;0" respawn="30sec"/>
	</npcmaker>
</list>`
)

// ExclusiveSpawnNormalMaker: the unique NPC (alias Excl_Leader, matched
// ignoring case) entering at start runs 1000 on the npcmaker maker_name
// names, deleting its NPC, and every other NPC of the group is deleted as
// it enters, arming no respawn. The unique NPC leaving runs 1001 there,
// which brings that NPC back, and arms its own respawn. While it is out an
// NPC of the group stays in and, leaving, respawns after its delay; the
// unique NPC back deletes the other maker's NPC again and an NPC of the
// group entering. A script event sets the flag to int1 before the default
// handling: after 1001 with int1 0, which brings nothing, an NPC of the
// group entering with the unique NPC in stays; 1000 with int1 0 then
// deletes the whole group, wakes nobody and arms no respawn.
func TestExclusiveSpawnNormalMakerKeepsItsGroupOut(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(aliasTemplates(map[int]string{exclLeaderID: "Excl_Leader"}, exclLeaderID, exclMinionID, exclOtherID)),
		gameservertest.WithNpcSpawns(batch1Table(t, exclSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, wantLeader, wantMinions, wantOther int) {
		t.Helper()
		for _, c := range []struct {
			what     string
			id, want int
		}{{"leaders", exclLeaderID, wantLeader}, {"minions", exclMinionID, wantMinions}, {"other maker's NPCs", exclOtherID, wantOther}} {
			if got := len(mw.inWorld(c.id)); got != c.want {
				t.Fatalf("%s: %s = %d, want %d", step, c.what, got, c.want)
			}
		}
	}
	count("boot", 1, 0, 0)
	for _, slot := range []string{"excl_m#1#0", "excl_m#1#1", "excl_a_other#0#0"} {
		if w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("boot: slot %s armed a respawn", slot)
		}
	}
	drainFrames(t, w.c)

	leader := mw.inWorld(exclLeaderID)[0]
	leader.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	count("the leader left", 0, 0, 1)
	frames := drainFrames(t, w.c)
	if got := deleted(frames); !slices.Equal(got, []int32{leader.ObjectID()}) {
		t.Fatalf("the leader left: deleted objects = %v, want [%d]", got, leader.ObjectID())
	}
	if got := shown(frames); !slices.Equal(got, []int{exclOtherID}) {
		t.Fatalf("the leader left: NPCs shown = %v, want [%d]", got, exclOtherID)
	}
	if !w.srv.NpcRespawns.Tracked("excl_m#0#0") {
		t.Fatal("the leader left and armed no respawn of its own")
	}

	w.srv.NpcSpawns.Respawn("excl_m#1#0")
	w.srv.Settle(t)
	count("a minion while the leader is out", 0, 1, 1)
	drainFrames(t, w.c)
	mw.inWorld(exclMinionID)[0].DeleteMe()
	w.srv.Settle(t)
	if !w.srv.NpcRespawns.Tracked("excl_m#1#0") {
		t.Fatal("a minion left while the leader is out and armed no respawn")
	}
	drainFrames(t, w.c)

	other := mw.inWorld(exclOtherID)[0]
	if !fireRespawn(w.srv, "excl_m#0#0") {
		t.Fatal("the leader's respawn is gone")
	}
	w.srv.Settle(t)
	count("the leader back", 1, 0, 0)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{other.ObjectID()}) {
		t.Fatalf("the leader back: deleted objects = %v, want [%d]", got, other.ObjectID())
	}
	if !fireRespawn(w.srv, "excl_m#1#0") {
		t.Fatal("the minion's respawn is gone")
	}
	w.srv.Settle(t)
	count("a minion while the leader is in", 1, 0, 0)
	frames = drainFrames(t, w.c)
	if got, gone := shown(frames), deleted(frames); !slices.Equal(got, []int{exclMinionID}) || len(gone) != 1 {
		t.Fatalf("a minion while the leader is in: shown %v and deleted %v, want the minion shown and deleted", got, gone)
	}
	if w.srv.NpcRespawns.Tracked("excl_m#1#0") {
		t.Fatal("a minion deleted while the leader is in armed a respawn")
	}

	// 1001 with int1 0 lowers the flag with the leader in; the default
	// 1001 then brings nothing, its NPCs' respawns being immediate.
	if !w.srv.NpcSpawns.MakerEvent("excl_m", "1001", 0, 0) {
		t.Fatal("npcmaker excl_m not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	count("1001 with int1 0", 1, 0, 0)
	w.srv.NpcSpawns.Respawn("excl_m#1#1")
	w.srv.Settle(t)
	count("a minion after 1001 with int1 0", 1, 1, 0)
	drainFrames(t, w.c)

	group := objectIDs(append(mw.inWorld(exclLeaderID), mw.inWorld(exclMinionID)...))
	if !w.srv.NpcSpawns.MakerEvent("excl_m", "1000", 0, 0) {
		t.Fatal("npcmaker excl_m not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	count("1000 with int1 0", 0, 0, 0)
	frames = drainFrames(t, w.c)
	if got := deleted(frames); !slices.Equal(got, group) {
		t.Fatalf("1000 with int1 0: deleted objects = %v, want the group %v", got, group)
	}
	if got := shown(frames); len(got) != 0 {
		t.Fatalf("1000 with int1 0: NPCs shown = %v, want none", got)
	}
	for _, slot := range []string{"excl_m#0#0", "excl_m#1#0", "excl_m#1#1"} {
		if w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("1000 with int1 0: slot %s armed a respawn", slot)
		}
	}
}

const (
	despBrazierID = 21965
	despMobID     = 21966
	despGuardID   = 21967
	despSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="desp_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="desp1_mob" territory="desp_field" maximumNpcs="2">
		<ai type="exclusive_despawn_normal">
			<set name="maker_name2" val="desp1_mob"/>
			<set name="despawn_count" val="2"/>
		</ai>
		<npc id="21966" total="2" pos="60;20;30;0" respawn="50sec"/>
	</npcmaker>
	<npcmaker name="desp2_guard" territory="desp_field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21967" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="desp3_brazier" territory="desp_field" maximumNpcs="1">
		<ai type="exclusive_despawn_normal">
			<set name="unique_npc" val="Holy_Brazier"/>
			<set name="maker_name1" val="desp2_guard"/>
			<set name="maker_name2" val="DESP1_MOB"/>
		</ai>
		<npc id="21965" total="1" pos="40;20;30;0" respawn="5min"/>
	</npcmaker>
</list>`
)

// ExclusiveDespawnNormalMaker, chained as the shipped holy braziers are:
// the mob maker, with no unique NPC, deletes its NPCs as they enter at
// start. The brazier (alias holy_brazier) entering runs 1001 with int1 1
// on the guard maker, whose guard comes in a second later, and on the mob
// maker, which only raises its flag: a mob entering then stays and,
// leaving, respawns after its delay. The brazier leaving runs 1000 on the
// guard maker and 10025 on the mob maker, and arms its own respawn. The
// mob maker's second 10025 (its despawn_count) runs 1000 on itself, which
// only lowers its flag again: its mob stays and, leaving, arms no respawn.
// A 1001 to the mob maker raises the flag and spawns nothing.
func TestExclusiveDespawnNormalMakerFollowsItsBrazier(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(aliasTemplates(map[int]string{despBrazierID: "holy_brazier"}, despBrazierID, despMobID, despGuardID)),
		gameservertest.WithNpcSpawns(batch1Table(t, despSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, wantBrazier, wantMobs, wantGuard int) {
		t.Helper()
		for _, c := range []struct {
			what     string
			id, want int
		}{{"braziers", despBrazierID, wantBrazier}, {"mobs", despMobID, wantMobs}, {"guards", despGuardID, wantGuard}} {
			if got := len(mw.inWorld(c.id)); got != c.want {
				t.Fatalf("%s: %s = %d, want %d", step, c.what, got, c.want)
			}
		}
	}
	count("boot", 1, 0, 0)
	for _, slot := range []string{"desp1_mob#0#0", "desp1_mob#0#1"} {
		if w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("boot: slot %s armed a respawn", slot)
		}
	}
	w.srv.Advance(t, time.Second)
	count("a second after boot", 1, 0, 1)
	drainFrames(t, w.c)

	killBrazier := func(step string, wantMobs int) {
		t.Helper()
		brazier := mw.inWorld(despBrazierID)[0]
		guard := mw.inWorld(despGuardID)[0]
		brazier.DeleteMe()
		w.srv.Settle(t)
		count(step, 0, wantMobs, 0)
		want := objectIDs([]*npc.Hostile{brazier, guard})
		if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, want) {
			t.Fatalf("%s: deleted objects = %v, want %v", step, got, want)
		}
		if !w.srv.NpcRespawns.Tracked("desp3_brazier#0#0") {
			t.Fatalf("%s: the brazier armed no respawn", step)
		}
	}
	killBrazier("the brazier left", 0)

	w.srv.NpcSpawns.Respawn("desp1_mob#0#0")
	w.srv.Settle(t)
	count("a mob after the brazier came in", 0, 1, 0)
	mw.inWorld(despMobID)[0].DeleteMe()
	w.srv.Settle(t)
	if !fireRespawn(w.srv, "desp1_mob#0#0") {
		t.Fatal("a mob left while the flag is up and armed no respawn")
	}
	w.srv.Settle(t)
	count("the mob respawned", 0, 1, 0)
	drainFrames(t, w.c)

	if !fireRespawn(w.srv, "desp3_brazier#0#0") {
		t.Fatal("the brazier's respawn is gone")
	}
	w.srv.Advance(t, time.Second)
	count("the brazier back", 1, 1, 1)
	drainFrames(t, w.c)

	killBrazier("the brazier left the despawn_count-th time", 1)
	mw.inWorld(despMobID)[0].DeleteMe()
	w.srv.Settle(t)
	count("a mob left with the flag down", 0, 0, 0)
	if w.srv.NpcRespawns.Tracked("desp1_mob#0#0") {
		t.Fatal("a mob left with the flag down and armed a respawn")
	}
	drainFrames(t, w.c)

	if !w.srv.NpcSpawns.MakerEvent("desp1_mob", "1001", 1, 0) {
		t.Fatal("npcmaker desp1_mob not found by name")
	}
	w.srv.Advance(t, time.Minute)
	count("1001 to the mob maker", 0, 0, 0)
	if got := shown(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("1001 to the mob maker: NPCs shown = %v, want none", got)
	}
	w.srv.NpcSpawns.Respawn("desp1_mob#0#1")
	w.srv.Settle(t)
	count("a mob after 1001 with int1 1", 0, 1, 0)
}

const (
	isrSourceID  = 21970
	isrOneID     = 21971
	isrTwoID     = 21972
	isrThreeID   = 21973
	isrSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="isr_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="isr_src" territory="isr_field" maximumNpcs="1">
		<ai type="maker_instant_spawn_random">
			<set name="maker_cnt" val="3"/>
			<set name="maker_name1" val="isr_one"/>
			<set name="maker_name2" val="ISR_TWO"/>
			<set name="maker_name3" val="isr_three"/>
		</ai>
		<npc id="21970" total="1" pos="40;20;30;0" respawn="20sec"/>
	</npcmaker>
	<npcmaker name="isr_one" territory="isr_field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21971" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="isr_two" territory="isr_field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21972" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="isr_three" territory="isr_field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21973" total="1" pos="100;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// InstantSpawnRandomMaker: its group's last NPC leaving draws Rnd(maker_cnt)
// from the scripts' random source and wakes the npcmaker of that draw
// (draw 1 maker_name2, draw 2 maker_name3) with 1001 and the NPC's 20
// second respawn delay; its own NPC arms no respawn.
func TestInstantSpawnRandomMakerWakesADrawnMaker(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(isrSourceID, isrOneID, isrTwoID, isrThreeID)),
		gameservertest.WithNpcSpawns(batch1Table(t, isrSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
		gameservertest.WithScriptRand(draws.draw),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want ...int) {
		t.Helper()
		for i, id := range []int{isrSourceID, isrOneID, isrTwoID, isrThreeID} {
			if got := len(mw.inWorld(id)); got != want[i] {
				t.Fatalf("%s: NPCs of %d = %d, want %d", step, id, got, want[i])
			}
		}
	}
	count("boot", 1, 0, 0, 0)
	if got := draws.takeAsked(); len(got) != 0 {
		t.Fatalf("boot drew %v, want nothing", got)
	}
	drainFrames(t, w.c)

	wake := func(step string, draw, id int, want []int) {
		t.Helper()
		draws.queue(draw)
		mw.inWorld(isrSourceID)[0].DeleteMe()
		w.srv.Settle(t)
		if got := draws.takeAsked(); !slices.Equal(got, []int{3}) {
			t.Fatalf("%s: draws = %v, want one Rnd(3)", step, got)
		}
		if w.srv.NpcRespawns.Tracked("isr_src#0#0") {
			t.Fatalf("%s: the source NPC armed a respawn", step)
		}
		w.srv.Advance(t, 19*time.Second)
		if got := shown(drainFrames(t, w.c)); len(got) != 0 {
			t.Fatalf("%s: NPCs shown before the delay = %v, want none", step, got)
		}
		w.srv.Advance(t, time.Second)
		count(step, want...)
		if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{id}) {
			t.Fatalf("%s: NPCs shown = %v, want [%d]", step, got, id)
		}
	}
	wake("draw 1", 1, isrTwoID, []int{0, 0, 1, 0})

	if !w.srv.NpcSpawns.MakerEvent("isr_src", "1001", 1, 0) {
		t.Fatal("npcmaker isr_src not found by name")
	}
	w.srv.Settle(t)
	if !fireRespawn(w.srv, "isr_src#0#0") {
		t.Fatal("1001 with int1 1 armed no respawn of the source NPC")
	}
	w.srv.Settle(t)
	count("the source back", 1, 0, 1, 0)
	drainFrames(t, w.c)
	wake("draw 2", 2, isrThreeID, []int{0, 0, 1, 1})
}

const (
	rsFirstID   = 21975
	rsSecondID  = 21976
	rsOffID     = 21977
	rspFirstID  = 21978
	rspSecondID = 21979
	rspOffID    = 21980
	rsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rs_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rs_m" territory="rs_field" maximumNpcs="2">
		<ai type="random_spawn"/>
		<npc id="21975" total="2" pos="40;20;30;0" respawn="30sec"/>
		<npc id="21976" total="1" pos="60;20;30;0" respawn="40sec"/>
	</npcmaker>
	<npcmaker name="rs_off" territory="rs_field" maximumNpcs="1">
		<ai type="random_spawn"><set name="on_start_spawn" val="0"/></ai>
		<npc id="21977" total="1" pos="80;20;30;0" respawn="30sec"/>
	</npcmaker>
	<npcmaker name="rsp_m" territory="rs_field" maximumNpcs="2">
		<ai type="random_spawn_pawn"/>
		<npc id="21978" total="2" pos="100;20;30;0" respawn="30sec"/>
		<npc id="21979" total="1" pos="120;20;30;0" respawn="40sec"/>
	</npcmaker>
	<npcmaker name="rsp_off" territory="rs_field" maximumNpcs="1">
		<ai type="random_spawn_pawn"><set name="on_start_spawn" val="0"/></ai>
		<npc id="21980" total="1" pos="140;20;30;0" respawn="30sec"/>
	</npcmaker>
</list>`
)

// RandomSpawnMaker and RandomSpawnPawnMaker. At start the random maker
// draws one of its spawns, which places its whole total; the pawn maker
// fills its spawns in order up to its maximum; an npcmaker not starting
// at boot does neither. Each NPC leaving draws a spawn again: one short of
// its total, while the group is under its maximum, brings an NPC in after
// that spawn's delay (an NPC out of the world back first); a drawn spawn
// already full brings nothing. Neither maker arms an NPC's own respawn.
func TestRandomSpawnMakersDrawTheirSpawn(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	draws.queue(0)
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(rsFirstID, rsSecondID, rsOffID, rspFirstID, rspSecondID, rspOffID)),
		gameservertest.WithNpcSpawns(batch1Table(t, rsSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
		gameservertest.WithScriptRand(draws.draw),
	)
	mw := &makerWorld{srv: w.srv}
	ids := []int{rsFirstID, rsSecondID, rsOffID, rspFirstID, rspSecondID, rspOffID}
	count := func(step string, want ...int) {
		t.Helper()
		for i, id := range ids {
			if got := len(mw.inWorld(id)); got != want[i] {
				t.Fatalf("%s: NPCs of %d = %d, want %d", step, id, got, want[i])
			}
		}
	}
	count("boot", 2, 0, 0, 2, 0, 0)
	if got := draws.takeAsked(); !slices.Equal(got, []int{2}) {
		t.Fatalf("boot draws = %v, want the random maker's one Rnd(2)", got)
	}
	drainFrames(t, w.c)

	// leave deletes the first NPC of id in the world with the next draw
	// answered draw, and checks no own respawn is armed.
	leave := func(step string, id, draw int) {
		t.Helper()
		draws.queue(draw)
		mw.inWorld(id)[0].DeleteMe()
		w.srv.Settle(t)
		if got := draws.takeAsked(); !slices.Equal(got, []int{2}) {
			t.Fatalf("%s: draws = %v, want one Rnd(2)", step, got)
		}
		for _, slot := range []string{"rs_m#0#0", "rs_m#0#1", "rs_m#1#0", "rsp_m#0#0", "rsp_m#0#1", "rsp_m#1#0"} {
			if w.srv.NpcRespawns.Tracked(slot) {
				t.Fatalf("%s: slot %s armed a respawn", step, slot)
			}
		}
	}
	// arrive expects id shown once delay has passed and not a second
	// before.
	arrive := func(step string, id int, delay time.Duration, want ...int) {
		t.Helper()
		w.srv.Advance(t, delay-time.Second)
		if got := shown(drainFrames(t, w.c)); len(got) != 0 {
			t.Fatalf("%s: NPCs shown before the delay = %v", step, got)
		}
		w.srv.Advance(t, time.Second)
		count(step, want...)
		if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{id}) {
			t.Fatalf("%s: NPCs shown = %v, want [%d]", step, got, id)
		}
	}

	leave("random: a first NPC left, second spawn drawn", rsFirstID, 1)
	arrive("random: second spawn filled", rsSecondID, 40*time.Second, 1, 1, 0, 2, 0, 0)

	leave("random: a first NPC left, full second spawn drawn", rsFirstID, 1)
	w.srv.Advance(t, time.Minute)
	count("random: nothing drawn short", 0, 1, 0, 2, 0, 0)
	if got := shown(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("random: full spawn drawn: NPCs shown = %v, want none", got)
	}

	leave("random: the second NPC left, first spawn drawn", rsSecondID, 0)
	arrive("random: a first NPC back", rsFirstID, 30*time.Second, 1, 0, 0, 2, 0, 0)

	leave("pawn: a first NPC left, second spawn drawn", rspFirstID, 1)
	arrive("pawn: second spawn filled", rspSecondID, 40*time.Second, 1, 0, 0, 1, 1, 0)
}

const (
	ukBossID     = 21985
	ukMobID      = 21986
	ukNextID     = 21987
	ukWakeBossID = 21988
	ukWakeMobID  = 21989
	ukBossAlias  = "Chapel_Boss"
	ukSpawnlist  = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="uk_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="uk_next" territory="uk_field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21987" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="uk_delete" territory="uk_field" maximumNpcs="3">
		<ai type="unique_npc_kill_event">
			<set name="unique_npc" val="chapel_boss"/>
			<set name="event" val="0"/>
		</ai>
		<npc id="21985" total="1" pos="60;20;30;0" respawn="60sec"/>
		<npc id="21986" total="2" pos="80;20;30;0" respawn="30sec"/>
	</npcmaker>
	<npcmaker name="uk_wake" territory="uk_field" maximumNpcs="2">
		<ai type="unique_npc_kill_event">
			<set name="unique_npc" val="CHAPEL_BOSS"/>
			<set name="event" val="1"/>
			<set name="maker_name" val="uk_next"/>
		</ai>
		<npc id="21988" total="1" pos="100;20;30;0" respawn="60sec"/>
		<npc id="21989" total="1" pos="120;20;30;0" respawn="30sec"/>
	</npcmaker>
</list>`
)

// UniqueNpcKillEventMaker: an NPC other than the unique one (alias
// Chapel_Boss, matched ignoring case) leaving respawns as the default
// maker's does. With event 0 the unique NPC leaving runs the maker's own
// 1000, deleting the rest of its group and cancelling their respawns, and
// arms its own respawn, which brings it back. With event 1 it runs 1001 on the npcmaker
// maker_name names, which brings that maker's NPC in, and arms its own
// respawn; the rest of its group stays.
func TestUniqueNpcKillEventMakerActsOnItsUniqueNPC(t *testing.T) {
	t.Parallel()
	alias := map[int]string{ukBossID: ukBossAlias, ukWakeBossID: ukBossAlias}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(aliasTemplates(alias, ukBossID, ukMobID, ukNextID, ukWakeBossID, ukWakeMobID)),
		gameservertest.WithNpcSpawns(batch1Table(t, ukSpawnlist)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	ids := []int{ukBossID, ukMobID, ukNextID, ukWakeBossID, ukWakeMobID}
	count := func(step string, want ...int) {
		t.Helper()
		for i, id := range ids {
			if got := len(mw.inWorld(id)); got != want[i] {
				t.Fatalf("%s: NPCs of %d = %d, want %d", step, id, got, want[i])
			}
		}
	}
	count("boot", 1, 2, 0, 1, 1)
	drainFrames(t, w.c)

	mobs := mw.inWorld(ukMobID)
	mobs[0].DeleteMe()
	w.srv.Settle(t)
	armed := func() []string {
		var out []string
		for _, slot := range []string{"uk_delete#1#0", "uk_delete#1#1"} {
			if w.srv.NpcRespawns.Tracked(slot) {
				out = append(out, slot)
			}
		}
		return out
	}
	if got := armed(); len(got) != 1 {
		t.Fatalf("a mob left: respawns armed = %v, want one", got)
	}
	drainFrames(t, w.c)

	boss := mw.inWorld(ukBossID)[0]
	boss.DeleteMe()
	w.srv.Settle(t)
	count("event 0: the unique NPC left", 0, 0, 0, 1, 1)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, objectIDs([]*npc.Hostile{boss, mobs[1]})) {
		t.Fatalf("event 0: deleted objects = %v, want the boss and the other mob", got)
	}
	if got := armed(); len(got) != 0 {
		t.Fatalf("event 0: mob respawns still armed = %v", got)
	}
	if !w.srv.NpcRespawns.Tracked("uk_delete#0#0") {
		t.Fatal("event 0: the unique NPC armed no respawn after the 1000")
	}

	wakeBoss := mw.inWorld(ukWakeBossID)[0]
	wakeBoss.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	count("event 1: the unique NPC left", 0, 0, 1, 0, 1)
	frames := drainFrames(t, w.c)
	if got := shown(frames); !slices.Equal(got, []int{ukNextID}) {
		t.Fatalf("event 1: NPCs shown = %v, want [%d]", got, ukNextID)
	}
	if got := deleted(frames); !slices.Equal(got, []int32{wakeBoss.ObjectID()}) {
		t.Fatalf("event 1: deleted objects = %v, want [%d]", got, wakeBoss.ObjectID())
	}
	if !w.srv.NpcRespawns.Tracked("uk_wake#0#0") {
		t.Fatal("event 1: the unique NPC armed no respawn")
	}

	// Both unique NPCs come back when their respawns come due: the event
	// 0 one too, though the 1000 had deleted its group; the mobs it
	// deleted stay gone. The harness never fires a respawn timer.
	for _, slot := range []string{"uk_delete#0#0", "uk_wake#0#0"} {
		w.srv.NpcRespawns.Cancel(slot)
		w.srv.NpcSpawns.Respawn(slot)
	}
	w.srv.Settle(t)
	count("the unique NPCs respawned", 1, 0, 1, 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{ukBossID, ukWakeBossID}) {
		t.Fatalf("respawn: NPCs shown = %v, want [%d %d]", got, ukBossID, ukWakeBossID)
	}
}

const (
	psaPriestID  = 21990
	psaExtraID   = 21991
	psaDeadID    = 21992
	psaRowPriest = "psa_priest"
	psaRowExtra  = "psa_extra"
	psaRowDead   = "psa_dead"
	psaSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="psa_lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="psa_a" territory="psa_lair" maximumNpcs="1"><ai type="maker_event_log"/></npcmaker>
	<npcmaker name="psa_b" territory="psa_lair" maximumNpcs="1"><ai type="maker_event_log"/></npcmaker>
	<npcmaker name="parent_spawn_all_default" territory="psa_lair" maximumNpcs="1"><ai type="maker_event_log"/></npcmaker>
	<npcmaker name="psa_m" territory="psa_lair" maximumNpcs="1">
		<ai type="parent_spawn_all">
			<set name="unique_npc" val="triolls_priest"/>
			<set name="maker_nameA1" val="PSA_A"/>
			<set name="maker_nameA3" val="psa_missing"/>
			<set name="maker_nameB1" val="Parent_Spawn_All_Default"/>
			<set name="maker_nameB4" val="psa_b"/>
		</ai>
		<npc id="21990" total="1" pos="40;20;30;0" respawn="60sec" dbName="psa_priest"/>
		<npc id="21991" total="1" pos="60;20;30;0" respawn="60sec" dbName="psa_extra"/>
	</npcmaker>
	<npcmaker name="psa_dead_m" territory="psa_lair" maximumNpcs="1">
		<ai type="parent_spawn_all">
			<set name="unique_npc" val="triolls_priest"/>
			<set name="maker_nameA1" val="psa_a"/>
		</ai>
		<npc id="21992" total="1" pos="80;20;30;0" respawn="60sec" dbName="psa_dead"/>
	</npcmaker>
</list>`
)

// ParentSpawnAllMaker: at start each database-tracked spawn is loaded; the
// priest's live row places it, whose entering runs 1001 on the A and B
// makers (A1 and B4 here; a name no npcmaker has, and one naming
// parent_spawn_all_default ignoring case, run nothing), and the loaded
// spawn runs 1001 on them again. A second spawn that would take the group
// over its maximum, and a spawn whose row is saved dead, place nothing and
// run nothing, the dead row left as saved. The priest leaving runs 1000 on
// the A makers only and arms its respawn; back, its entering runs 1001 on
// the A and B makers once.
func TestParentSpawnAllMakerBringsItsChildMakers(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	at := location.Location{X: 100, Y: 100, Z: 30}
	rows := map[string]*spawn.State{
		psaRowPriest: {Name: psaRowPriest, Status: spawn.StatusAlive, CurrentHP: 100, CurrentMP: 10, Location: at},
		psaRowExtra:  {Name: psaRowExtra, Status: spawn.StatusAlive, CurrentHP: 100, CurrentMP: 10, Location: at},
		psaRowDead:   {Name: psaRowDead, Status: spawn.StatusDead, Location: at, RespawnTime: time.Now().Add(time.Hour).UnixMilli()},
	}
	if err := store.SaveStates(context.Background(), rows); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	events := &makerEventLog{}
	makers := batch1Makers()
	maps.Copy(makers, events.catalog())
	alias := map[int]string{psaPriestID: "Triolls_Priest", psaExtraID: "Triolls_Priest", psaDeadID: "Triolls_Priest"}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(aliasTemplates(alias, psaPriestID, psaExtraID, psaDeadID)),
		gameservertest.WithNpcSpawns(batch1Table(t, psaSpawnlist)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, wantPriest int) {
		t.Helper()
		if got := len(mw.inWorld(psaPriestID)); got != wantPriest {
			t.Fatalf("%s: priests = %d, want %d", step, got, wantPriest)
		}
		if got := len(mw.inWorld(psaExtraID)) + len(mw.inWorld(psaDeadID)); got != 0 {
			t.Fatalf("%s: other database-tracked NPCs = %d, want 0", step, got)
		}
	}
	both := []string{"psa_a:1001(0,0)", "psa_b:1001(0,0)"}
	count("boot", 1)
	events.expect(t, "boot", append(slices.Clone(both), both...)...)
	if row := rowState(t, w.srv, psaRowDead); row.Status != spawn.StatusDead {
		t.Fatalf("dead row status after boot = %v, want dead", row.Status)
	}
	drainFrames(t, w.c)

	priest := mw.inWorld(psaPriestID)[0]
	priest.DeleteMe()
	w.srv.Settle(t)
	count("the priest left", 0)
	events.expect(t, "the priest left", "psa_a:1000(0,0)")
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{priest.ObjectID()}) {
		t.Fatalf("the priest left: deleted objects = %v, want [%d]", got, priest.ObjectID())
	}
	if !fireRespawn(w.srv, psaRowPriest) {
		t.Fatal("the priest left and armed no respawn")
	}
	w.srv.Settle(t)
	count("the priest back", 1)
	events.expect(t, "the priest came back", both...)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{psaPriestID}) {
		t.Fatalf("the priest back: NPCs shown = %v, want [%d]", got, psaPriestID)
	}
}
