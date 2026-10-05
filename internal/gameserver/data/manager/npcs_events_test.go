package manager

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// eventsSpawnlist declares one maker per spawn condition, each spawning the
// npc id its comment names.
const eventsSpawnlist = `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<!-- 1: a plain on-start maker -->
	<npcmaker name="plain" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;10;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 2: an 18age event maker -->
	<npcmaker name="sellers" territory="field" event="18age" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="2" total="1" pos="20;20;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 3: a christmas event maker -->
	<npcmaker name="tree" territory="field" event="christmas" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="3" total="1" pos="30;30;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 4: an event_maker for 18age -->
	<npcmaker name="orc_seller" territory="field" maximumNpcs="1">
		<ai type="event_maker"><set name="EventName" val="18age"/></ai>
		<npc id="4" total="1" pos="40;40;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 5: an event_maker for event_mutant_pig -->
	<npcmaker name="pig" territory="field" maximumNpcs="1">
		<ai type="event_maker"><set name="EventName" val="event_mutant_pig"/></ai>
		<npc id="5" total="1" pos="50;50;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 6: a Seven Signs event maker -->
	<npcmaker name="ssq" territory="field" event="ssq_event" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="6" total="1" pos="60;60;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 7: an event_maker for extra_mob -->
	<npcmaker name="extra" territory="field" maximumNpcs="1">
		<ai type="event_maker"><set name="EventName" val="extra_mob"/></ai>
		<npc id="7" total="1" pos="70;70;0;0" respawn="60sec"/>
	</npcmaker>
</list>`

// eventsFixture is a population booted over a spawn list with the given
// SpawnEvents list.
type eventsFixture struct {
	npcs    *Npcs
	state   *world.State
	respawn *task.Respawn
	queues  *sim.Inline
}

func newEventsFixture(t *testing.T, table *spawn.Table, templates *npc.Table, events []string) *eventsFixture {
	t.Helper()
	f := &eventsFixture{state: world.New(), queues: npcQueues()}
	decay, _ := task.NewDecay(nopDecayEffects{}, time.Now)
	f.respawn, _ = task.NewRespawn(nopRespawnEffects{}, time.Now)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, time.Now, f.state)
	var err error
	f.npcs, err = newNpcs(NewSpawns(table, nil), templates, fakeGeo{}, f.state, &sequentialIDs{}, decay, f.respawn,
		task.NewAI(f.state, zerolog.Nop()), task.NewPositionUpdates(f.state), item.NewTable(nil), &recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(),
		nil, actorcast.EffectHandlers{}, walker, nil, nil, 20, 30, 0, npc.DefaultRaidMultipliers(), npc.DefaultAIConfig(), events,
		effect.Env{Activity: task.NewEffects()}, f.queues)
	if err != nil {
		t.Fatalf("newNpcs() error: %v", err)
	}
	return f
}

func eventsTemplates() *npc.Table {
	var templates []*npc.Template
	for id := 1; id <= 7; id++ {
		templates = append(templates, &npc.Template{ID: id, TemplateID: id, Type: "Monster", HPMax: 100, RunSpeed: 100, AIParams: commons.NewStatSet()})
	}
	return npc.NewTable(templates)
}

// spawnedIDs returns the sorted template ids of the NPCs in the world.
func (f *eventsFixture) spawnedIDs() []int {
	var out []int
	for _, obj := range f.state.Objects() {
		switch o := obj.(type) {
		case *npc.Hostile:
			out = append(out, o.Instance.Template.ID)
		case *npc.Folk:
			out = append(out, o.Instance.Template.ID)
		}
	}
	slices.Sort(out)
	return out
}

func (f *eventsFixture) hostile(t *testing.T, npcID int) *npc.Hostile {
	t.Helper()
	for _, obj := range f.state.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == npcID {
			return h
		}
	}
	t.Fatalf("no npc %d in the world", npcID)
	return nil
}

