package npcs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The maker fixture's NPC templates, one per npcmaker.
const (
	makerPlainID     = 21001 // no maker type
	makerDefaultID   = 21002 // default_maker
	makerNoStartID   = 21003 // no_on_start_maker
	makerEventOnID   = 21004 // event_maker, EventName listed
	makerEventOffID  = 21005 // event_maker, EventName not listed
	makerScriptedID  = 21006 // a scripted type with no maker of its own
	makerGroupID     = 21007 // default_maker, three NPCs, maximum three
	makerSSQEventID  = 21011
	makerSeal1NoneID = 21012
	makerSeal1DawnID = 21013
	makerSeal1DuskID = 21014
	makerSeal2NoneID = 21015
	makerSeal2DawnID = 21016
	makerSeal2DuskID = 21017
)

// makerSpawnlist declares one npcmaker per maker case, every NPC at a
// fixed point in sight of the fixture character at (10, 20).
const makerSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="plain_m" territory="field" maximumNpcs="1">
		<npc id="21001" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="default_m" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21002" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="nostart_m" territory="field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21003" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="event_on" territory="field" maximumNpcs="1">
		<ai type="event_maker"><set name="EventName" val="18age"/></ai>
		<npc id="21004" total="1" pos="70;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="event_off" territory="field" maximumNpcs="1">
		<ai type="event_maker"><set name="EventName" val="christmas"/></ai>
		<npc id="21005" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="scripted_m" territory="field" maximumNpcs="1">
		<ai type="random_spawn"/>
		<npc id="21006" total="1" pos="90;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="group_m" territory="field" maximumNpcs="3">
		<ai type="default_maker"/>
		<npc id="21007" total="3" pos="100;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="ssq_ev" territory="field" event="ssq_event" maximumNpcs="3">
		<ai type="default_maker"/>
		<npc id="21011" total="3" pos="110;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="seal1_none" territory="field" event="ssq_seal1_none" maximumNpcs="1"><ai type="default_maker"/><npc id="21012" total="1" pos="120;20;30;0" respawn="60sec"/></npcmaker>
	<npcmaker name="seal1_dawn" territory="field" event="ssq_seal1_dawn" maximumNpcs="1"><ai type="default_maker"/><npc id="21013" total="1" pos="130;20;30;0" respawn="60sec"/></npcmaker>
	<npcmaker name="seal1_twilight" territory="field" event="ssq_seal1_twilight" maximumNpcs="1"><ai type="default_maker"/><npc id="21014" total="1" pos="140;20;30;0" respawn="60sec"/></npcmaker>
	<npcmaker name="seal2_none" territory="field" event="ssq_seal2_none" maximumNpcs="1"><ai type="default_maker"/><npc id="21015" total="1" pos="150;20;30;0" respawn="60sec"/></npcmaker>
	<npcmaker name="seal2_dawn" territory="field" event="ssq_seal2_dawn" maximumNpcs="1"><ai type="default_maker"/><npc id="21016" total="1" pos="160;20;30;0" respawn="60sec"/></npcmaker>
	<npcmaker name="seal2_twilight" territory="field" event="ssq_seal2_twilight" maximumNpcs="1"><ai type="default_maker"/><npc id="21017" total="1" pos="170;20;30;0" respawn="60sec"/></npcmaker>
