package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Frintezza's follower (#198 PR 1), the dummy it leaves, and two listener
// NPCs that claim the global memo keys of Frintezza (4) and of the
// follower's transform (6) as they appear and record the script events
// they hear.
const (
	frFollowerID  = int32(29046)
	frDummyID     = int32(29053)
	frFrintezzaID = int32(29993)
	frTransformID = int32(29994)
	frRow         = "follower_of_frintessa"
	frDirector    = "FrDirector"
)

// frSpawnlist places the listeners first, then the follower under its
// shipped database maker, and declares the two evil spirit npcmakers as
// probes that record the maker script events they get. The tomb holds the
// character.
const frSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fr_tomb" minZ="0" maxZ="100"><node x="-400" y="-400"/><node x="400" y="-400"/><node x="400" y="400"/><node x="-400" y="400"/></territory>
	<npcmaker name="fr_memo" territory="fr_tomb" maximumNpcs="2">
		<npc id="29993" total="1" pos="-200;20;30;0" respawn="0sec"/>
		<npc id="29994" total="1" pos="-200;120;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="frintessa_maker" territory="fr_tomb" maximumNpcs="9">
		<ai type="default_use_db_maker_for_frintessa"/>
		<npc id="29046" pos="200;20;30;0" total="1" respawn="48hour" dbName="follower_of_frintessa"/>
	</npcmaker>
	<npcmaker name="frintessa_evilate_maker1" territory="fr_tomb" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
	<npcmaker name="frintessa_evilate_maker2" territory="fr_tomb" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
