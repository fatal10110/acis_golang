package manager

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/rs/zerolog"
)

// spawnTimeSpawnlist declares one maker per spawn time, each spawning the
// npc id its comment names.
const spawnTimeSpawnlist = `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<!-- 1: castle 1's guards while it has no owner -->
	<npcmaker name="gludio_guards" territory="field" spawnTime="siege_warfare_start(1)" maximumNpcs="2">
		<ai type="default_maker"/>
		<npc id="1" total="2" pos="10;10;0;0" respawn="3hour"/>
	</npcmaker>
	<!-- 2: castle 1's owner's guards -->
	<npcmaker name="gludio_owner_guards" territory="field" spawnTime="PC_SIEGE_WARFARE_START(1)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="2" total="1" pos="20;20;0;0" respawn="3hour"/>
	</npcmaker>
	<!-- 3: castle 2's guards while it has no owner -->
	<npcmaker name="dion_guards" territory="field" spawnTime="siege_warfare_start(2)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="3" total="1" pos="30;30;0;0" respawn="3hour"/>
	</npcmaker>
	<!-- 4: a battle royal maker with three parameters -->
	<npcmaker name="royal" territory="field" spawnTime="agit_battle_royal_start(63;blue;1)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="4" total="1" pos="40;40;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 5: Core's door makers -->
	<npcmaker name="kuruma" territory="field" spawnTime="door_open([kuruma_parent])" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="5" total="1" pos="50;50;0;0" respawn="60sec"/>
	</npcmaker>
	<!-- 6: a hall's defenders -->
	<npcmaker name="hall_defenders" territory="field" spawnTime="agit_defend_warfare_start(34)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="6" total="1" pos="60;60;0;0" respawn="60sec"/>
	</npcmaker>
</list>`

// fakeSpawnTimes is the world state the spawn-time makers read.
type fakeSpawnTimes struct {
	mu       sync.Mutex
	active   map[int]bool
	owned    map[int]bool
	hall     map[int]bool
	coreOpen bool
}

func (f *fakeSpawnTimes) CastleSiege(id int) (active, owned, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != 1 && id != 2 {
		return false, false, false
	}
	return f.active[id], f.owned[id], true
}

func (f *fakeSpawnTimes) HallSiegeInProgress(id int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hall[id]
}

func (f *fakeSpawnTimes) DoorOpen(id int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return id == coreDoorID && f.coreOpen
}

func spawnTimeFixture(t *testing.T, st *fakeSpawnTimes) *eventsFixture {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "spawntime.xml"), spawnTimeSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	var templates []*npc.Template
	for id := 1; id <= 6; id++ {
		templates = append(templates, &npc.Template{ID: id, TemplateID: id, Type: "Monster", HPMax: 100, RunSpeed: 100})
	}
	f := newEventsFixture(t, table, npc.NewTable(templates), nil)
	if st != nil {
		f.npcs.SetSpawnTimes(st)
	}
	return f
}

// TestSpawnTimeStartStop pins SpawnManager.startSpawnTime and
// stopSpawnTime: no spawn-time maker spawns at boot; a start runs the
// makers whose spawn time has the name, ignoring case, and whose first
// parameters equal the arguments, ignoring case, a missing argument
// matching none; a stop deletes their NPCs. The default maker spawns a
// started maker's NPCs only while its spawn condition does not hold
// (SpawnMaker.checkHasSpawnCondition): a castle's siege guards while its
// siege zone is active and it has no owner, its owner's guards while the
// zone is active and it has one.
func TestSpawnTimeStartStop(t *testing.T) {
	st := &fakeSpawnTimes{active: map[int]bool{}, owned: map[int]bool{}, hall: map[int]bool{}}
	f := spawnTimeFixture(t, st)
	if got := f.spawnedIDs(); len(got) != 0 {
		t.Fatalf("boot spawned %v, want none", got)
	}

	if n := f.npcs.StartSpawnTime("siege_warfare_start", "1"); n != 0 || len(f.spawnedIDs()) != 0 {
		t.Fatalf("start with the zone inactive: %d, %v; want none", n, f.spawnedIDs())
	}

	st.active[1] = true
	if n := f.npcs.StartSpawnTime("SIEGE_WARFARE_START", "1"); n != 2 {
		t.Fatalf("start = %d, want 2", n)
	}
	if n := f.npcs.StartSpawnTime("siege_warfare_start", "1"); n != 2 {
		t.Fatalf("second start = %d, want the same 2", n)
	}
	f.npcs.StartSpawnTime("pc_siege_warfare_start", "1")
	if got := f.spawnedIDs(); !slices.Equal(got, []int{1, 1}) {
		t.Fatalf("ownerless castle: %v, want [1 1]", got)
	}
	if n := f.npcs.StopSpawnTime("siege_warfare_start", "1"); n != 2 || len(f.spawnedIDs()) != 0 {
		t.Fatalf("stop = %d, %v; want 2, none", n, f.spawnedIDs())
	}

	st.owned[1] = true
	f.npcs.StartSpawnTime("siege_warfare_start", "1")
	f.npcs.StartSpawnTime("pc_siege_warfare_start", "1")
	if got := f.spawnedIDs(); !slices.Equal(got, []int{2}) {
		t.Fatalf("owned castle: %v, want [2]", got)
	}
	f.npcs.StopSpawnTime("pc_siege_warfare_start", "1")

	f.npcs.StartSpawnTime("siege_warfare_start")
	f.npcs.StartSpawnTime("siege_warfare_start", "3")
	f.npcs.StartSpawnTime("siege_start", "1")
	if got := f.spawnedIDs(); len(got) != 0 {
		t.Fatalf("unmatched starts spawned %v", got)
	}
}

