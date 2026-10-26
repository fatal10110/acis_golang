package ai

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
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/zaken"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Zaken and the four privates its waves place.
const (
	zakenID            = int32(29022)
	zakenDollBladerID  = int32(29023)
	zakenValeMasterID  = int32(29024)
	zakenCaptainID     = int32(29026)
	zakenPirateID      = int32(29027)
	zakenRow           = "zaken"
	zakenDirector      = "ZakenDirector"
	zakenNightToDay    = modelskill.ID(4223)
	zakenRegeneration  = modelskill.ID(4227)
	zakenInstantMove   = modelskill.ID(4222)
	zakenScatterEnemy  = modelskill.ID(4216)
	zakenMassTeleport  = modelskill.ID(4217)
	zakenAggroBuffID   = modelskill.ID(4999)
	zakenSkillHitTimes = 500
)

// zakenHome is where Zaken's maker places it, on the middle deck of its
// ship; the character enters the world 200 west of it.
var (
	zakenHome      = location.Location{X: 55312, Y: 219168, Z: -3223}
	zakenCharacter = location.Location{X: 55112, Y: 219168, Z: -3223}
)

// zakenSpawnlist is one territory over Zaken's whole ship and its
// database-tracked maker, placing it at zakenHome.
const zakenSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="zaken_ship" minZ="-3600" maxZ="-2800">
		<node x="52500" y="216800"/>
		<node x="57800" y="216800"/>
		<node x="57800" y="221300"/>
		<node x="52500" y="221300"/>
	</territory>
	<npcmaker name="zaken_ship" territory="zaken_ship" maximumNpcs="200">
		<ai type="default_maker"/>
		<npc id="29022" pos="55312;219168;-3223;0" total="1" respawn="60hour" respawnRand="0sec" dbName="zaken" dbSaving="DEATH_TIME;PARAMETERS;POS"/>
	</npcmaker>
