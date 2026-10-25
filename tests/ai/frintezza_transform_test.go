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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The follower's transform (#198 PR 2) and a listener NPC that claims the
// follower's global memo key (5); frFrintezzaID claims Frintezza's (4).
const (
	trTransformID = int32(29047)
	trFollowerID  = int32(29995)
	trRow         = "follower_of_frintessa_tr"
)

// trSpawnlist places the listeners first, then the transform under the
// shipped database maker, and declares the two evil spirit npcmakers as
// probes. The tomb holds the character.
const trSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fr_tomb" minZ="0" maxZ="100"><node x="-400" y="-400"/><node x="400" y="-400"/><node x="400" y="400"/><node x="-400" y="400"/></territory>
	<npcmaker name="fr_memo" territory="fr_tomb" maximumNpcs="2">
		<npc id="29993" total="1" pos="-200;20;30;0" respawn="0sec"/>
		<npc id="29995" total="1" pos="-200;120;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="frintessa_maker" territory="fr_tomb" maximumNpcs="9">
		<ai type="default_use_db_maker_for_frintessa"/>
		<npc id="29047" pos="200;20;30;0" total="1" respawn="48hour" dbName="follower_of_frintessa_tr"/>
	</npcmaker>
	<npcmaker name="frintessa_evilate_maker1" territory="fr_tomb" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
	<npcmaker name="frintessa_evilate_maker2" territory="fr_tomb" maximumNpcs="1">
		<ai type="rb5_probe"/>
	</npcmaker>