// TestSpawnTimeMatch pins the maker selection of startSpawnTime: the name
// ignores case, each of the maker's parameters needs an argument equal to
// it, ignoring case, and arguments past the maker's parameters are not
// read.
func TestSpawnTimeMatch(t *testing.T) {
	f := spawnTimeFixture(t, nil)
	names := func(name string, args ...string) []string {
		var out []string
		for _, g := range f.npcs.spawnTimeGroups(name, args) {
			out = append(out, g.def.Name)
		}
		return out
	}
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"siege_warfare_start", []string{"1"}, []string{"gludio_guards"}},
		{"Pc_Siege_Warfare_Start", []string{"1", "extra"}, []string{"gludio_owner_guards"}},
		{"siege_warfare_start", []string{"2"}, []string{"dion_guards"}},
		{"siege_warfare_start", nil, nil},
		{"agit_battle_royal_start", []string{"63", "BLUE", "1"}, []string{"royal"}},
		{"agit_battle_royal_start", []string{"63", "blue"}, nil},
		{"agit_battle_royal_start", []string{"63", "blue", "2"}, nil},
		{"door_open", []string{"[KURUMA_PARENT]"}, []string{"kuruma"}},
		{"", []string{"1"}, nil},
		{"unknown_start", []string{"1"}, nil},
	} {
		if got := names(tc.name, tc.args...); !slices.Equal(got, tc.want) {
			t.Errorf("%s%v = %v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}

// TestSpawnTimeHeldBranches pins the other spawn conditions: Core's door
// makers spawn only while door 20210001 stands open, a hall's defenders
// only while its siege is being fought, a battle royal maker never through
// the default maker, and with no world state every spawn time holds.
func TestSpawnTimeHeldBranches(t *testing.T) {
	st := &fakeSpawnTimes{active: map[int]bool{1: true}, owned: map[int]bool{}, hall: map[int]bool{}}
	f := spawnTimeFixture(t, st)
	f.npcs.StartSpawnTime("door_open", "[kuruma_parent]")
	f.npcs.StartSpawnTime("agit_defend_warfare_start", "34")
	f.npcs.StartSpawnTime("agit_battle_royal_start", "63", "blue", "1")
	if got := f.spawnedIDs(); len(got) != 0 {
		t.Fatalf("held makers spawned %v", got)
	}
	st.coreOpen = true
	st.hall[34] = true
	f.npcs.StartSpawnTime("door_open", "[kuruma_parent]")
	f.npcs.StartSpawnTime("agit_defend_warfare_start", "34")
	if got := f.spawnedIDs(); !slices.Equal(got, []int{5, 6}) {
		t.Fatalf("released makers: %v, want [5 6]", got)
	}

	bare := spawnTimeFixture(t, nil)
	bare.npcs.StartSpawnTime("siege_warfare_start", "1")
	if got := bare.spawnedIDs(); len(got) != 0 {
		t.Fatalf("no world state: spawned %v", got)
	}
}