</list>`

// frHeard records the script events the listeners hear as
// "<npc id> event=<id> arg1=<n> arg2=<n>".
type frHeard struct {
	mu     sync.Mutex
	events []string
}

func (h *frHeard) take() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.events
	h.events = nil
	return out
}

// frWorld is the character in the tomb beside the follower.
type frWorld struct {
	*warriorWorld
	draws  *ind1Draws
	heard  *frHeard
	makers *rb5Makers
}

// frTemplates are the fixture templates: the follower of type kind whose
// corpse lasts 18 s, as the shipped one's, seeing 1000 around itself, none
// of them walking.
func frTemplates(kind string) []*npc.Template {
	follower := warriorTemplate(frFollowerID, nil)
	follower.Type, follower.CorpseTime, follower.AggroRange = kind, 18, 1000
	var out []*npc.Template
	for _, tmpl := range []*npc.Template{follower, warriorTemplate(frDummyID, nil), warriorTemplate(frFrintezzaID, nil), warriorTemplate(frTransformID, nil)} {
		tmpl.CanMove = false
		out = append(out, tmpl)
	}
	return out
}

// frSkills are the follower's skills as fixtures: its body change on
// itself, the rest on one target.
func frSkills() *modelskill.Table {
	skill := func(id modelskill.ID, level int, self bool) modelskill.Definition {
		d := ind1Skill(id, 600, self)
		d.Level = level
		return d
	}
	return modelskill.NewTable([]modelskill.Definition{
		skill(4258, 1, false), skill(5014, 1, false), skill(5014, 2, false), skill(5015, 2, false),
		skill(5015, 4, false), skill(5016, 1, false), skill(5018, 1, false), skill(5017, 1, true),
	})
}

// bootFollower boots the tomb backed by the spawn_data table, the follower
// of type kind, after saving row as its live spawn_data row when it is set.
func bootFollower(t *testing.T, row *spawn.State, kind string) *frWorld {
	t.Helper()
	if row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{frRow: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fr.xml"), []byte(frSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	heard := &frHeard{}
	const memo = "test.FrMemo"
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":              defaultnpc.New,
		"script.ai.boss.frintezza.FollowerOfFrintezza": frintezza.FollowerOfFrintezza,
		"test." + frDirector:                           func() script.Script { return script.Script{} },
		memo: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{frFrintezzaID, frTransformID}, Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					key := "4"
					if e.NPC.NpcID() == frTransformID {
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
		{Path: "script.ai.boss.frintezza.FollowerOfFrintezza"},
		{Path: memo},
		{Path: "test." + frDirector},
	}
	templates := frTemplates(kind)
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	makers := &rb5Makers{}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(frSkills()),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers.catalog()),
	}}, templates...)
	return &frWorld{warriorWorld: w, draws: draws, heard: heard, makers: makers}
}

// follower returns the follower in the world.
func (w *frWorld) follower(t *testing.T) *npc.Hostile {
	t.Helper()
	live := w.liveNPCs(frFollowerID)
	if len(live) != 1 {
		t.Fatalf("followers in the world = %d, want 1", len(live))
	}
	return live[0]
}

// memo returns the object id the global memo holds under key, 0 for none.
func (w *frWorld) memo(t *testing.T, key string) int {
	t.Helper()
	var v int
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) { v = s.GlobalMemo().Int(key, 0) })
	return v
}

// send sends h script event eventID with arg1 from the director, on the
// character's queue.
func (w *frWorld) send(t *testing.T, h *npc.Hostile, eventID, arg1 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), eventID, arg1, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// hitTo lands a hit on h that leaves it at ratio of its maximum HP, then a
// hit of 1, whose attacked hook reads that ratio: the hook of a hit runs
// before its damage is taken.
func (w *frWorld) hitTo(t *testing.T, h *npc.Hostile, ratio float64) {
	t.Helper()
	damage := h.CurrentHP() - int(ratio*float64(h.MaxHP()))
	if damage <= 0 {
		t.Fatalf("setup: HP %d of %d is already at or below %v", h.CurrentHP(), h.MaxHP(), ratio)
	}
	w.hit(t, h, damage)
	w.hit(t, h, 1)
	w.srv.ReadQueued(t, w.srv.Client)
}

// frAt is where h stands and the way it faces.
func frAt(h *npc.Hostile) [4]int {
	x, y, z := h.Position()
	return [4]int{x, y, z, h.Heading()}
}

// frRowValue is the db value Shutdown saved in the follower's row.
func frRowValue(t *testing.T, srv *gameservertest.Server) int {
	t.Helper()
	states, err := gamesql.NewSpawnStore(srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	row, ok := states[frRow]
	if !ok {
		t.Fatalf("no spawn_data row %q", frRow)
	}
	return row.DBValue
}

// frLiveRow is a live spawn_data row of the follower at its spawn point
// holding value, its HP more than full.
func frLiveRow(value int) *spawn.State {
	return &spawn.State{Name: frRow, Status: spawn.StatusAlive, CurrentHP: 1 << 20, CurrentMP: 1 << 20, Location: location.Location{X: 200, Y: 20, Z: 30}, DBValue: value}
}

// TestFollowerOfFrintezzaSleepsThenRises runs the follower's created,
// script-event and timer hooks from a spawn with no row. Created, it claims
// memo key 5, sets its db value 0 and tells Frintezza (key 4) and its
// transform (key 6) so, its object id as the event id; it starts at
// i_ai3 5, running. Arg1 4 sets the db value 4 and leaves its dummy at the
// hall facing 16384; 6 s later it jumps there facing 16384, and 18 s later
// it is awake at 5, which Shutdown saves in its spawn_data row.
func TestFollowerOfFrintezzaSleepsThenRises(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, nil, "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	if got := w.memo(t, "5"); got != int(id) {
		t.Fatalf("memo 5 = %d, want the follower %d", got, id)
	}
	if n.DBLoaded() || n.DBValue() != 0 {
		t.Fatalf("follower loaded=%v value=%d, want not loaded at 0", n.DBLoaded(), n.DBValue())
	}
	want := []string{fmt.Sprintf("29993 event=%d arg1=0 arg2=0", id), fmt.Sprintf("29994 event=%d arg1=0 arg2=0", id)}
	if got := w.heard.take(); !slices.Equal(got, want) {
		t.Fatalf("heard as the follower appeared = %q, want %q", got, want)
	}
	if got := n.Int(script.SlotIntAI3); got != 5 || !f.Running() {
		t.Fatalf("follower i_ai3=%d running=%v, want 5 running", got, f.Running())
	}

	w.send(t, f, 1, 4)
	if got := n.DBValue(); got != 4 {
		t.Fatalf("db value after arg1 4 = %d, want 4", got)
	}
	dummies := w.liveNPCs(frDummyID)
	if len(dummies) != 1 || frAt(dummies[0]) != [4]int{174230, -88012, -5112, 16384} {
		t.Fatalf("dummies = %d, want one at the hall facing 16384", len(dummies))
	}
	w.srv.Advance(t, 5999*time.Millisecond)
	if x, _, _ := f.Position(); x != 200 {
		t.Fatalf("follower moved before 6 s: at x %d", x)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := frAt(f); got != [4]int{174230, -88012, -5112, 16384} {
		t.Fatalf("follower 6 s after arg1 4 at %v, want the hall facing 16384", got)
	}
	w.srv.Advance(t, 18*time.Second-time.Millisecond)
	if got := n.DBValue(); got != 4 {
		t.Fatalf("db value before it woke = %d, want 4", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := n.DBValue(); got != 5 {
		t.Fatalf("db value 24 s after arg1 4 = %d, want 5", got)
	}

	w.srv.Shutdown(t)
	if got := frRowValue(t, w.srv); got != 5 {
		t.Fatalf("saved db value = %d, want 5", got)
	}
}

// frShots are the nine camera shots of the follower's exit, after the
// object id: distance, yaw, pitch, time, duration, turn, rise, widescreen
// and the last field.
var frShots = [][]int32{
	{250, 180, 20, 0, 10000, 0, 6, 1, 1},
	{250, 180, 20, 0, 10000, 0, 6, 1, 1},
	{250, 180, 20, 0, 10000, 0, 6, 1, 1},
	{250, 180, 20, 0, 10000, 0, 6, 1, 1},
	{300, 220, 0, 3000, 10000, 0, 0, 1, 1},
	{100, 150, -3, 0, 10000, 0, 0, 1, 1},
	{100, 150, -3, 0, 10000, 0, 0, 1, 1},
	{100, 150, 10, 3000, 5000, 0, 15, 1, 1},
	{300, 180, 10, 2000, 2000, 0, 5, 1, 1},
}

// exitShots plays the follower's exit from its timer 2000, due in first,
// through its first count camera shots, each checked in order: 9.2 s,
// 9.3 s, 10.8 s, 10.9 s, 11.3 s, 14.3 s, 14.4 s, 14.8 s and 18.8 s after
// 2000, its transform called onto its place facing 16384 and told event 7
// at 9.7 s.
func (w *frWorld) exitShots(t *testing.T, f *npc.Hostile, first time.Duration, count int) {
	t.Helper()
	id := f.ObjectID()
	at := frAt(f)
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, first+9200*time.Millisecond-time.Millisecond)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames before the first shot = %x, want none", got)
	}
	gaps := []time.Duration{1, 100, 1500, 100, 400, 3000, 100, 400, 4000}
	for i, gap := range gaps[:count] {
		w.srv.Advance(t, gap*time.Millisecond)
		got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera)
		if want := ind8CameraPayload(id, frShots[i]...); len(got) != 1 || !slices.Equal(got[0], want) {
			t.Fatalf("camera frames at shot %d = %x, want %x", i+1, got, want)
		}
		if i == 2 {
			// 2100 ran 500 ms after the second shot.
			if got, want := w.heard.take(), []string{"29994 event=7 arg1=0 arg2=0"}; !slices.Equal(got, want) {
				t.Fatalf("heard as the transform was called = %q, want %q", got, want)
			}
			tr := w.liveNPCs(frTransformID)[0]
			if got := frAt(tr); got[0] != at[0] || got[1] != at[1] || got[3] != 16384 {
				t.Fatalf("transform at %v, want on the follower's %v facing 16384", got, at)
			}
		}
	}
}

// TestFollowerOfFrintezzaChangesBodyAndIsBeaten runs the attacked and timer
// hooks from a row restored at stage 5. Restored, it runs maker event 1001
// on both evil spirit makers. Hit below 60%, it drops its desires, wants
// its body change on itself at 1000000, goes to stage 6 and tells
// Frintezza 6; 2 s later it holds weapon 7903, and 3 s after that it takes
// hits again. Below 50% it tells Frintezza 30010; at 20% or less it sets
// its db value 50000, tells the NPCs within 6000 that it is beaten, and
// 10 s later plays its exit.
func TestFollowerOfFrintezzaChangesBodyAndIsBeaten(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(5), "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	if !n.DBLoaded() || n.DBValue() != 5 {
		t.Fatalf("follower loaded=%v value=%d, want loaded at 5", n.DBLoaded(), n.DBValue())
	}
	if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1001", "frintessa_evilate_maker2 1001"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as it was restored = %q, want %q", got, want)
	}
	w.heard.take()

	w.hitTo(t, f, 0.55)
	if got := n.DBValue(); got != 6 {
		t.Fatalf("db value after the hit below 60%% = %d, want 6", got)
	}
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=6 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard as it changed body = %q, want %q", got, want)
	}
	if got, want := ind8Casts(f), []string{fmt.Sprintf("5017>%d", id)}; !slices.Equal(got, want) {
		t.Fatalf("cast desires after the body change = %q, want %q", got, want)
	}
	for _, d := range ind8Desires(f, actorai.IntentionCast) {
		if d.Weight != 1000000 {
			t.Fatalf("body change weight = %v, want 1000000", d.Weight)
		}
	}
	if got := n.Int(script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after the body change = %d, want 1", got)
	}
	w.srv.Advance(t, 2*time.Second)
	if got := f.NPCInfoSnapshot().RightHand; got != 7903 {
		t.Fatalf("right hand 2 s after the body change = %d, want 7903", got)
	}
	w.srv.Advance(t, 3*time.Second)
	if got := n.Int(script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 5 s after the body change = %d, want 0", got)
	}

	w.hitTo(t, f, 0.45)
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=30010 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard below 50%% = %q, want %q", got, want)
	}
	w.hitTo(t, f, 0.15)
	if got := n.DBValue(); got != 50000 {
		t.Fatalf("db value at 15%% = %d, want 50000", got)
	}
	if got, want := w.heard.take(), []string{"29993 event=0 arg1=50000 arg2=0", "29994 event=0 arg1=50000 arg2=0"}; !slices.Equal(slices.Sorted(slices.Values(got)), want) {
		t.Fatalf("heard as it was beaten = %q, want %q", got, want)
	}
	w.exitShots(t, f, 10*time.Second, len(frShots))
	w.srv.Advance(t, 2000*time.Millisecond-time.Millisecond)
	if len(w.liveNPCs(frFollowerID)) != 1 {
		t.Fatal("the follower left before its last timer")
	}
	w.srv.Advance(t, time.Millisecond)
	if len(w.liveNPCs(frFollowerID)) != 0 {
		t.Fatal("the follower is still in the world after its exit")
	}
	if got := w.memo(t, "5"); got != 0 {
		t.Fatalf("memo 5 after the exit = %d, want none", got)
	}
	w.srv.Shutdown(t)
	if got := frRowValue(t, w.srv); got != 0 {
		t.Fatalf("saved db value after the exit = %d, want 0", got)
	}
}

// TestFollowerOfFrintezzaDies runs the dying hook, 3 s after the death: it
// tells Frintezza 30011, gives up memo key 5, sets its db value 0 and plays
// its exit from 0.5 s later. Its corpse decays 18 s after the death, which
// cancels the exit's timers past its seventh shot: nothing more is filmed.
func TestFollowerOfFrintezzaDies(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(6), "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	w.heard.take()
	w.kill(t, f)
	w.srv.Advance(t, 3*time.Second)
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=30011 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard as it died = %q, want %q", got, want)
	}
	if got := w.memo(t, "5"); got != 0 {
		t.Fatalf("memo 5 after its death = %d, want none", got)
	}
	if got := n.DBValue(); got != 0 {
		t.Fatalf("db value after its death = %d, want 0", got)
	}
	w.exitShots(t, f, 500*time.Millisecond, 7)
	// The harness runs no decay task: the corpse decays by hand at 18 s.
	w.srv.Advance(t, 100*time.Millisecond)
	w.onQueue(t, f, func() { f.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(f.ObjectID())) })
	w.srv.Advance(t, 10*time.Second)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames after the decay = %x, want none", got)
	}
}

// untouchedFor sets the game tick of h's last hit to secs before now: the
// game ticks are the server's real uptime.
func (w *frWorld) untouchedFor(t *testing.T, h *npc.Hostile, secs int32) {
	t.Helper()
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) {
		script.NPCOf(h).SetInt(npc.IntQuest1, s.CurrentTick()-secs)
	})
}

// TestFollowerOfFrintezzaFallsAsleep runs the 60 s timer 3000 from a row
// restored at stage 6: restored, it changes body again (Frintezza hears
// 6). At its timer 3000, every 60 s, more than 15 minutes of game ticks
// with no hit put it to sleep: db
// value 0, Frintezza and the transform told so, the NPCs within 8000 told
// 40000, the character in its territory sent out of the tomb, the follower
// at its sleeping point facing 16384 with weapon 8204, and maker event 1000
// on both evil spirit makers.
func TestFollowerOfFrintezzaFallsAsleep(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(6), "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1001", "frintessa_evilate_maker2 1001"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as it was restored = %q, want %q", got, want)
	}
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=6 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard as it was restored = %q, want %q", got, want)
	}
	if got := n.Int(script.SlotIntAI3); got != 5 {
		t.Fatalf("i_ai3 after its created hook = %d, want 5", got)
	}
	// Every draw 99: the look around of each timer 3000 does not make it
	// play with the NPCs it sees (a draw below 1 of 100).
	w.draws.set(99)
	w.untouchedFor(t, f, 800)
	w.srv.Advance(t, 60*time.Second)
	if got := n.DBValue(); got != 6 {
		t.Fatalf("db value at 3000 after 800 s untouched = %d, want 6", got)
	}
	w.untouchedFor(t, f, 1000)
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 60*time.Second)
	if got := n.DBValue(); got != 0 {
		t.Fatalf("db value at 3000 after 1000 s untouched = %d, want 0", got)
	}
	got := slices.Sorted(slices.Values(w.heard.take()))
	want := []string{
		"29993 event=0 arg1=40000 arg2=0", fmt.Sprintf("29993 event=%d arg1=0 arg2=0", id),
		"29994 event=0 arg1=40000 arg2=0", fmt.Sprintf("29994 event=%d arg1=0 arg2=0", id),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("heard as it fell asleep = %q, want %q", got, want)
	}
	if got := frAt(f); got != [4]int{-105200, -253104, -15264, 16384} {
		t.Fatalf("follower asleep at %v, want its sleeping point facing 16384", got)
	}
	if got := f.NPCInfoSnapshot().RightHand; got != 8204 {
		t.Fatalf("right hand asleep = %d, want 8204", got)
	}
	if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1000", "frintessa_evilate_maker2 1000"}; !slices.Equal(got, want) {
		t.Fatalf("maker events as it fell asleep = %q, want %q", got, want)
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x < 150037-150 || x > 150037+150 || y < -57255-150 || y > -57255+150 {
		t.Fatalf("character at %d,%d, want within 150 of 150037,-57255", x, y)
	}
}

// TestFollowerOfFrintezzaIsSentToSleep runs the script event arg1 0 on a
// follower restored at stage 5: db value 0, its scores cleared, at its
// sleeping point facing 16384, which Shutdown saves.
func TestFollowerOfFrintezzaIsSentToSleep(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(5), "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	w.hitTo(t, f, 0.9)
	if got := n.Int(npc.IntQuest2) + n.Int(npc.IntQuest3) + n.Int(npc.IntQuest4); got == 0 {
		t.Fatal("setup: the hit scored nobody")
	}
	w.send(t, f, 1, 0)
	if got := n.DBValue(); got != 0 {
		t.Fatalf("db value after arg1 0 = %d, want 0", got)
	}
	for _, slot := range []script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
		if got := n.Int(slot); got != 0 {
			t.Fatalf("score slot %d after arg1 0 = %d, want 0", slot, got)
		}
	}
	if got := frAt(f); got != [4]int{-105200, -253104, -15264, 16384} {
		t.Fatalf("follower after arg1 0 at %v, want its sleeping point facing 16384", got)
	}
	w.srv.Shutdown(t)
	if got := frRowValue(t, w.srv); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// moveCharacter puts the character dx, dy from h and runs one AI tick.
func (w *frWorld) moveCharacter(t *testing.T, h *npc.Hostile, dx, dy int) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x+dx, y+dy, z); err != nil {
		t.Fatalf("move the character: %v", err)
	}
	w.ticks(t, 1)
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestFollowerOfFrintezzaScoresWhatItSees runs the see-creature hook and
// the attack choice at stage 5. Seeing the character with no creature
// scored, a tie of the second and third slots goes to the second, a tie of
// the second and fourth too; the character scores 1000 plus a draw of
// 3000 (7) in it. The attack choice then keeps that score (a draw of 99 of
// 100), passes on the no-charge case (2000 of 10000) and, on 499 of 10000,
// wants the anti-gravity on the character at 10000. The follower is a plain
// monster here, which tells its see-creature hooks about a playable once as
// it comes in sight.
func TestFollowerOfFrintezzaScoresWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(5), "Monster")
	f := w.follower(t)
	n := script.NPCOf(f)
	w.moveCharacter(t, f, 3000, 0)
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	for _, slot := range []script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
		n.SetInt(slot, 0)
	}
	for _, slot := range []script.CreatureSlot{npc.CreatureQuest2, npc.CreatureQuest3, npc.CreatureQuest4} {
		n.SetSlot(slot, nil)
	}
	w.draws.set(0, 0, 0, 7, 99, 2000, 499)
	w.moveCharacter(t, f, -100, 0)
	if got := n.Int(npc.IntQuest2); got != 1007 {
		t.Fatalf("second score = %d, want 1007", got)
	}
	if c := n.Slot(npc.CreatureQuest2); c == nil || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("second slot = %v, want the character", c)
	}
	if got, want := ind8Casts(f), []string{fmt.Sprintf("5016>%d", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("cast desires = %q, want %q", got, want)
	}
	for _, d := range ind8Desires(f, actorai.IntentionCast) {
		if d.Weight != 10000 {
			t.Fatalf("anti-gravity weight = %v, want 10000", d.Weight)
		}
	}
}