</list>`

// bootTransform boots the tomb backed by the spawn_data table with the
// transform of type kind, after saving row as its live spawn_data row when
// it is set, with extra's options and step before the character enters.
func bootTransform(t *testing.T, row *spawn.State, kind string, extra raidBoot) *frWorld {
	t.Helper()
	if row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{trRow: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tr.xml"), []byte(trSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	heard := &frHeard{}
	const memo = "test.TrMemo"
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                       defaultnpc.New,
		"script.ai.boss.frintezza.FollowerOfFrintezzaTransform": frintezza.FollowerOfFrintezzaTransform,
		"test." + frDirector:                                    func() script.Script { return script.Script{} },
		memo: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{frFrintezzaID, trFollowerID}, Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					key := "4"
					if e.NPC.NpcID() == trFollowerID {
						key = "5"
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
		{Path: "script.ai.boss.frintezza.FollowerOfFrintezzaTransform"},
		{Path: memo},
		{Path: "test." + frDirector},
	}
	transform := warriorTemplate(trTransformID, nil)
	transform.Type, transform.CorpseTime, transform.AggroRange = kind, 18, 1000
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range []*npc.Template{transform, warriorTemplate(frFrintezzaID, nil), warriorTemplate(trFollowerID, nil)} {
		tmpl.CanMove = false
		templates = append(templates, tmpl)
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	skill := func(id modelskill.ID, level int) modelskill.Definition {
		d := ind1Skill(id, 600, false)
		d.Level = level
		return d
	}
	skills := modelskill.NewTable([]modelskill.Definition{
		skill(4258, 1), skill(5014, 1), skill(5015, 3), skill(5015, 6), skill(5016, 1), skill(5018, 2), skill(5019, 1),
	})
	makers := &rb5Makers{}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{ready: extra.ready, opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(skills),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers.catalog()),
	}, extra.opts...)}, templates...)
	return &frWorld{warriorWorld: w, draws: draws, heard: heard, makers: makers}
}

// transform returns the transform in the world.
func (w *frWorld) transform(t *testing.T) *npc.Hostile {
	t.Helper()
	live := w.liveNPCs(trTransformID)
	if len(live) != 1 {
		t.Fatalf("transforms in the world = %d, want 1", len(live))
	}
	return live[0]
}

// trLiveRow is a live spawn_data row of the transform at its spawn point
// holding value, with hp HP (more than full by default).
func trLiveRow(value, hp int) *spawn.State {
	return &spawn.State{Name: trRow, Status: spawn.StatusAlive, CurrentHP: hp, CurrentMP: 1 << 20, Location: location.Location{X: 200, Y: 20, Z: 30}, DBValue: value}
}

// trRowValue is the db value Shutdown saved in the transform's row.
func trRowValue(t *testing.T, srv *gameservertest.Server) int {
	t.Helper()
	states, err := gamesql.NewSpawnStore(srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	row, ok := states[trRow]
	if !ok {
		t.Fatalf("no spawn_data row %q", trRow)
	}
	return row.DBValue
}

// trAsleep is the transform's sleeping point, facing 16384.
var trAsleep = [4]int{-105200, -253104, -15264, 16384}

// TestFrintezzaTransformSpawnsAsleep runs the created hook from a spawn
// with no row: it claims memo key 6, sets its db value 0 and tells
// Frintezza (key 4) so, its object id as the event id, then sets 5 and
// tells the transform (key 6, itself), which sends it to sleep: db value 0
// at its sleeping point facing 16384, which Shutdown saves.
func TestFrintezzaTransformSpawnsAsleep(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, nil, "GrandBoss", raidBoot{})
	f := w.transform(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	if got := w.memo(t, "6"); got != int(id) {
		t.Fatalf("memo 6 = %d, want the transform %d", got, id)
	}
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=0 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard as the transform appeared = %q, want %q", got, want)
	}
	if n.DBLoaded() || n.DBValue() != 0 {
		t.Fatalf("transform loaded=%v value=%d, want not loaded at 0", n.DBLoaded(), n.DBValue())
	}
	if got := frAt(f); got != trAsleep {
		t.Fatalf("transform at %v, want its sleeping point %v", got, trAsleep)
	}
	w.srv.Shutdown(t)
	if got := trRowValue(t, w.srv); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestFrintezzaTransformRisesAndFallsAsleep runs the created and timer
// hooks from a row restored at stage 7, every draw 99. Restored at full HP
// it tells Frintezza 7 (30021 below 75% of its HP); 2 s later 7 again; 6 s
// after that it wants social 2 at 10000000 (i_ai0 30020); 7 s after the
// pose it runs maker event 1000 on both evil spirit makers and tells the
// NPCs within 8000 arg1 20000, fighting at i_ai0 7 and i_ai4 1. At its
// timer 3000, every 60 s, more than 15 minutes of game ticks with no hit
// put it to sleep: db value 0, Frintezza and the follower (key 5) told so,
// the character in its territory sent out of the tomb, the transform at
// its sleeping point facing 16384.
func TestFrintezzaTransformRisesAndFallsAsleep(t *testing.T) {
	t.Parallel()
	t.Run("low HP", func(t *testing.T) {
		t.Parallel()
		w := bootTransform(t, trLiveRow(7, 1), "GrandBoss", raidBoot{})
		f := w.transform(t)
		if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=30021 arg2=0", f.ObjectID())}; !slices.Equal(got, want) {
			t.Fatalf("heard as it was restored at 1 HP = %q, want %q", got, want)
		}
	})
	t.Run("full HP", func(t *testing.T) {
		t.Parallel()
		w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
		f := w.transform(t)
		n := script.NPCOf(f)
		id := f.ObjectID()
		w.draws.set(99)
		if !n.DBLoaded() || n.DBValue() != 7 {
			t.Fatalf("transform loaded=%v value=%d, want loaded at 7", n.DBLoaded(), n.DBValue())
		}
		told7 := []string{fmt.Sprintf("29993 event=%d arg1=7 arg2=0", id)}
		if got := w.heard.take(); !slices.Equal(got, told7) {
			t.Fatalf("heard as it was restored = %q, want %q", got, told7)
		}
		if got := n.Int(script.SlotIntAI4); got != 0 {
			t.Fatalf("i_ai4 after its created hook = %d, want 0", got)
		}
		w.srv.Advance(t, 2*time.Second-time.Millisecond)
		if got := w.heard.take(); len(got) != 0 {
			t.Fatalf("heard before 2 s = %q, want nothing", got)
		}
		w.srv.Advance(t, time.Millisecond)
		if got := w.heard.take(); !slices.Equal(got, told7) {
			t.Fatalf("heard at 2 s = %q, want %q", got, told7)
		}
		w.srv.Advance(t, 6*time.Second-time.Millisecond)
		if got := n.Int(script.SlotIntAI0); got != 0 {
			t.Fatalf("i_ai0 before the pose = %d, want 0", got)
		}
		w.srv.Advance(t, time.Millisecond)
		if got := n.Int(script.SlotIntAI0); got != 30020 {
			t.Fatalf("i_ai0 at the pose = %d, want 30020", got)
		}
		w.srv.Advance(t, 7*time.Second-time.Millisecond)
		if got := w.makers.take(); len(got) != 0 {
			t.Fatalf("maker events before 15 s = %q, want none", got)
		}
		w.srv.Advance(t, time.Millisecond)
		if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1000", "frintessa_evilate_maker2 1000"}; !slices.Equal(got, want) {
			t.Fatalf("maker events at 15 s = %q, want %q", got, want)
		}
		if got, want := slices.Sorted(slices.Values(w.heard.take())), []string{"29993 event=0 arg1=20000 arg2=0", "29995 event=0 arg1=20000 arg2=0"}; !slices.Equal(got, want) {
			t.Fatalf("heard at 15 s = %q, want %q", got, want)
		}
		if ai0, ai4 := n.Int(script.SlotIntAI0), n.Int(script.SlotIntAI4); ai0 != 7 || ai4 != 1 {
			t.Fatalf("i_ai0=%d i_ai4=%d at 15 s, want 7 and 1", ai0, ai4)
		}

		w.untouchedFor(t, f, 800)
		w.srv.Advance(t, 60*time.Second)
		if got := n.DBValue(); got != 7 {
			t.Fatalf("db value at 3000 after 800 s untouched = %d, want 7", got)
		}
		w.untouchedFor(t, f, 1000)
		w.srv.ReadQueued(t, w.srv.Client)
		w.heard.take()
		w.srv.Advance(t, 60*time.Second)
		if got := n.DBValue(); got != 0 {
			t.Fatalf("db value at 3000 after 1000 s untouched = %d, want 0", got)
		}
		want := []string{fmt.Sprintf("29993 event=%d arg1=0 arg2=0", id), fmt.Sprintf("29995 event=%d arg1=0 arg2=0", id)}
		if got := slices.Sorted(slices.Values(w.heard.take())); !slices.Equal(got, want) {
			t.Fatalf("heard as it fell asleep = %q, want %q", got, want)
		}
		if got := frAt(f); got != trAsleep {
			t.Fatalf("transform asleep at %v, want %v", got, trAsleep)
		}
		if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x < 150037-150 || x > 150037+150 || y < -57255-150 || y > -57255+150 {
			t.Fatalf("character at %d,%d, want within 150 of 150037,-57255", x, y)
		}
		w.srv.Shutdown(t)
		if got := trRowValue(t, w.srv); got != 0 {
			t.Fatalf("saved db value = %d, want 0", got)
		}
	})
}

// TestFrintezzaTransformAnswersTheFollower runs the script event hook on a
// transform restored at stage 7, past its own start: event 7 tells
// Frintezza 7 2 s later, once (the second 1001 timer, due at 7 s, is the
// same timer and never starts); arg1 0 sends it to sleep: db value 0, its
// scores cleared, at its sleeping point facing 16384.
func TestFrintezzaTransformAnswersTheFollower(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
	f := w.transform(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	w.draws.set(99)
	w.srv.Advance(t, 20*time.Second)
	w.heard.take()
	w.send(t, f, 7, 1)
	w.srv.Advance(t, 2*time.Second)
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=7 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard 2 s after event 7 = %q, want %q", got, want)
	}
	w.srv.Advance(t, 6*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard 8 s after event 7 = %q, want nothing", got)
	}

	w.hit(t, f, 1)
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
	if got := frAt(f); got != trAsleep {
		t.Fatalf("transform after arg1 0 at %v, want %v", got, trAsleep)
	}
}

// trShots are the three camera shots of the transform's death, after the
// object id: distance, yaw, pitch, time, duration, turn, rise, widescreen
// and the last field.
var trShots = [][]int32{
	{430, 300, 80, 0, 10000, 0, 0, 1, 1},
	{430, 300, 80, 0, 10000, 0, 0, 1, 1},
	{0, 180, 80, 4000, 6000, 0, 0, 1, 1},
}

// TestFrintezzaTransformDiesOnCamera runs the dying hook, 3 s after the
// death of a transform restored at stage 7, past its own start: it gives up memo key 6, tells the NPCs within 8000 event 8 (its
// object id as the event id) and films its death: a shot 1 ms later,
// BS05_D bound to it then the same shot 100 ms after that, and a slow
// shot 400 ms after the second.
func TestFrintezzaTransformDiesOnCamera(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
	f := w.transform(t)
	id := f.ObjectID()
	w.draws.set(99)
	w.srv.Advance(t, 20*time.Second)
	w.heard.take()
	w.kill(t, f)
	w.srv.Advance(t, 3*time.Second-time.Millisecond)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard before the dying hook = %q, want nothing", got)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, time.Millisecond)
	want := []string{fmt.Sprintf("29993 event=%d arg1=8 arg2=0", id), fmt.Sprintf("29995 event=%d arg1=8 arg2=0", id)}
	if got := slices.Sorted(slices.Values(w.heard.take())); !slices.Equal(got, want) {
		t.Fatalf("heard as it died = %q, want %q", got, want)
	}
	if got := w.memo(t, "6"); got != 0 {
		t.Fatalf("memo 6 after its death = %d, want none", got)
	}
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames as it died = %x, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 1 || !slices.Equal(got[0], ind8CameraPayload(id, trShots[0]...)) {
		t.Fatalf("camera frames 1 ms after the dying hook = %x, want the first shot", got)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	var got [][]byte
	for _, fr := range w.srv.ReadQueued(t, w.srv.Client) {
		if fr[0] == serverpackets.OpcodePlaySound || fr[0] == serverpackets.OpcodeSpecialCamera {
			got = append(got, fr)
		}
	}
	if want := [][]byte{ind8MusicPayload(id, "BS05_D", ind8At(f)), ind8CameraPayload(id, trShots[1]...)}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("frames 101 ms after the dying hook = %x, want BS05_D then the second shot", got)
	}
	w.srv.Advance(t, 400*time.Millisecond-time.Millisecond)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames before the third shot = %x, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 1 || !slices.Equal(got[0], ind8CameraPayload(id, trShots[2]...)) {
		t.Fatalf("camera frames 501 ms after the dying hook = %x, want the third shot", got)
	}
	w.srv.Advance(t, 10*time.Second)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames after the third shot = %x, want none", got)
	}
}

// TestFrintezzaTransformFights runs the attacked hook and the attack
// choice on a transform restored at stage 7, past its own start. Every hit
// tells Frintezza how it fares, read before the hit's damage: 30024 at 75%
// of its HP or more, 30021 below. With the character its only scored
// creature (5000, which a hit of 1 does not top), the attack choice keeps
// the score (99 of 100), passes the no-charge case (2000 of 10000) and
// draws its band's odds in turn: above 75% the anti-gravity on 999 of
// 1000; between 50% and 75% the magic circle 5018 level 2 on 499 of 500
// past the dash (1000) and the anti-gravity (750); between 10% and 25%
// the vampiric touch on 999 of 1000 past the dash, the anti-gravity and
// the magic circle (1000 each). A hit by 5008 level 2 starts the dash
// chain: the first dash (5015 level 3) wanted at the attacker, checked,
// then the chain's second unchecked, at i_ai2 3.
func TestFrintezzaTransformFights(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
	f := w.transform(t)
	n := script.NPCOf(f)
	id := f.ObjectID()
	me := w.player.ObjectID()
	w.draws.set(99)
	w.srv.Advance(t, 20*time.Second)
	w.heard.take()

	choose := func(want string, draws ...int) {
		t.Helper()
		w.onQueue(t, f, func() { f.AI().Desires().Clear() })
		w.scoreMe(t, f, 5000)
		w.draws.set(99, draws...)
		w.hit(t, f, 1)
		if got := frCastDesires(f); !slices.Equal(got, []string{want}) {
			t.Fatalf("cast desires = %q, want %q", got, want)
		}
	}
	choose(fmt.Sprintf("5016-1>%d=10000", me), 99, 2000, 1000, 999)
	if got, want := w.heard.take(), []string{fmt.Sprintf("29993 event=%d arg1=30024 arg2=0", id)}; !slices.Equal(got, want) {
		t.Fatalf("heard at full HP = %q, want %q", got, want)
	}
	if got := n.Int(script.SlotIntAI4); got != 30024 {
		t.Fatalf("i_ai4 at full HP = %d, want 30024", got)
	}

	w.hitTo(t, f, 0.7)
	want := []string{fmt.Sprintf("29993 event=%d arg1=30024 arg2=0", id), fmt.Sprintf("29993 event=%d arg1=30021 arg2=0", id)}
	if got := w.heard.take(); !slices.Equal(got, want) {
		t.Fatalf("heard on the hits down to 70%% = %q, want %q", got, want)
	}
	choose(fmt.Sprintf("5018-2>%d=10000", me), 99, 2000, 1000, 750, 499)

	w.hitTo(t, f, 0.2)
	choose(fmt.Sprintf("5019-1>%d=10000", me), 99, 2000, 1000, 1000, 1000, 999)

	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.scoreMe(t, f, 5000)
	w.draws.set(99)
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, me).Post(func() {
		defer close(done)
		f.ReduceHP(1, w.player, modelskill.Definition{ID: 5008, Level: 2})
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	if got := n.Int(script.SlotIntAI2); got != 3 {
		t.Fatalf("i_ai2 after the spirit call hit = %d, want 3", got)
	}
	if got := frCastDesires(f); len(got) != 1 || got[0][:len("5015-3>")] != "5015-3>" {
		t.Fatalf("cast desires after the spirit call hit = %q, want the dash at the character", got)
	}
}

// TestFrintezzaTransformHindersARider runs the attacked hook's strider
// check, every draw 99: a hit by the character riding a strider, holding
// no Hinder Strider, makes the transform want Hinder Strider (4258 level
// 1) on it at 1000000.
func TestFrintezzaTransformHindersARider(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
	f := w.transform(t)
	w.draws.set(99)
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	rider, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T", obj)
	}
	done := make(chan struct{})
	w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		if !rider.Mount(12526, 1) {
			t.Error("the character did not mount")
		}
	})
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if !script.Riding(w.me(t)) {
		t.Fatal("setup: the character is not riding")
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.hit(t, f, 1)
	if got, want := frCastDesires(f), []string{fmt.Sprintf("4258-1>%d=1e+06", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("cast desires after the rider's hit = %q, want %q", got, want)
	}
}

// TestFrintezzaTransformScoresWhatItSees runs the see-creature hook on a
// transform restored at stage 7, a plain monster here, which tells its
// see-creature hooks about a playable once as it comes in sight. At i_ai4
// 0 it scores nothing. Fighting (i_ai4 1), with no creature scored, a tie
// of the second and third slots goes to the second, a tie of the second
// and fourth too; the character scores 1000 plus a draw of 3000 (7) in it,
// and the attack choice at full HP wants the anti-gravity on it (99 of
// 100, 2000, 1000, then 999 of 1000).
func TestFrintezzaTransformScoresWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "Monster", raidBoot{})
	f := w.transform(t)
	n := script.NPCOf(f)
	w.draws.set(99)
	reset := func(ai4 int32) {
		w.moveCharacter(t, f, 3000, 0)
		w.onQueue(t, f, func() {
			f.AI().Desires().Clear()
			n.SetInt(script.SlotIntAI4, ai4)
			for _, slot := range []script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
				n.SetInt(slot, 0)
			}
			for _, slot := range []script.CreatureSlot{npc.CreatureQuest2, npc.CreatureQuest3, npc.CreatureQuest4} {
				n.SetSlot(slot, nil)
			}
		})
	}
	reset(0)
	w.moveCharacter(t, f, -100, 0)
	if c := n.Slot(npc.CreatureQuest2); c != nil {
		t.Fatalf("second slot at i_ai4 0 = %v, want nobody", c)
	}

	reset(1)
	w.draws.set(0, 0, 0, 7, 99, 2000, 1000, 999)
	w.moveCharacter(t, f, -100, 0)
	if got := n.Int(npc.IntQuest2); got != 1007 {
		t.Fatalf("second score = %d, want 1007", got)
	}
	if c := n.Slot(npc.CreatureQuest2); c == nil || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("second slot = %v, want the character", c)
	}
	if got, want := frCastDesires(f), []string{fmt.Sprintf("5016-1>%d=10000", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("cast desires = %q, want %q", got, want)
	}
}

// TestFrintezzaTransformLooksAroundWithNothingToDo runs the no-desire hook
// on a transform restored at stage 7, fighting from 15 s, every draw 0
// past 45 s (a look around takes at most one every 30 s). AI ticks with a
// desire queued see nothing new; with nothing to do, the ticks' no-desire
// hook looks around, and the character it sees takes the second slot,
// scoring 1000 plus a draw of 0, which the attack choice's draw of 0 of
// 100 then drops to 500.
func TestFrintezzaTransformLooksAroundWithNothingToDo(t *testing.T) {
	t.Parallel()
	w := bootTransform(t, trLiveRow(7, 1<<20), "GrandBoss", raidBoot{})
	f := w.transform(t)
	n := script.NPCOf(f)
	w.draws.set(99)
	w.onQueue(t, f, func() { n.AddDoNothingDesire(60, 50) })
	w.srv.Advance(t, 45*time.Second)
	w.scoreMe(t, f, 0)
	w.onQueue(t, f, func() { n.SetSlot(npc.CreatureQuest2, nil) })
	w.draws.set(0)
	w.ticks(t, 5)
	for _, slot := range []script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
		if got := n.Int(slot); got != 0 {
			t.Fatalf("score slot %d after ticks with a desire queued = %d, want 0", slot, got)
		}
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.ticks(t, 5)
	if got := n.Int(npc.IntQuest2); got != 500 {
		t.Fatalf("second score after ticks with nothing to do = %d, want 500", got)
	}
	if c := n.Slot(npc.CreatureQuest2); c == nil || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("second slot = %v, want the character", c)
	}
}