</list>`

func makerTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "makers.xml"), []byte(makerSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

func makerTemplates() *npc.Table {
	var templates []*npc.Template
	for _, id := range []int{
		makerPlainID, makerDefaultID, makerNoStartID, makerEventOnID, makerEventOffID, makerScriptedID, makerGroupID,
		makerSSQEventID, makerSeal1NoneID, makerSeal1DawnID, makerSeal1DuskID, makerSeal2NoneID, makerSeal2DawnID, makerSeal2DuskID,
	} {
		templates = append(templates, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Maker Wolf", Level: 1, HPMax: 100,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true,
		})
	}
	return npc.NewTable(templates)
}

// bootMakers boots the maker fixture with the Seven Signs status seeded as
// status says, and the character in the world.
func bootMakers(t *testing.T, status func(*sevensigns.StatusRow), extra ...gameservertest.Option) *makerWorld {
	t.Helper()
	w := &makerWorld{}
	opts := append([]gameservertest.Option{
		gameservertest.WithNPCs(makerTemplates()),
		gameservertest.WithNpcSpawns(makerTable(t)),
		withSevenSignsStatus(t, status),
		gameservertest.WithSevenSignsTimer(func(_ time.Duration, fn func()) *time.Timer {
			w.change = fn
			return nil
		}),
	}, extra...)
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
	}, opts...)...)
	w.srv = srv
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	w.burst = enterWorld(t, srv, c)
	return w
}

// withSevenSignsStatus edits the seven_signs_status row before the Seven
// Signs state restores it.
func withSevenSignsStatus(t *testing.T, edit func(*sevensigns.StatusRow)) gameservertest.Option {
	return gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
		ctx := context.Background()
		row, found, err := store.LoadStatus(ctx)
		if err != nil || !found {
			t.Errorf("load status row: found=%v err=%v", found, err)
			return
		}
		edit(&row)
		if err := store.SaveStatus(ctx, row); err != nil {
			t.Errorf("seed status row: %v", err)
		}
	})
}

// competitionStatus is the competition period.
func competitionStatus(row *sevensigns.StatusRow) { row.Period = sevensigns.Competition }

// validationStatus is seal validation with Dawn ahead on score, owning
// Avarice, and Dusk owning Gnosis.
func validationStatus(row *sevensigns.StatusRow) {
	row.Period = sevensigns.SealValidation
	row.DawnStoneScore, row.DuskStoneScore = 100, 0
	row.DawnFestivalScore, row.DuskFestivalScore = 0, 0
	row.SealOwners = [3]sevensigns.Cabal{sevensigns.Dawn, sevensigns.Dusk, sevensigns.NoCabal}
}

type makerWorld struct {
	srv    *gameservertest.Server
	burst  [][]byte
	change func()
}

// shown returns the template ids of the NpcInfo frames among frames,
// sorted.
func shown(frames [][]byte) []int {
	var out []int
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32()
		out = append(out, int(r.ReadInt32())-1000000)
	}
	slices.Sort(out)
	return out
}

// deleted returns the object ids of the DeleteObject frames among frames.
func deleted(frames [][]byte) []int32 {
	var out []int32
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeDeleteObject {
			out = append(out, wire.NewReader(frame[1:]).ReadInt32())
		}
	}
	slices.Sort(out)
	return out
}

// inWorld returns the hostiles of template id in the world.
func (w *makerWorld) inWorld(id int) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == id {
			out = append(out, h)
		}
	}
	slices.SortFunc(out, func(a, b *npc.Hostile) int { return int(a.ObjectID() - b.ObjectID()) })
	return out
}

func objectIDs(hs []*npc.Hostile) []int32 {
	var out []int32
	for _, h := range hs {
		out = append(out, h.ObjectID())
	}
	slices.Sort(out)
	return out
}

func repeatID(id, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = id
	}
	return out
}

// Boot dispatches on the maker type: the plain, default and scripted-but-
// unported makers spawn as the default maker does, the no-on-start maker
// spawns nothing, and an event_maker spawns only with its EventName in
// SpawnEvents (the shipped default lists 18age, not christmas). The
// competition spawns the Seven Signs event group and no seal group. The
// character sees exactly those NPCs.
func TestMakerBootDispatchesOnMakerType(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, competitionStatus)

	want := []int{makerPlainID, makerDefaultID, makerEventOnID, makerScriptedID}
	want = append(want, repeatID(makerGroupID, 3)...)
	want = append(want, repeatID(makerSSQEventID, 3)...)
	slices.Sort(want)
	if got := shown(w.burst); !slices.Equal(got, want) {
		t.Fatalf("NPCs shown at enter world = %v, want %v", got, want)
	}
	for _, id := range []int{makerNoStartID, makerEventOffID, makerSeal1NoneID, makerSeal2NoneID} {
		if got := w.inWorld(id); len(got) != 0 {
			t.Errorf("npc %d in the world: %d, want none", id, len(got))
		}
	}
}

// A default maker's NPC that leaves the world arms its respawn, and the
// respawn brings a new NPC the character is shown.
func TestDefaultMakerRespawnsADeletedNPC(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, competitionStatus)
	c := w.srv.Client

	old := w.inWorld(makerDefaultID)[0]
	old.DeleteMe()
	w.srv.Settle(t)
	if got := deleted(drainFrames(t, c)); !slices.Equal(got, []int32{old.ObjectID()}) {
		t.Fatalf("deleted objects = %v, want [%d]", got, old.ObjectID())
	}
	const key = "default_m#0#0"
	if !w.srv.NpcRespawns.Tracked(key) {
		t.Fatal("deleted default maker NPC armed no respawn")
	}
	w.srv.NpcSpawns.Respawn(key)
	w.srv.Settle(t)
	if got := shown(drainFrames(t, c)); !slices.Equal(got, []int{makerDefaultID}) {
		t.Fatalf("NPCs shown after the respawn = %v, want [%d]", got, makerDefaultID)
	}
	if now := w.inWorld(makerDefaultID); len(now) != 1 || now[0].ObjectID() == old.ObjectID() {
		t.Fatalf("respawned NPCs = %v, want one new NPC", objectIDs(now))
	}
}

// Maker script event 1000 deletes every NPC of the maker, with no respawn;
// 1001 brings the maker back to its maximum int1 seconds later, an NPC
// that left the world through its own respawn at that delay.
func TestDefaultMakerScriptEvents(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, competitionStatus)
	c := w.srv.Client
	npcs := w.srv.NpcSpawns

	group := w.inWorld(makerGroupID)
	if len(group) != 3 {
		t.Fatalf("group NPCs at boot = %d, want 3", len(group))
	}
	if !npcs.MakerEvent("GROUP_M", "1000", 0, 0) {
		t.Fatal("maker group_m not found by name")
	}
	w.srv.Settle(t)
	if got := deleted(drainFrames(t, c)); !slices.Equal(got, objectIDs(group)) {
		t.Fatalf("deleted objects after 1000 = %v, want %v", got, objectIDs(group))
	}
	for i := range 3 {
		if key := fmt.Sprintf("group_m#0#%d", i); w.srv.NpcRespawns.Tracked(key) {
			t.Fatalf("slot %s armed a respawn after 1000", key)
		}
	}

	npcs.MakerEvent("group_m", "1001", 2, 0)
	w.srv.Settle(t)
	if got := w.inWorld(makerGroupID); len(got) != 0 {
		t.Fatalf("group NPCs before the delay = %d, want 0", len(got))
	}
	w.srv.Advance(t, 2*time.Second)
	if got := shown(drainFrames(t, c)); !slices.Equal(got, repeatID(makerGroupID, 3)) {
		t.Fatalf("NPCs shown after 1001 = %v, want three of %d", got, makerGroupID)
	}

	// One NPC leaves: 1001 respawns it rather than spawning a fourth.
	gone := w.inWorld(makerGroupID)[0]
	gone.DeleteMe()
	w.srv.Settle(t)
	drainFrames(t, c)
	npcs.MakerEvent("group_m", "1001", 1, 0)
	w.srv.Advance(t, time.Second)
	if got := shown(drainFrames(t, c)); len(got) != 0 {
		t.Fatalf("NPCs shown after the second 1001 = %v, want none before the respawn fires", got)
	}
	if got := len(w.inWorld(makerGroupID)); got != 2 {
		t.Fatalf("group NPCs = %d, want 2", got)
	}

	// A maker whose spawn condition holds ignores 1001.
	npcs.MakerEvent("event_off", "1001", 0, 0)
	w.srv.Advance(t, time.Second)
	if got := w.inWorld(makerEventOffID); len(got) != 0 {
		t.Fatalf("held event maker spawned %d NPCs on 1001", len(got))
	}
}

// A no-on-start maker spawns when something starts it: 1001 fills it.
func TestNoOnStartMakerSpawnsOnScriptEvent(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, competitionStatus)
	w.srv.NpcSpawns.MakerEvent("nostart_m", "1001", 0, 0)
	w.srv.Advance(t, time.Millisecond)
	if got := shown(drainFrames(t, w.srv.Client)); !slices.Equal(got, []int{makerNoStartID}) {
		t.Fatalf("NPCs shown = %v, want [%d]", got, makerNoStartID)
	}
}

// At seal validation the seal groups follow the seal owners and the
// winner: Dawn won and owns Avarice, so its Dawn group stands; Dusk owns
// Gnosis without winning, so its ownerless group does. The period change
// to recruiting swaps them for the event group, and the character sees
// the swap.
func TestSevenSignsPeriodChangeSwapsGroups(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, validationStatus)
	c := w.srv.Client

	want := []int{makerPlainID, makerDefaultID, makerEventOnID, makerScriptedID, makerSeal1DawnID, makerSeal2NoneID}
	want = append(want, repeatID(makerGroupID, 3)...)
	slices.Sort(want)
	if got := shown(w.burst); !slices.Equal(got, want) {
		t.Fatalf("NPCs shown at enter world = %v, want %v", got, want)
	}
	seals := append(w.inWorld(makerSeal1DawnID), w.inWorld(makerSeal2NoneID)...)

	if w.change == nil {
		t.Fatal("no period change pending")
	}
	w.change()
	w.srv.Settle(t)
	frames := drainFrames(t, c)
	if got := deleted(frames); !slices.Equal(got, objectIDs(seals)) {
		t.Fatalf("deleted objects = %v, want the seal NPCs %v", got, objectIDs(seals))
	}
	if got := shown(frames); !slices.Equal(got, repeatID(makerSSQEventID, 3)) {
		t.Fatalf("NPCs shown = %v, want the event group", got)
	}
}

// The maker's created and deleted hooks race a respawn and a Seven Signs
// period change: one goroutine keeps swapping the groups while another
// keeps deleting the event group's NPCs and respawning their slots. Once
// both stop and one last change runs, the event group stands at its total
// and every NPC in the world belongs to a spawn slot, on both executors.
func TestMakerHooksRaceRespawnAndPeriodChange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootMakers(t, competitionStatus, tc.opts...)
			npcs := w.srv.NpcSpawns

			done := make(chan struct{})
			go func() {
				defer close(done)
				for range 20 {
					npcs.SevenSignsChanged()
				}
			}()
			for range 20 {
				for _, h := range w.inWorld(makerSSQEventID) {
					h.DeleteMe()
				}
				for i := range 64 {
					npcs.Respawn(fmt.Sprintf("ssq_ev#0#%d", i))
				}
			}
			<-done
			w.srv.Settle(t)
			npcs.SevenSignsChanged()
			w.srv.Settle(t)

			if got := len(w.inWorld(makerSSQEventID)); got != 3 {
				t.Fatalf("event group NPCs = %d, want 3", got)
			}
			inWorld := 0
			for _, obj := range w.srv.State.Objects() {
				switch obj.(type) {
				case *npc.Hostile, *npc.Folk:
					inWorld++
				}
			}
			if got := npcs.LiveCount(); got != inWorld {
				t.Fatalf("NPCs tracked by a slot = %d, NPCs in the world = %d", got, inWorld)
			}
		})
	}
}
