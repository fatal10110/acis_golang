package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Frintezza (#198 PR 3), his organ dummy, and two listener NPCs that claim
// the global memo keys of his follower (5) and of its transform (6) as they
// appear and record the script events they hear.
const (
	fzID          = int32(29045)
	fzOrganID     = int32(29052)
	fzFollowerID  = int32(29996)
	fzTransformID = int32(29997)
	fzRow         = "frintessa"
)

// fzSpawnlist places the listeners first, then Frintezza under the shipped
// database maker, and declares the npcmakers he signals as probes. The
// tomb holds the character.
const fzSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fr_tomb" minZ="0" maxZ="100"><node x="-400" y="-400"/><node x="400" y="-400"/><node x="400" y="400"/><node x="-400" y="400"/></territory>
	<npcmaker name="fz_memo" territory="fr_tomb" maximumNpcs="2">
		<npc id="29996" total="1" pos="-200;20;30;0" respawn="0sec"/>
		<npc id="29997" total="1" pos="-200;120;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="frintessa_maker" territory="fr_tomb" maximumNpcs="9">
		<ai type="default_use_db_maker_for_frintessa"/>
		<npc id="29045" pos="200;20;30;0" total="1" respawn="48hour" dbName="frintessa"/>
	</npcmaker>
	<npcmaker name="frintessa_2515_m01" territory="fr_tomb" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
	<npcmaker name="frintessa_evilate_maker1" territory="fr_tomb" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
	<npcmaker name="frintessa_evilate_maker2" territory="fr_tomb" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
	<npcmaker name="frintessa_teleportcuve" territory="fr_tomb" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
</list>`

// fzDoorNames are the tomb's doors in the order Frintezza sets them: the
// four grave pathways, then the walls.
var fzDoorNames = []string{
	"grave_pathway_1", "grave_pathway_2", "grave_pathway_3", "grave_pathway_4",
	"wall_door_a_center_1", "wall_door_a_center_2", "wall_door_a_center_3", "wall_door_a_center_4",
	"wall_door_a_center_5", "wall_door_a_center_6", "wall_door_a_center_7", "wall_door_a_center_8",
	"wall_door_b_left_1", "wall_door_b_left_2", "wall_door_b_left_3", "wall_door_b_left_4", "wall_door_b_left_5",
	"wall_door_b_right_1", "wall_door_b_right_2", "wall_door_b_right_3", "wall_door_b_right_4", "wall_door_b_right_5",
}

// fzDoorID is the door id of the fixture door at index i of fzDoorNames.
func fzDoorID(i int) int { return 25150001 + i }

// fzDoors are the tomb's doors as fixtures, every one open, spread out
// away from the tomb.
func fzDoors() []*door.Template {
	var out []*door.Template
	for i, name := range fzDoorNames {
		at := location.Location{X: 3000 + i*300, Y: 3000, Z: 30}
		out = append(out, &door.Template{
			ID: fzDoorID(i), Name: name, Kind: door.KindDoor, Level: 1, Opened: true,
			Position:    at,
			Coordinates: []location.Point{{X: at.X - 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y + 100}, {X: at.X - 50, Y: at.Y + 100}},
			HP:          100000, PDef: 600, MDef: 500, Height: 1024,
		})
	}
	return out
}

// The door states Frintezza sets: sealed, pathways 3 and 4 open and the
// rest closed; shut, every door closed; and every door open as booted.
var (
	fzSealed = func() []bool { s := make([]bool, len(fzDoorNames)); s[2], s[3] = true, true; return s }()
	fzShut   = make([]bool, len(fzDoorNames))
	fzOpen   = func() []bool {
		s := make([]bool, len(fzDoorNames))
		for i := range s {
			s[i] = true
		}
		return s
	}()
)

// bootFrintezza boots the tomb backed by the spawn_data table, after
// saving row as Frintezza's live spawn_data row when it is set.
func bootFrintezza(t *testing.T, row *spawn.State) *frWorld {
	t.Helper()
	return bootFrintezzaDrawn(t, row, &ind1Draws{})
}

// bootFrintezzaDrawn is bootFrintezza with the script draws draws, set
// before Frintezza appears.
func bootFrintezzaDrawn(t *testing.T, row *spawn.State, draws *ind1Draws) *frWorld {
	t.Helper()
	if row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{fzRow: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fz.xml"), []byte(fzSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	heard := &frHeard{}
	const memo = "test.FzMemo"
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":    defaultnpc.New,
		"script.ai.boss.frintezza.Frintezza": frintezza.Frintezza,
		"test." + frDirector:                 func() script.Script { return script.Script{} },
		memo: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{fzFollowerID, fzTransformID}, Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					key := "5"
					if e.NPC.NpcID() == fzTransformID {
						key = "6"
					}
					s.GlobalMemo().Set(key, int(e.NPC.ObjectID()))
				},
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
					heard.mu.Lock()
					defer heard.mu.Unlock()
					heard.events = append(heard.events, fmt.Sprintf("%d event=%d arg1=%d arg2=%d", e.NPC.NpcID(), e.EventID, e.Arg1, e.Arg2))
				},
			}}
		},
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.frintezza.Frintezza"},
		{Path: memo},
		{Path: "test." + frDirector},
	}
	boss := warriorTemplate(fzID, nil)
	boss.Type, boss.CorpseTime = "GrandBoss", 18
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range []*npc.Template{boss, warriorTemplate(fzOrganID, nil), warriorTemplate(fzFollowerID, nil), warriorTemplate(fzTransformID, nil)} {
		tmpl.CanMove = false
		templates = append(templates, tmpl)
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	song := func(id modelskill.ID, level int) modelskill.Definition {
		d := ind1Skill(id, 600, true)
		d.Level = level
		return d
	}
	skills := modelskill.NewTable([]modelskill.Definition{
		song(5006, 1), song(5007, 1), song(5007, 2), song(5007, 3), song(5007, 4), song(5007, 5),
	})
	makers := &rb5Makers{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(skills),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers.catalog()),
		gameservertest.WithDoors(fzDoors()...),
	}}, templates...)
	return &frWorld{warriorWorld: w, draws: draws, heard: heard, makers: makers}
}

// frintezza returns Frintezza in the world.
func (w *frWorld) frintezza(t *testing.T) *npc.Hostile {
	t.Helper()
	live := w.liveNPCs(fzID)
	if len(live) != 1 {
		t.Fatalf("Frintezzas in the world = %d, want 1", len(live))
	}
	return live[0]
}

// listener returns the listener NPC of id.
func (w *frWorld) listener(t *testing.T, id int32) *npc.Hostile {
	t.Helper()
	live := w.liveNPCs(id)
	if len(live) != 1 {
		t.Fatalf("listeners %d in the world = %d, want 1", id, len(live))
	}
	return live[0]
}

// wantDoors fails unless the tomb's doors stand open as want says.
func (w *frWorld) wantDoors(t *testing.T, when string, want []bool) {
	t.Helper()
	got := make([]bool, len(fzDoorNames))
	for i := range fzDoorNames {
		_, got[i] = w.srv.DoorState(t, fzDoorID(i))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s: doors open = %v, want %v", when, got, want)
	}
}

// fzLiveRow is a live spawn_data row of Frintezza at his spawn point
// holding value, his HP more than full.
func fzLiveRow(value int) *spawn.State {
	return &spawn.State{Name: fzRow, Status: spawn.StatusAlive, CurrentHP: 1 << 20, CurrentMP: 1 << 20, Location: location.Location{X: 200, Y: 20, Z: 30}, DBValue: value}
}

// fzRowValue is the db value Shutdown saved in Frintezza's row.
func fzRowValue(t *testing.T, srv *gameservertest.Server) int {
	t.Helper()
	states, err := gamesql.NewSpawnStore(srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	row, ok := states[fzRow]
	if !ok {
		t.Fatalf("no spawn_data row %q", fzRow)
	}
	return row.DBValue
}

// fzText is the on-screen text of song name id, shown for 3 s.
func fzText(t *testing.T, id int32) string {
	t.Helper()
	text, ok := npcstring.Text(id)
	if !ok {
		t.Fatalf("no npc string %d", id)
	}
	return text + "@3000"
}

// fzSong is a song Frintezza wants to cast on himself, as frCastDesires
// renders it.
func fzSong(f *npc.Hostile, skill string) string {
	return fmt.Sprintf("%s>%d=10000", skill, f.ObjectID())
}

// fzSocials renders h's queued social desires as "social/timer=weight".
func fzSocials(h *npc.Hostile) []string {
	var out []string
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionSocial {
			out = append(out, fmt.Sprintf("%d/%d=%v", d.ItemObjectID, d.Timer, d.Weight))
		}
	}
	slices.Sort(out)
	return out
}

// TestFrintezzaSealsTheTomb runs the created hook from a spawn with no row:
// he claims memo key 4, sets his db value 0, runs maker event 1001 on
// frintessa_2515_m01, closes grave pathways 1 and 2 and every wall, opens
// pathways 3 and 4, and tells his follower (key 5) and its transform (key
// 6) 0, his object id as the event id. Shutdown saves 0.
func TestFrintezzaSealsTheTomb(t *testing.T) {
	t.Parallel()
	w := bootFrintezza(t, nil)
	f := w.frintezza(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	if got := w.memo(t, "4"); got != int(id) {
		t.Fatalf("memo 4 = %d, want Frintezza %d", got, id)
	}
	if n.DBLoaded() || n.DBValue() != 0 {
		t.Fatalf("Frintezza loaded=%v value=%d, want not loaded at 0", n.DBLoaded(), n.DBValue())
	}
	if got, want := w.makers.take(), []string{"frintessa_2515_m01 1001"}; !slices.Equal(got, want) {
		t.Fatalf("maker events = %q, want %q", got, want)
	}
	want := []string{fmt.Sprintf("29996 event=%d arg1=0 arg2=0", id), fmt.Sprintf("29997 event=%d arg1=0 arg2=0", id)}
	if got := w.heard.take(); !slices.Equal(got, want) {
		t.Fatalf("heard as Frintezza appeared = %q, want %q", got, want)
	}
	w.wantDoors(t, "sealed", fzSealed)
	if got := w.liveNPCs(fzOrganID); len(got) != 0 {
		t.Fatalf("organ dummies = %d, want none", len(got))
	}
	w.srv.Shutdown(t)
	if got := fzRowValue(t, w.srv); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestFrintezzaResumesHisStage runs the created hook from a row restored
// at each stage. At 1 he runs maker event 1001 on frintessa_2515_m01 and
// leaves the doors be. At 2 he runs maker event 1000 there and, 10 minutes
// later, jumps to his hall facing 16384 with every door closed. At 3 and 5
// he closes every door, leaves his organ dummy at the hall facing 16384,
// runs maker event 1000 and plays the stage's song: at 3 the plain music
// (5006 level 1, screen text 1000527) after five missed draws; at 5 the
// healing song (5007 level 1, 1000522) on a draw of 1999 of 2000. A second
// later, his timer 3000 plays the fight song of the stage: at 5 its
// healing odds are 2 in 10000, so a draw of 2 then 1999 is the song of
// power (5007 level 3, 1000523).
func TestFrintezzaResumesHisStage(t *testing.T) {
	t.Parallel()
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		w := bootFrintezza(t, fzLiveRow(1))
		if got, want := w.makers.take(), []string{"frintessa_2515_m01 1001"}; !slices.Equal(got, want) {
			t.Fatalf("maker events = %q, want %q", got, want)
		}
		w.wantDoors(t, "restored at 1", fzOpen)
		if got := w.heard.take(); len(got) != 0 {
			t.Fatalf("heard = %q, want nothing", got)
		}
	})
	t.Run("2", func(t *testing.T) {
		t.Parallel()
		w := bootFrintezza(t, fzLiveRow(2))
		f := w.frintezza(t)
		if got, want := w.makers.take(), []string{"frintessa_2515_m01 1000"}; !slices.Equal(got, want) {
			t.Fatalf("maker events = %q, want %q", got, want)
		}
		w.srv.Advance(t, 10*time.Minute-time.Millisecond)
		if x, _, _ := f.Position(); x != 200 {
			t.Fatalf("Frintezza moved before 10 minutes: at x %d", x)
		}
		w.wantDoors(t, "before 10 minutes", fzOpen)
		w.srv.Advance(t, time.Millisecond)
		if got, want := frAt(f), [4]int{174239, -89808, -5016, 16384}; got != want {
			t.Fatalf("Frintezza at %v, want his hall %v", got, want)
		}
		w.wantDoors(t, "at 10 minutes", fzShut)
	})
	for _, tc := range []struct {
		value       int
		draws       []int
		song        string
		fight       []int
		fightSong   string
		fightTextID int32
	}{
		{3, []int{0, 0, 0, 0, 0}, "5006-1", []int{0, 0, 0, 0, 0}, "5006-1", 1000527},
		{5, []int{1999}, "5007-1", []int{2, 1999}, "5007-3", 1000523},
	} {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			t.Parallel()
			draws := &ind1Draws{}
			draws.set(9999, tc.draws...)
			w := bootFrintezzaDrawn(t, fzLiveRow(tc.value), draws)
			f := w.frintezza(t)
			n := script.NPCOf(f)
			if !n.DBLoaded() || n.DBValue() != tc.value {
				t.Fatalf("Frintezza loaded=%v value=%d, want loaded at %d", n.DBLoaded(), n.DBValue(), tc.value)
			}
			w.wantDoors(t, "restored", fzShut)
			organs := w.liveNPCs(fzOrganID)
			if len(organs) != 1 || frAt(organs[0]) != [4]int{174239, -89809, -5016, 16384} {
				t.Fatalf("organ dummies = %d, want one at the hall facing 16384", len(organs))
			}
			if got, want := w.makers.take(), []string{"frintessa_2515_m01 1000"}; !slices.Equal(got, want) {
				t.Fatalf("maker events = %q, want %q", got, want)
			}
			if got, want := frCastDesires(f), []string{fzSong(f, tc.song)}; !slices.Equal(got, want) {
				t.Fatalf("cast desires after the restore = %q, want %q", got, want)
			}
			for _, slot := range []script.IntSlot{script.SlotIntAI0, script.SlotIntAI1, script.SlotIntAI2, script.SlotIntAI3, script.SlotIntAI4, npc.IntQuest0, npc.IntQuest1, npc.IntQuest2, npc.IntQuest3} {
				if got := n.Int(slot); got != 0 {
					t.Fatalf("slot %d after the created hook = %d, want 0", slot, got)
				}
			}
			w.onQueue(t, f, func() { f.AI().Desires().Clear() })
			w.srv.ReadQueued(t, w.srv.Client)
			w.draws.set(9999, tc.fight...)
			w.srv.Advance(t, time.Second)
			if got, want := frCastDesires(f), []string{fzSong(f, tc.fightSong)}; !slices.Equal(got, want) {
				t.Fatalf("cast desires at timer 3000 = %q, want %q", got, want)
			}
			if got, want := screenTexts(w.srv.ReadQueued(t, w.srv.Client)), []string{fzText(t, tc.fightTextID)}; !slices.Equal(got, want) {
				t.Fatalf("screen texts at timer 3000 = %q, want %q", got, want)
			}
		})
	}
}