// TestSpawnEventsGateBootSpawn pins SpawnManager.spawn: the on-start makers
// spawn, an event_maker only when its EventName is listed
// (EventMaker.shouldSpawn), then every maker of each listed event
// (spawnEventNpcs, DefaultMaker.onStart). A Seven Signs event never spawns
// through the list, and an event listed twice spawns its makers once.
func TestSpawnEventsGateBootSpawn(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "events.xml"), eventsSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	for _, tc := range []struct {
		name   string
		events []string
		want   []int
	}{
		{"shipped default", DefaultSpawnEvents(), []int{1, 2, 4, 7}},
		{"christmas only", []string{"christmas"}, []int{1, 3}},
		{"18age removed", []string{"extra_mob", "start_weapon"}, []int{1, 7}},
		{"seven signs and a repeat", []string{"ssq_event", "18age", "18age"}, []int{1, 2, 4}},
		{"none", nil, []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventsFixture(t, table, eventsTemplates(), tc.events)
			if got := f.spawnedIDs(); !slices.Equal(got, tc.want) {
				t.Fatalf("spawned npc ids = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSpawnEventsGateRespawn pins DefaultMaker.onNpcDeleted and
// EventMaker.onNpcDeleted: an event NPC removed from the world arms its
// respawn while its event is listed, and stays gone once it is not; a plain
// maker's NPC respawns either way. //respawnall spawns the listed event
// makers again.
func TestSpawnEventsGateRespawn(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "events.xml"), eventsSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	f := newEventsFixture(t, table, eventsTemplates(), DefaultSpawnEvents())

	f.hostile(t, 2).DeleteMe()
	f.queues.Run()
	if !f.respawn.Tracked("sellers#0#0") {
		t.Fatal("listed event maker NPC: respawn not armed")
	}

	// The list no longer names 18age or extra_mob.
	f.npcs.events = newSpawnEvents([]string{"start_weapon"})
	for _, tc := range []struct {
		npcID int
		key   string
		armed bool
	}{
		{4, "orc_seller#0#0", false},
		{7, "extra#0#0", false},
		{1, "plain#0#0", true},
	} {
		f.hostile(t, tc.npcID).DeleteMe()
		f.queues.Run()
		if got := f.respawn.Tracked(tc.key); got != tc.armed {
			t.Fatalf("npc %d respawn armed = %v, want %v", tc.npcID, got, tc.armed)
		}
	}

	f.npcs.events = newSpawnEvents(DefaultSpawnEvents())
	f.npcs.DespawnAll()
	f.npcs.RespawnAll(NewSpawns(table, nil))
	if got, want := f.spawnedIDs(), []int{1, 2, 4, 7}; !slices.Equal(got, want) {
		t.Fatalf("after RespawnAll spawned npc ids = %v, want %v", got, want)
	}
}

// lotterySellerIDs are the five Lottery Ticket Seller templates
// (data/xml/npcs/30000-30999.xml 30990-30994, type Folk).
var lotterySellerIDs = []int32{30990, 30991, 30992, 30993, 30994}

// TestShippedSpawnEventsSpawnEveryLotterySeller boots the shipped
// spawnlist's lottery seller makers with the shipped SpawnEvents: all 18
// sellers stand in the world (17 18age makers plus Schuttgart's 18age
// event_maker), and none without 18age listed.
func TestShippedSpawnEventsSpawnEveryLotterySeller(t *testing.T) {
	dp := datapack.Require(t)
	full, err := xml.LoadSpawnlist(filepath.Join(dp, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	var makers []*spawn.Maker
	var territories []*spawn.Territory
	for _, maker := range full.Makers() {
		if !slices.ContainsFunc(maker.Entries, func(e spawn.Entry) bool { return slices.Contains(lotterySellerIDs, e.NPCID) }) {
			continue
		}
		makers = append(makers, maker)
		territories = append(territories, maker.Territories...)
		territories = append(territories, maker.BannedTerritories...)
	}
	table, err := spawn.NewTable(territories, makers)
	if err != nil {
		t.Fatalf("spawn.NewTable() error: %v", err)
	}
	var templates []*npc.Template
	for _, id := range lotterySellerIDs {
		templates = append(templates, &npc.Template{ID: int(id), TemplateID: int(id), Type: "Folk", Name: "Lottery Ticket Seller", Level: 70, HPMax: 2444, AIParams: commons.NewStatSet()})
	}

	shipped := newEventsFixture(t, table, npc.NewTable(templates), DefaultSpawnEvents())
	if got := len(shipped.spawnedIDs()); got != 18 {
		t.Fatalf("lottery sellers in the world = %d, want 18", got)
	}
	var keys []string
	for _, maker := range makers {
		keys = append(keys, maker.Name)
	}
	if !slices.ContainsFunc(keys, func(k string) bool { return strings.EqualFold(k, "gludio06_npc1722_tk01") }) {
		t.Fatalf("gludio06_npc1722_tk01 not among the seller makers %v", keys)
	}

	without := newEventsFixture(t, table, npc.NewTable(templates), []string{"extra_mob", "start_weapon"})
	if got := without.spawnedIDs(); len(got) != 0 {
		t.Fatalf("lottery sellers in the world without 18age = %v, want none", got)
	}
}