</list>`

// zakenSkills are the skills Zaken casts, as one-target fixture buffs of
// reach 600 it can always pay for; with forms, also its day and night
// forms and its regenerations, which otherwise it has no data for, so
// timer 1001 queues none of them.
func zakenSkills(forms bool) []modelskill.Definition {
	ids := []modelskill.ID{4216, 4217, 4218, 4219, 4220, 4221, 4222, 4258}
	if forms {
		ids = append(ids, 4223, 4224, 4227, 4242)
	}
	var defs []modelskill.Definition
	for _, id := range ids {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 600, HitTime: zakenSkillHitTimes, StaticHitTime: true, StaticReuse: true,
		})
	}
	return defs
}

// zakenWorld is a booted Zaken ship: the character, the in-game clock and
// the script draws.
type zakenWorld struct {
	*baiumWorld
	draws *ind1Draws
}

// zakenBoot is what a Zaken test boots with: night pins the in-game hour
// at 01:00 (otherwise 12:00), forms gives Zaken its form skills
// (zakenSkills), row is Zaken's live spawn_data row (none
// when nil), extra are boot options and ready runs before the character
// enters the world.
type zakenBoot struct {
	night bool
	forms bool
	row   *spawn.State
	extra []gameservertest.Option
	ready func(*gameservertest.Server, int32)
}

// bootZaken boots Zaken's ship backed by the spawn_data table, every script
// draw the highest its bound allows until a test queues others, Zaken and
// its privates parked, and the character entering the world at
// zakenCharacter.
func bootZaken(t *testing.T, b zakenBoot) *zakenWorld {
	t.Helper()
	if b.row != nil {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{zakenRow: b.row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := midnight.Add(2 * time.Hour)
	if b.night {
		start = midnight.Add(10 * time.Minute)
	}
	w := &zakenWorld{baiumWorld: &baiumWorld{start: start}, draws: &ind1Draws{fallback: 1 << 30}}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zaken.xml"), []byte(zakenSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := warriorTemplate(zakenID, nil)
	boss.Type, boss.Name, boss.Level, boss.CanMove = "GrandBoss", "Zaken", 60, false
	boss.AggroRange, boss.CanSeeThrough = 1000, true
	templates := []*npc.Template{boss}
	kinds := map[int32]script.NPCKind{zakenID: script.KindHostile}
	for _, id := range []int32{zakenDollBladerID, zakenValeMasterID, zakenCaptainID, zakenPirateID} {
		pvt := warriorTemplate(id, nil)
		pvt.CanMove = false
		templates = append(templates, pvt)
		kinds[id] = script.KindHostile
	}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.boss.zaken.Zaken"}, {Path: "test." + zakenDirector}}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc": defaultnpc.New,
		"script.ai.boss.zaken.Zaken":      zaken.Zaken,
		"test." + zakenDirector:           func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{zakenCharacter}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(zakenSkills(b.forms))),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(w.draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
	}, b.extra...)...)
	if b.ready != nil {
		b.ready(srv, srv.SoleObjectID(t))
	}
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	w.warriorWorld = &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
	w.appear(t)
	return w
}

// zaken returns the one Zaken in the world.
func (w *zakenWorld) zaken(t *testing.T) *npc.Hostile {
	t.Helper()
	got := w.liveNPCs(zakenID)
	if len(got) != 1 {
		t.Fatalf("Zakens in the world = %d, want 1", len(got))
	}
	return got[0]
}

// zakenSlot reads one integer of h's script memory.
func zakenSlot(h *npc.Hostile, slot script.IntSlot) int32 { return script.NPCOf(h).Int(slot) }

// zakenDest is where Instant Move takes h: its slots 1 to 3.
func zakenDest(h *npc.Hostile) location.Location {
	return location.Location{X: int(zakenSlot(h, script.SlotIntAI1)), Y: int(zakenSlot(h, script.SlotIntAI2)), Z: int(zakenSlot(h, script.SlotIntAI3))}
}

// privateCounts counts the live privates of each id in the world.
func (w *zakenWorld) privateCounts() map[int32]int {
	out := map[int32]int{}
	for _, id := range []int32{zakenDollBladerID, zakenValeMasterID, zakenCaptainID, zakenPirateID} {
		if n := len(w.liveNPCs(id)); n > 0 {
			out[id] = n
		}
	}
	return out
}

// wantCounts fails unless the live privates of each id number as want.
func (w *zakenWorld) wantCounts(t *testing.T, when string, want map[int32]int) {
	t.Helper()
	got := w.privateCounts()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s: privates = %v, want %v", when, got, want)
	}
}

// TestZakenAppearsAndPlacesItsWaves covers Zaken's created hook and timer
// 1003 for a Zaken whose spawn_data row is not live: it records where it
// stands as its Instant Move point, may escape 3 HP quarters, and, standing
// inside its territory, places a wave of privates 1.7 s after it appeared
// and every 1.7 s after that, seven waves in all. Every draw is the
// highest: each room point is its room's corner plus 649 on X and Y, each
// private faces 359. Wave 1 places a pirate captain in each of the 15
// rooms, wave 2 a doll blader, wave 3 two vale masters, wave 4 five
// pirates; waves 5 to 7 place 28, 28 and 29 privates at fixed points. No
// wave follows the seventh.
func TestZakenAppearsAndPlacesItsWaves(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	if got := zakenDest(h); got != zakenHome {
		t.Errorf("Instant Move point = %v, want where Zaken appeared, %v", got, zakenHome)
	}
	if got := zakenSlot(h, npc.IntQuest2); got != 3 {
		t.Errorf("HP quarters = %d, want 3", got)
	}
	if got := zakenSlot(h, script.SlotIntAI4); got != 1 {
		t.Errorf("next wave = %d, want 1", got)
	}
	w.wantCounts(t, "as Zaken appears", map[int32]int{})

	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "wave 1", map[int32]int{zakenCaptainID: 15})
	var got []string
	for _, p := range w.liveNPCs(zakenCaptainID) {
		at := positionOf(p)
		got = append(got, fmt.Sprintf("%d,%d heading %d", at.X, at.Y, p.Heading()))
	}
	slices.Sort(got)
	// The rooms' corners, five on each deck, bottom deck first.
	corners := [15][2]int{
		{53950, 219860},
		{55980, 219820},
		{54950, 218790},
		{55970, 217770},
		{53930, 217760},
		{55970, 217770},
		{55980, 219920},
		{54960, 218790},
		{53950, 219860},
		{53930, 217760},
		{55970, 217770},
		{55980, 219920},
		{54960, 218790},
		{53950, 219860},
		{53930, 217760},
	}
	var want []string
	for _, c := range corners {
		want = append(want, fmt.Sprintf("%d,%d heading 359", c[0]+649, c[1]+649))
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("wave 1 points =\n%q\nwant\n%q", got, want)
	}

	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "wave 2", map[int32]int{zakenCaptainID: 15, zakenDollBladerID: 15})
	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "wave 3", map[int32]int{zakenCaptainID: 15, zakenDollBladerID: 15, zakenValeMasterID: 30})
	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "wave 4", map[int32]int{zakenCaptainID: 15, zakenDollBladerID: 15, zakenValeMasterID: 30, zakenPirateID: 75})
	w.advance(t, 3*1700*time.Millisecond)
	w.wantCounts(t, "wave 7", map[int32]int{zakenCaptainID: 36, zakenDollBladerID: 34, zakenValeMasterID: 47, zakenPirateID: 103})
	if got := zakenSlot(h, script.SlotIntAI4); got != 8 {
		t.Errorf("next wave after the seventh = %d, want 8", got)
	}
	if got := len(h.Minions()); got != 220 {
		t.Errorf("Zaken's privates = %d, want 220", got)
	}
	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "1.7 s after wave 7", map[int32]int{zakenCaptainID: 36, zakenDollBladerID: 34, zakenValeMasterID: 47, zakenPirateID: 103})
}

// TestZakenRestoresOutsideItsTerritory covers the created hook's other
// branches: a Zaken placed from its live spawn_data row, here outside its
// territory, keeps no escapable HP quarter and places no wave.
func TestZakenRestoresOutsideItsTerritory(t *testing.T) {
	t.Parallel()
	outside := location.Location{X: 50000, Y: 219168, Z: -3223}
	w := bootZaken(t, zakenBoot{row: &spawn.State{Name: zakenRow, Status: spawn.StatusAlive, CurrentHP: 1000, CurrentMP: 1000, Location: outside}})
	h := w.zaken(t)
	if got := positionOf(h); got.X != outside.X || got.Y != outside.Y {
		t.Fatalf("setup: Zaken at %v, want restored at %v", got, outside)
	}
	if got := zakenSlot(h, npc.IntQuest2); got != 0 {
		t.Errorf("HP quarters = %d, want 0", got)
	}
	if got := zakenSlot(h, script.SlotIntAI4); got != 0 {
		t.Errorf("next wave = %d, want 0", got)
	}
	w.advance(t, 1700*time.Millisecond)
	w.wantCounts(t, "1.7 s after Zaken appeared outside its territory", map[int32]int{})
}

// wantsOrCast reports whether h wants skill id, or cast it in frames.
func wantsOrCast(frames [][]byte, h *npc.Hostile, id modelskill.ID) bool {
	_, ok := castDesireOf(h, id)
	return ok || len(castsOf(frames, h, id)) > 0
}

// settle lets every wave be placed and timer 1001's first check run, which
// draw, so the draws a test queues next reach the hooks it drives (until
// the next check, 30 s later).
func (w *zakenWorld) settle(t *testing.T) {
	t.Helper()
	w.advance(t, 7*1700*time.Millisecond)
}

// until runs AI ticks, each followed by 600 ms, until cond holds, and
// returns the frames sent meanwhile; it fails after 8 rounds.
func (w *zakenWorld) until(t *testing.T, what string, cond func() bool) [][]byte {
	t.Helper()
	var frames [][]byte
	for i := 0; !cond(); i++ {
		if i == 8 {
			t.Fatalf("%s: not after 8 AI ticks", what)
		}
		frames = append(frames, w.ticks(t, 1)...)
		frames = append(frames, w.advance(t, 600*time.Millisecond)...)
	}
	return frames
}

// zakenCast makes h want skill id on target, then runs AI ticks until done
// holds.
func (w *zakenWorld) zakenCast(t *testing.T, h *npc.Hostile, id modelskill.ID, target attackable.Combatant, done func() bool) {
	t.Helper()
	w.onQueue(t, h, func() {
		script.NPCOf(h).AddCastDesire(script.PlayerOf(target), modelskill.Ref{ID: id, Level: 1}, 1000000)
	})
	w.until(t, fmt.Sprintf("the cast of %d", id), done)
}

// playerAt reports whether the character c stands at at's X and Y.
func (w *zakenWorld) playerAt(t *testing.T, c attackable.Combatant, at location.Location) func() bool {
	return func() bool {
		x, y, _ := w.srv.PlayerPosition(t, c.ObjectID())
		return x == at.X && y == at.Y
	}
}

// TestZakenChecksByDayAndNight covers timer 1001's game-hour gate, 1 s
// after Zaken appeared: by day (12:00) it wants its day form (4223) and
// may again escape 3 HP quarters; at night (01:00, under hour 5) it wants
// its regeneration (4227) instead.
func TestZakenChecksByDayAndNight(t *testing.T) {
	t.Parallel()
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprintf("night=%v", night), func(t *testing.T) {
			t.Parallel()
			w := bootZaken(t, zakenBoot{night: night, forms: true})
			h := w.zaken(t)
			w.onQueue(t, h, func() { script.NPCOf(h).SetInt(npc.IntQuest2, 1) })
			frames := w.advance(t, time.Second)
			if got := wantsOrCast(frames, h, zakenNightToDay); got == night {
				t.Errorf("wants its day form = %v, want %v", got, !night)
			}
			if got := wantsOrCast(frames, h, zakenRegeneration); got != night {
				t.Errorf("wants its regeneration = %v, want %v", got, night)
			}
			want := int32(3)
			if night {
				want = 1
			}
			if got := zakenSlot(h, npc.IntQuest2); got != want {
				t.Errorf("HP quarters = %d, want %d", got, want)
			}
		})
	}
}

// TestZakenEscapesAQuarterByDay covers the attacked hook's game-hour gate
// and Instant Move. A hit of 10 adds 10 / max HP / 0.05 * 20000 to its
// attack desire on the character, unrounded. By day, the hit landing while Zaken
// stands under 3 of its 4 HP quarters takes one off and picks a room (a
// draw of 2 of 14: room 3, then 0 and 0 on X and Y) as its Instant Move
// point; Zaken casts Instant Move there, landing at the room's corner, and
// drops its attack desires. At night the same hit moves nothing.
func TestZakenEscapesAQuarterByDay(t *testing.T) {
	t.Parallel()
	room3 := location.Location{X: 54950, Y: 218790, Z: -3488}
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprintf("night=%v", night), func(t *testing.T) {
			t.Parallel()
			w := bootZaken(t, zakenBoot{night: night})
			h := w.zaken(t)
			(&ind1World{warriorWorld: w.warriorWorld}).setHPShare(t, h, 0.5)
			w.draws.set(1<<30, 9, 2, 0, 0)
			w.hit(t, h, 10)
			d, _ := attackOn(h, w.player)
			if want := 10 / float64(script.NPCOf(h).MaxHP()) / 0.05 * 20000; d.Weight != want {
				t.Errorf("attack desire on the character = %v, want %v", d.Weight, want)
			}
			if night {
				if got := zakenSlot(h, npc.IntQuest2); got != 3 {
					t.Errorf("HP quarters = %d, want 3", got)
				}
				if got := zakenDest(h); got != zakenHome {
					t.Errorf("Instant Move point = %v, want unchanged %v", got, zakenHome)
				}
				if _, ok := castDesireOf(h, zakenInstantMove); ok {
					t.Error("Zaken wants Instant Move at night")
				}
				return
			}
			if got := zakenSlot(h, npc.IntQuest2); got != 2 {
				t.Errorf("HP quarters = %d, want 2", got)
			}
			if got := zakenDest(h); got != room3 {
				t.Errorf("Instant Move point = %v, want room 3's corner %v", got, room3)
			}
			if d, ok := castDesireOf(h, zakenInstantMove); !ok || d.Weight != 10000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != h.ObjectID() {
				t.Fatalf("Instant Move desire = %+v, want on itself at 10000000", d)
			}
			w.until(t, "Instant Move", func() bool {
				at := positionOf(h)
				return at.X == room3.X && at.Y == room3.Y
			})
			if d, ok := attackOn(h, w.player); ok {
				t.Errorf("attack desire after Instant Move = %+v, want none", d)
			}
		})
	}
}

// TestZakenScattersItsTargets covers the skill-finished hook's Scatter
// Enemy and mass teleport. Scatter Enemy sends its target to a room (draws
// 2, 0, 0: room 3's corner) and drops Zaken's hate of it. The mass
// teleport does the same (draws 7, 152, 378: room 8's corner moved onto
// where Mate stands), then sends each other marked creature standing
// within 250 of, and less than 100 above or below, where the target lands
// to a room of its own (draws 5, 0, 0: room 6's corner). Mate, marked and
// standing at the landing point, 413 from where the character stood, goes
// there; the character, marked too, is the target and is skipped.
func TestZakenScattersItsTargets(t *testing.T) {
	t.Parallel()
	room3 := location.Location{X: 54950, Y: 218790, Z: -3488}
	room6 := location.Location{X: 55970, Y: 217770, Z: -3216}
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	w.settle(t)
	w.draws.set(1<<30, 9)
	w.hit(t, h, 10)

	w.draws.set(1<<30, 2, 0, 0)
	w.zakenCast(t, h, zakenScatterEnemy, w.player, w.playerAt(t, w.player, room3))
	w.appear(t)
	if got := h.AggroHate(w.player); got != 0 {
		t.Errorf("hate of the scattered character = %v, want 0", got)
	}

	// Mate enters the world where the character did, 200 west of Zaken and
	// 7 above the middle deck; the character, at room 3's corner, is still
	// within Zaken's reach.
	mate := (&ind1World{warriorWorld: w.warriorWorld}).joinCharacter(t, "mate", "Mate")
	w.onQueue(t, h, func() {
		n := script.NPCOf(h)
		n.SetSlot(npc.CreatureQuest0, script.PlayerOf(w.player))
		n.SetSlot(npc.CreatureQuest1, script.PlayerOf(mate))
		n.SetInt(npc.IntQuest0, 2)
	})
	landing := location.Location{X: zakenCharacter.X, Y: zakenCharacter.Y, Z: -3216}
	w.draws.set(1<<30, 7, 152, 378, 5, 0, 0)
	w.zakenCast(t, h, zakenMassTeleport, w.player, w.playerAt(t, w.player, landing))
	w.srv.Settle(t)
	if !w.playerAt(t, mate, room6)() {
		x, y, _ := w.srv.PlayerPosition(t, mate.ObjectID())
		t.Fatalf("Mate after the mass teleport at %d,%d, want room 6's corner %v", x, y, room6)
	}
}

// TestZakenMassTeleportSendsAMarkOnce covers a creature marked twice: Zaken
// marks what it sees with no check for an earlier mark. Mate, its second
// and third mark, stands 100 west of where the character lands (draws 7,
// 252, 378) and is sent to room 6's corner (draws 5, 0, 0); its second mark
// is measured where it lands, over 1500 from the character's landing
// point, so it is left alone and draws nothing: the draws queued after (2,
// 0, 0) stay queued.
func TestZakenMassTeleportSendsAMarkOnce(t *testing.T) {
	t.Parallel()
	room6 := location.Location{X: 55970, Y: 217770, Z: -3216}
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	w.settle(t)
	mate := (&ind1World{warriorWorld: w.warriorWorld}).joinCharacter(t, "mate", "Mate")
	w.onQueue(t, h, func() {
		n := script.NPCOf(h)
		n.SetSlot(npc.CreatureQuest0, script.PlayerOf(w.player))
		n.SetSlot(npc.CreatureQuest1, script.PlayerOf(mate))
		n.SetSlot(npc.CreatureQuest2, script.PlayerOf(mate))
		n.SetInt(npc.IntQuest0, 3)
	})
	landing := location.Location{X: zakenCharacter.X + 100, Y: zakenCharacter.Y, Z: -3216}
	w.draws.set(1<<30, 7, 252, 378, 5, 0, 0, 2, 0, 0)
	w.zakenCast(t, h, zakenMassTeleport, w.player, w.playerAt(t, w.player, landing))
	w.srv.Settle(t)
	if !w.playerAt(t, mate, room6)() {
		x, y, _ := w.srv.PlayerPosition(t, mate.ObjectID())
		t.Fatalf("Mate after the mass teleport at %d,%d, want room 6's corner %v", x, y, room6)
	}
	w.draws.mu.Lock()
	left := slices.Clone(w.draws.queue)
	w.draws.mu.Unlock()
	if !slices.Equal(left, []int{2, 0, 0}) {
		t.Fatalf("draws left after the mass teleport = %v, want [2 0 0]", left)
	}
}

// TestZakenBringsBackADeadPrivate covers the party-died hook: once every
// wave stands, a pirate captain dying while Zaken lives, its corpse then
// decayed, is asked back 30 s plus a draw of 60 after its death, here the
// highest (59): 89 s, and comes back at its own point.
func TestZakenBringsBackADeadPrivate(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	w.settle(t)
	alive := func() int {
		n := 0
		for _, p := range w.liveNPCs(zakenCaptainID) {
			if !p.Dead() {
				n++
			}
		}
		return n
	}
	if got := alive(); got != 36 {
		t.Fatalf("setup: live pirate captains = %d, want 36", got)
	}
	victim := w.liveNPCs(zakenCaptainID)[0]
	home := positionOf(victim)
	before := time.Now()
	w.kill(t, victim)
	after := time.Now()
	if got := alive(); got != 35 {
		t.Fatalf("live pirate captains after the death = %d, want 35", got)
	}
	w.onQueue(t, victim, func() { victim.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(victim.ObjectID())) })
	var armed []string
	for i := 1; i <= 256; i++ {
		if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
			armed = append(armed, key)
		}
	}
	if len(armed) != 1 {
		t.Fatalf("privates with a respawn pending = %q, want one", armed)
	}
	deadline, ok := w.srv.NpcRespawns.Deadline(armed[0])
	if !ok {
		t.Fatalf("no deadline for the armed respawn %s", armed[0])
	}
	const delay = 89 * time.Second
	if deadline.Before(before.Add(delay)) || deadline.After(after.Add(delay)) {
		t.Fatalf("respawn due %s after the death, want %s", deadline.Sub(before), delay)
	}

	w.srv.NpcSpawns.Respawn(armed[0])
	w.srv.Settle(t)
	if got := alive(); got != 36 {
		t.Fatalf("live pirate captains after the respawn = %d, want 36", got)
	}
	back := slices.ContainsFunc(w.liveNPCs(zakenCaptainID), func(p *npc.Hostile) bool {
		at := positionOf(p)
		return !p.Dead() && at.X == home.X && at.Y == home.Y
	})
	if !back {
		t.Errorf("no live pirate captain at the dead one's point %v", home)
	}
}

// TestZakenSeesAndMarks covers the see-creature hook. A grand boss is
// told about a playable on its deck on each AI tick that either of them
// moves: the character, standing still beside Zaken since it entered the
// world, is first seen walking. On that tick Zaken, inside its territory,
// wants to attack it at 200, marks it as its first mark (a draw of 0 of 3)
// and, on a draw of 0 of 15, picks a skill: a draw of 0 of 225 wants
// Scatter Enemy on it.
func TestZakenSeesAndMarks(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	w.settle(t)
	if _, ok := attackOn(h, w.player); ok {
		t.Fatal("setup: Zaken wants to attack the standing character")
	}
	w.draws.set(1<<30, 0, 0, 0)
	w.walkStep(t, zakenCharacter, location.Location{X: zakenCharacter.X, Y: zakenCharacter.Y + 300, Z: zakenCharacter.Z})
	w.ticks(t, 1)
	if d, ok := attackOn(h, w.player); !ok || d.Weight != 200 {
		t.Errorf("attack desire on the character = %+v (%v), want 200", d, ok)
	}
	if got := zakenSlot(h, npc.IntQuest0); got != 1 {
		t.Errorf("marks = %d, want 1", got)
	}
	if got := script.NPCOf(h).Slot(npc.CreatureQuest0); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Errorf("first mark = %v, want the character", got)
	}
	if !castOn(h, zakenScatterEnemy, w.player.ObjectID()) {
		t.Error("Zaken does not want Scatter Enemy on the character")
	}
}

// TestZakenPicksASkillWhenHit covers the attacked hook's skill choice: a
// hit, on a draw of 0 of 10, draws 4 of 225 and wants Zaken's 4218 on the
// character.
func TestZakenPicksASkillWhenHit(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	w.settle(t)
	w.draws.set(1<<30, 0, 4)
	w.hit(t, h, 10)
	if !castOn(h, 4218, w.player.ObjectID()) {
		t.Error("Zaken does not want 4218 on the character")
	}
}

// TestZakenAnswersAPrivateHitAtNight covers the party-attacked hook's
// game-hour gate. At night, a private of Zaken's hit for 5 while Zaken
// wanders, on a draw of 0 of 450, makes the private's point Zaken's
// Instant Move point; 300 ms later timer 1002 drops its marks and wants
// Instant Move. A hit of 10 draws nothing and moves nothing. By day the
// light hit moves nothing either.
func TestZakenAnswersAPrivateHitAtNight(t *testing.T) {
	t.Parallel()
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprintf("night=%v", night), func(t *testing.T) {
			t.Parallel()
			w := bootZaken(t, zakenBoot{night: night})
			h := w.zaken(t)
			w.settle(t)
			for i := 0; script.NPCOf(h).Intention() != script.IntentionWander; i++ {
				if i == 5 {
					t.Fatalf("setup: Zaken's intention = %v after 5 AI ticks, want wander", script.NPCOf(h).Intention())
				}
				w.ticks(t, 1)
			}
			// The captain of room 3, 413 from where the character stands.
			var captain *npc.Hostile
			for _, p := range w.liveNPCs(zakenCaptainID) {
				if at := positionOf(p); at.X == 54950+649 && at.Y == 218790+649 {
					captain = p
				}
			}
			if captain == nil {
				t.Fatal("setup: no pirate captain in room 3")
			}
			w.onQueue(t, h, func() { script.NPCOf(h).SetInt(npc.IntQuest0, 2) })

			w.draws.set(1<<30, 0)
			w.hit(t, captain, 10)
			if got := zakenDest(h); got != zakenHome {
				t.Fatalf("Instant Move point after a hit of 10 = %v, want unchanged %v", got, zakenHome)
			}
			w.hit(t, captain, 5)
			w.advance(t, 300*time.Millisecond)
			if !night {
				if got := zakenDest(h); got != zakenHome {
					t.Errorf("Instant Move point by day = %v, want unchanged %v", got, zakenHome)
				}
				if got := zakenSlot(h, npc.IntQuest0); got != 2 {
					t.Errorf("marks by day = %d, want 2", got)
				}
				return
			}
			if got, want := zakenDest(h), positionOf(captain); got != want {
				t.Errorf("Instant Move point = %v, want the captain's point %v", got, want)
			}
			if got := zakenSlot(h, npc.IntQuest0); got != 0 {
				t.Errorf("marks after timer 1002 = %d, want 0", got)
			}
			if d, ok := castDesireOf(h, zakenInstantMove); !ok || d.Weight != 10000000 {
				t.Errorf("Instant Move desire = %+v (%v), want one at 10000000", d, ok)
			}
		})
	}
}

// TestZakenLeavesAFarFightAtNight covers timer 1001's night move: Zaken
// fighting a top desire target more than 1500 from its Instant Move point,
// with no mark, picks a room (draws 2, 0, 0: room 3's corner) and wants
// Instant Move. By day the same fight moves nothing.
func TestZakenLeavesAFarFightAtNight(t *testing.T) {
	t.Parallel()
	room3 := location.Location{X: 54950, Y: 218790, Z: -3488}
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprintf("night=%v", night), func(t *testing.T) {
			t.Parallel()
			w := bootZaken(t, zakenBoot{night: night})
			h := w.zaken(t)
			w.settle(t)
			w.draws.set(1<<30, 9)
			w.hit(t, h, 10)
			w.ticks(t, 1)
			if got := script.NPCOf(h).Intention(); got != script.IntentionAttack {
				t.Fatalf("setup: Zaken's intention = %v, want attack", got)
			}
			w.onQueue(t, h, func() { script.NPCOf(h).SetInt(script.SlotIntAI1, int32(zakenHome.X+1600)) })

			w.draws.set(1<<30, 2, 0, 0)
			w.advance(t, 31*time.Second-7*1700*time.Millisecond)
			if !night {
				if _, ok := castDesireOf(h, zakenInstantMove); ok {
					t.Error("Zaken wants Instant Move by day")
				}
				return
			}
			if got := zakenDest(h); got != room3 {
				t.Errorf("Instant Move point = %v, want room 3's corner %v", got, room3)
			}
			if d, ok := castDesireOf(h, zakenInstantMove); !ok || d.Weight != 10000000 {
				t.Errorf("Instant Move desire = %+v (%v), want one at 10000000", d, ok)
			}
		})
	}
}

// TestZakenDropsALongFightAtNight covers timer 1001's night streak: Zaken
// fighting the same top desire target counts each check; the sixth drops
// its hate of that target and starts the count again.
func TestZakenDropsALongFightAtNight(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{night: true})
	h := w.zaken(t)
	w.settle(t)
	w.draws.set(1<<30, 9)
	w.hit(t, h, 10)
	w.ticks(t, 1)
	w.advance(t, 31*time.Second-7*1700*time.Millisecond)
	for i := int32(1); i <= 5; i++ {
		if got := zakenSlot(h, npc.IntQuest1); got != i {
			t.Fatalf("checks on the character after check %d = %d, want %d", i, got, i)
		}
		if h.AggroHate(w.player) == 0 {
			t.Fatalf("Zaken no longer hates the character after check %d", i)
		}
		w.advance(t, 30*time.Second)
	}
	if got := zakenSlot(h, npc.IntQuest1); got != 0 {
		t.Errorf("checks after the sixth = %d, want 0", got)
	}
	if got := h.AggroHate(w.player); got != 0 {
		t.Errorf("hate of the character after the sixth check = %v, want 0", got)
	}
}

// TestZakenHatesASpellByWholeShares covers the see-spell hook: the
// character's self buff of 2500 aggro points, not offensive, adds to
// Zaken's attack desire on it 2500 / max HP, in whole numbers, times 1500.
func TestZakenHatesASpellByWholeShares(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := modelskill.Definition{
		ID: zakenAggroBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: 2500,
	}
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, buff})
	w := bootZaken(t, zakenBoot{
		extra: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(buff),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(buff.ID), 1); err != nil {
				t.Fatalf("seed skill: %v", err)
			}
		},
	})
	h := w.zaken(t)
	w.settle(t)
	maxHP := script.NPCOf(h).MaxHP()
	want := float64(2500 / maxHP * 10 * 150)
	if want == 0 || want == 2500/float64(maxHP)*1500 {
		t.Fatalf("setup: max HP %d does not tell whole shares from fractions", maxHP)
	}
	castOwnSkill(w.warriorWorld, zakenAggroBuffID)
	w.srv.AdvanceUntil(t, "the attack desire", func() bool {
		_, ok := attackOn(h, w.player)
		return ok
	})
	if d, _ := attackOn(h, w.player); d.Weight != want {
		t.Fatalf("attack desire on the caster = %v, want %v", d.Weight, want)
	}
}

// TestZakenAppearsAndDiesWithItsMusic covers the music of the created and
// dying hooks: Zaken plays BS01_A bound to itself as it appears, and
// BS02_D 3 s after its death.
func TestZakenAppearsAndDiesWithItsMusic(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	n, err := w.srv.NpcSpawns.AddSpawn(zakenID, zakenCharacter.X, zakenCharacter.Y+200, zakenCharacter.Z, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.srv.Settle(t)
	h := n.(*npc.Hostile)
	want := ind8MusicPayload(h.ObjectID(), "BS01_A", positionOf(h))
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodePlaySound); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("PlaySound frames as Zaken appears = %x, want one of BS01_A bound to it", got)
	}

	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	frames := w.advance(t, 3*time.Second)
	want = ind8MusicPayload(h.ObjectID(), "BS02_D", positionOf(h))
	if got := ind8Pick(frames, serverpackets.OpcodePlaySound); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("PlaySound frames after the death = %x, want one of BS02_D bound to Zaken", got)
	}
}
