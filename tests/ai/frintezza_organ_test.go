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

// Frintezza's organ dummy, the tomb's seeker and the follower's dummy
// (#198 PR 5), all civilians, beside the real Frintezza.
const (
	foSeekerID = int32(29059)
	foDummyID  = int32(29053)
	foSeekRow  = "fo_seeker"
)

// foHall is where Frintezza leaves his organ dummy, facing 16384.
var foHall = location.Location{X: 174239, Y: -89809, Z: -5016}

// foBoot is one tomb fixture: Frintezza's live row (nil for none), and
// whether he is placed at all; the seeker's position ("" for no seeker)
// and whether a live row of it is saved; and whether the character enters
// the world beside the hall rather than the tomb.
type foBoot struct {
	atHall    bool
	frintezza bool
	row       *spawn.State
	seekerAt  string
	seekerRow bool
}

// foSpawnlist places Frintezza under the shipped database maker, then the
// seeker under a database maker, and declares frintessa_2515_m01 a probe.
func foSpawnlist(c foBoot) string {
	out := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fo_tomb" minZ="0" maxZ="100"><node x="-400" y="-400"/><node x="400" y="-400"/><node x="400" y="400"/><node x="-400" y="400"/></territory>
`
	if c.frintezza {
		out += `	<npcmaker name="frintessa_maker" territory="fo_tomb" maximumNpcs="9">
		<ai type="default_use_db_maker_for_frintessa"/>
		<npc id="29045" pos="200;20;30;0" total="1" respawn="48hour" dbName="frintessa"/>
	</npcmaker>
`
	}
	if c.seekerAt != "" {
		out += fmt.Sprintf(`	<npcmaker name="fo_seeker_maker" territory="fo_tomb" maximumNpcs="1">
		<ai type="default_use_db_maker"/>
		<npc id="29059" pos=%q total="1" respawn="0sec" dbName=%q/>
	</npcmaker>
`, c.seekerAt, foSeekRow)
	}
	return out + `	<npcmaker name="frintessa_2515_m01" territory="fo_tomb" maximumNpcs="1"><ai type="rb5_probe"/></npcmaker>
</list>`
}

// foFolkTemplate is a standing civilian fixture of id.
func foFolkTemplate(id int32) *npc.Template {
	tmpl := folkFighterTemplate(id)
	tmpl.CanMove = false
	return tmpl
}

// bootOrgan boots the tomb c describes, backed by the spawn_data table,
// with Frintezza's songs and the dummies' skills (5004 on itself, 5005 on
// one target) as NPC skills.
func bootOrgan(t *testing.T, c foBoot) *frWorld {
	t.Helper()
	rows := map[string]*spawn.State{}
	if c.row != nil {
		rows[fzRow] = c.row
	}
	if c.seekerRow {
		rows[foSeekRow] = &spawn.State{Name: foSeekRow, Status: spawn.StatusAlive, CurrentHP: 1 << 20, CurrentMP: 1 << 20, Location: location.Location{X: 20000, Y: 20000, Z: 30}}
	}
	if len(rows) > 0 {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), rows); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fo.xml"), []byte(foSpawnlist(c)), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":          defaultnpc.New,
		"script.ai.boss.frintezza.FollowerDummy":   frintezza.FollowerDummy,
		"script.ai.boss.frintezza.Frintezza":       frintezza.Frintezza,
		"script.ai.boss.frintezza.FrintezzaSeeker": frintezza.FrintezzaSeeker,
		"script.ai.boss.frintezza.OrganDummy":      frintezza.OrganDummy,
		"test." + frDirector:                       func() script.Script { return script.Script{} },
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.frintezza.FollowerDummy"},
		{Path: "script.ai.boss.frintezza.Frintezza"},
		{Path: "script.ai.boss.frintezza.FrintezzaSeeker"},
		{Path: "script.ai.boss.frintezza.OrganDummy"},
		{Path: "test." + frDirector},
	}
	boss := warriorTemplate(fzID, nil)
	// His HP outlasts the organ dummy's guard hits.
	boss.Type, boss.CorpseTime, boss.CanMove, boss.HPMax = "GrandBoss", 18, false, 1<<20
	templates := []*npc.Template{boss, foFolkTemplate(fzOrganID), foFolkTemplate(foSeekerID), foFolkTemplate(foDummyID)}
	kinds := map[int32]script.NPCKind{fzID: script.KindHostile, fzOrganID: script.KindFolk, foSeekerID: script.KindFolk, foDummyID: script.KindFolk}
	skills := foSkills()
	makers := &rb5Makers{}
	boot := raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(skills),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers.catalog()),
	}}
	if c.atHall {
		boot.ready = func(srv *gameservertest.Server, objID int32) {
			if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, foHall.X+100, foHall.Y, foHall.Z, objID); err != nil {
				t.Fatalf("place the character at the hall: %v", err)
			}
		}
	}
	w := bootRaid(t, boot, templates...)
	return &frWorld{warriorWorld: w, makers: makers}
}

// foSkills are Frintezza's songs and the dummies' skills, 5004 on itself
// and 5005, a buff, on one target, as NPC skills.
func foSkills() *modelskill.Table {
	skill := func(id modelskill.ID, level int, self bool) modelskill.Definition {
		d := ind1Skill(id, 600, self)
		d.Level = level
		return d
	}
	organ := skill(5005, 1, false)
	organ.Offensive, organ.Magic, organ.SkillType, organ.Power = false, false, "BUFF", 0
	return modelskill.NewTable([]modelskill.Definition{
		skill(5006, 1, true), skill(5007, 1, true), skill(5007, 2, true), skill(5007, 3, true), skill(5007, 4, true), skill(5007, 5, true),
		skill(5004, 1, true), organ,
	})
}

// folks returns the civilian NPCs of template id in the world.
func (w *frWorld) folks(id int32) []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.NpcID()) == id {
			out = append(out, f)
		}
	}
	return out
}

// organ returns the one organ dummy in the world.
func (w *frWorld) organ(t *testing.T) *npc.Folk {
	t.Helper()
	live := w.folks(fzOrganID)
	if len(live) != 1 {
		t.Fatalf("organ dummies in the world = %d, want 1", len(live))
	}
	return live[0]
}

// sendFolk sends f script event eventID with arg1 from the director.
func (w *frWorld) sendFolk(t *testing.T, f *npc.Folk, eventID, arg1 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(f), eventID, arg1, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// onFolkQueue runs fn on f's queue and waits for it.
func (w *frWorld) onFolkQueue(t *testing.T, f *npc.Folk, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !f.Queue().Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("the npc's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// folkDesires renders f's queued attack and cast desires as
// "attack>target=weight" and "skill-level>target=weight".
func folkDesires(f *npc.Folk) []string {
	var out []string
	for _, d := range f.Desires().Snapshot() {
		switch d.Kind {
		case actorai.IntentionAttack:
			out = append(out, fmt.Sprintf("attack>%d=%v", d.FinalTarget.ObjectID(), d.Weight))
		case actorai.IntentionCast:
			out = append(out, fmt.Sprintf("%d-%d>%d=%v", d.Skill.ID, d.Skill.Level, d.FinalTarget.ObjectID(), d.Weight))
		}
	}
	slices.Sort(out)
	return out
}

// foShot is one camera shot: its delay after the shot before, then
// distance, yaw, pitch, time, duration, turn and rise.
type foShot struct {
	gap    time.Duration
	fields []int32
}

// foAwakening is the organ dummy's film of Frintezza's awakening, timers
// 1000 to 1019; the first comes 1 ms after it appears.
var foAwakening = []foShot{
	{1, []int32{2500, 180, 20, 0, 10000, 20, 90}},
	{100, []int32{2500, 180, 20, 100, 10000, 20, 90}},
	{400, []int32{2000, 190, 0, 4000, 10000, 0, 30}},
	{4000, []int32{100, 180, 0, 5000, 10000, 0, 0}},
	{7300, []int32{5, 165, -15, 5000, 10000, 6, 15}},
	{7300, []int32{1100, 240, 0, 0, 10000, 210, 0}},
	{100, []int32{1100, 240, 0, 0, 10000, 210, 0}},
	{3400, []int32{150, 180, 0, 0, 10000, 0, 0}},
	{100, []int32{150, 180, 0, 0, 10000, 0, 0}},
	{400, []int32{50, 180, 20, 4000, 10000, 0, -5}},
	{5000, []int32{0, 280, 40, 0, 10000, 0, 0}},
	{2500, []int32{500, 230, 45, 7500, 10000, 15, 3}},
	{7500, []int32{2100, 200, 13, 9000, 10000, 4, 10}},
	{9500, []int32{2000, 190, 0, 0, 10000, -50, 45}},
	{100, []int32{2000, 190, 0, 0, 10000, -50, 45}},
	{1000, []int32{2000, 195, 0, 1800, 10000, -47, 42}},
	{1800, []int32{2000, 195, 0, 600, 10000, -47, 0}},
	{2800, []int32{1800, 175, 0, 0, 10000, 40, -3}},
	{100, []int32{1800, 175, 0, 0, 10000, 40, -3}},
	{900, []int32{2000, 180, 1, 2000, 3000, 0, -5}},
}

// foLastSong is the film of Frintezza's last song, timers 2000 to 2005,
// the first a second after script event arg1 50000.
var foLastSong = []foShot{
	{1000, []int32{500, 210, 10, 0, 10000, 10, 0}},
	{100, []int32{500, 210, 10, 0, 10000, 10, 0}},
	{400, []int32{5, 210, 10, 5500, 10000, 0, 0}},
	{5500, []int32{5, 150, 10, 0, 10000, 0, 0}},
	{100, []int32{5, 150, 10, 0, 10000, 0, 0}},
	{1000, []int32{1500, 180, 10, 6000, 6000, 0, 13}},
}

// foDeath is the film of Frintezza's death, timers 3000, 3001, 3002 and
// 3006, the first 5 s after script event arg1 8.
var foDeath = []foShot{
	{5000, []int32{0, 230, 10, 0, 10000, 0, -5}},
	{100, []int32{0, 230, 10, 0, 10000, 0, -5}},
	{400, []int32{0, 180, 10, 5000, 10000, 0, -5}},
	{7000, []int32{1000, 180, 15, 8000, 8000, 0, 10}},
}

// film plays shots of the organ dummy f, each checked to come at its gap
// and not a millisecond before, every shot widescreen; at, when set, runs
// after the shot of its index.
func (w *frWorld) film(t *testing.T, name string, f *npc.Folk, shots []foShot, at map[int]func()) {
	t.Helper()
	for i, shot := range shots {
		if shot.gap > 1 {
			w.srv.Advance(t, shot.gap*time.Millisecond-time.Millisecond)
			if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
				t.Fatalf("%s: camera frames before shot %d = %x, want none", name, i+1, got)
			}
		}
		w.srv.Advance(t, time.Millisecond)
		got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera)
		if want := ind8CameraPayload(f.ObjectID(), append(slices.Clone(shot.fields), 1, 1)...); len(got) != 1 || !slices.Equal(got[0], want) {
			t.Fatalf("%s: camera frames at shot %d = %x, want %x", name, i+1, got, want)
		}
		if fn := at[i]; fn != nil {
			fn()
		}
	}
}

// TestOrganDummyFilmsTheAwakening restores Frintezza at db value 3 at his
// hall, which leaves his organ dummy there facing 16384. Frintezza below
// 4, the dummy films his awakening: 1 ms after it appears BS04_A plays
// bound to it, then its first camera shot, and nineteen more over about
// 76 s, the tenth also making it want to attack Frintezza, weight 200. 3 s
// after the last its guard starts, which keeps it beside him.
func TestOrganDummyFilmsTheAwakening(t *testing.T) {
	t.Parallel()
	w := bootOrgan(t, foBoot{atHall: true, frintezza: true, row: fzHallRow(3)})
	fz := w.frintezza(t)
	f := w.organ(t)
	if x, y, z := f.Position(); x != foHall.X || y != foHall.Y || z != foHall.Z || f.Heading() != 16384 {
		t.Fatalf("organ dummy at %d,%d,%d facing %d, want the hall facing 16384", x, y, z, f.Heading())
	}

	w.srv.Advance(t, time.Millisecond)
	var got [][]byte
	for _, fr := range w.srv.ReadQueued(t, w.srv.Client) {
		if fr[0] == serverpackets.OpcodePlaySound || fr[0] == serverpackets.OpcodeSpecialCamera {
			got = append(got, fr)
		}
	}
	want := [][]byte{
		ind8MusicPayload(f.ObjectID(), "BS04_A", foHall),
		ind8CameraPayload(f.ObjectID(), append(slices.Clone(foAwakening[0].fields), 1, 1)...),
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("frames at 1 ms = %x, want BS04_A then the first shot %x", got, want)
	}
	w.film(t, "awakening", f, foAwakening[1:], map[int]func(){
		8: func() {
			if got, want := folkDesires(f), []string{fmt.Sprintf("attack>%d=200", fz.ObjectID())}; !slices.Equal(got, want) {
				t.Fatalf("desires at the tenth shot = %q, want %q", got, want)
			}
		},
	})
	w.srv.Advance(t, 10*time.Second)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames after the last shot = %x, want none", got)
	}
	if got := len(w.folks(fzOrganID)); got != 1 {
		t.Fatalf("organ dummies guarding Frintezza beside it = %d, want 1", got)
	}
}

// fzHallRow is a live row of Frintezza at value standing at his hall.
func fzHallRow(value int) *spawn.State {
	row := fzLiveRow(value)
	row.Location = foHall
	return row
}

// TestOrganDummyGuardsFrintezza restores Frintezza at db value 4 at his
// hall: his organ dummy films nothing and a second later starts its
// guard, wanting to attack him, weight 200, as he stands within 20 of
// it, again every 10 s. Once he stands farther, its next guard takes it
// out of the world.
func TestOrganDummyGuardsFrintezza(t *testing.T) {
	t.Parallel()
	w := bootOrgan(t, foBoot{atHall: true, frintezza: true, row: fzHallRow(4)})
	fz := w.frintezza(t)
	f := w.organ(t)
	w.srv.Advance(t, time.Second-time.Millisecond)
	if got := folkDesires(f); len(got) != 0 {
		t.Fatalf("desires before the guard = %q, want none", got)
	}
	w.srv.Advance(t, time.Millisecond)
	want := []string{fmt.Sprintf("attack>%d=200", fz.ObjectID())}
	if got := folkDesires(f); !slices.Equal(got, want) {
		t.Fatalf("desires at the guard = %q, want %q", got, want)
	}
	w.onFolkQueue(t, f, func() { f.Desires().Clear() })
	w.srv.Advance(t, 10*time.Second)
	if got := folkDesires(f); !slices.Equal(got, want) {
		t.Fatalf("desires at the second guard = %q, want %q", got, want)
	}
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames while guarding = %x, want none", got)
	}
	w.onQueue(t, fz, func() { script.NPCOf(fz).TeleportTo(script.Loc{X: foHall.X + 21, Y: foHall.Y, Z: foHall.Z}) })
	w.srv.Advance(t, 10*time.Second)
	if got := w.folks(fzOrganID); len(got) != 0 {
		t.Fatalf("organ dummies after a guard 21 from Frintezza = %d, want none", len(got))
	}
}

// TestOrganDummyFilmsTheEnd runs the organ dummy's script-event hook while
// it guards Frintezza: arg1 50000 films his last song a second later (six
// shots); arg1 8 films his death 5 s later, its second shot after it drops
// every desire and wants to cast skill 5005 level 1 on Frintezza, weight
// 1000000. Another arg1 films nothing.
func TestOrganDummyFilmsTheEnd(t *testing.T) {
	t.Parallel()
	w := bootOrgan(t, foBoot{atHall: true, frintezza: true, row: fzHallRow(4)})
	fz := w.frintezza(t)
	f := w.organ(t)
	w.srv.Advance(t, time.Second)
	w.srv.ReadQueued(t, w.srv.Client)

	w.sendFolk(t, f, 0, 7)
	w.srv.Advance(t, 6*time.Second)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames after arg1 7 = %x, want none", got)
	}
	w.sendFolk(t, f, 0, 50000)
	w.film(t, "last song", f, foLastSong, nil)
	w.sendFolk(t, f, 0, 8)
	w.film(t, "death", f, foDeath, map[int]func(){
		1: func() {
			if got, want := folkDesires(f), []string{fmt.Sprintf("5005-1>%d=1e+06", fz.ObjectID())}; !slices.Equal(got, want) {
				t.Fatalf("desires at the death's second shot = %q, want %q", got, want)
			}
		},
	})
	w.srv.Advance(t, 10*time.Second)
	if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("camera frames after the death's last shot = %x, want none", got)
	}
}

// TestOrganDummyDiesWithFrintezza runs the organ dummy's party-died hook:
// Frintezza's death kills his organ dummy, his private.
func TestOrganDummyDiesWithFrintezza(t *testing.T) {
	t.Parallel()
	w := bootOrgan(t, foBoot{atHall: true, frintezza: true, row: fzHallRow(4)})
	f := w.organ(t)
	w.kill(t, w.frintezza(t))
	if !f.Dead() {
		t.Fatal("the organ dummy is alive after Frintezza died")
	}
}

// TestFrintezzaSeekerLeavesAnOpenTomb runs the seeker's created hook, far
// from the character. Placed from a live row it stays while Frintezza is
// alive at db value 1, and leaves the world when he is at 2 or not there
// at all. Placed with no row it stays whatever his value.
func TestFrintezzaSeekerLeavesAnOpenTomb(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		boot foBoot
		stay bool
	}{
		{"loaded at 1", foBoot{frintezza: true, row: fzLiveRow(1), seekerRow: true}, true},
		{"loaded at 2", foBoot{frintezza: true, row: fzLiveRow(2), seekerRow: true}, false},
		{"loaded alone", foBoot{seekerRow: true}, false},
		{"not loaded at 2", foBoot{frintezza: true, row: fzLiveRow(2)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.boot.seekerAt = "20000;20000;30;0"
			w := bootOrgan(t, tc.boot)
			if got := len(w.folks(foSeekerID)); got != map[bool]int{true: 1, false: 0}[tc.stay] {
				t.Fatalf("seekers in the world = %d, want stay=%v", got, tc.stay)
			}
		})
	}
}

// TestFrintezzaSeekerCallsFrintezza runs the seeker's see-creature hook
// beside the character: while Frintezza is alive at db value 1 or less,
// seeing the character it tells him event 0, arg1 2, which takes him to
// db value 2 and runs maker event 1000 on frintessa_2515_m01, and it
// leaves the world. At 2 it sees the character and stays.
func TestFrintezzaSeekerCallsFrintezza(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value int
		call  bool
	}{{1, true}, {2, false}} {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			t.Parallel()
			w := bootOrgan(t, foBoot{frintezza: true, row: fzLiveRow(tc.value), seekerAt: "100;100;30;0"})
			n := script.NPCOf(w.frintezza(t))
			w.makers.take()
			if got := len(w.folks(foSeekerID)); got != 1 {
				t.Fatalf("seekers before it looks = %d, want 1", got)
			}
			cycle(t, w.srv)
			w.srv.Settle(t)
			wantValue, wantSeekers, wantMakers := tc.value, 1, []string(nil)
			if tc.call {
				wantValue, wantSeekers, wantMakers = 2, 0, []string{"frintessa_2515_m01 1000"}
			}
			if got := n.DBValue(); got != wantValue {
				t.Fatalf("Frintezza's db value after the seeker looked = %d, want %d", got, wantValue)
			}
			if got := len(w.folks(foSeekerID)); got != wantSeekers {
				t.Fatalf("seekers after it looked = %d, want %d", got, wantSeekers)
			}
			if got := w.makers.take(); !slices.Equal(got, wantMakers) {
				t.Fatalf("maker events after the seeker looked = %q, want %q", got, wantMakers)
			}
		})
	}
}

// TestFollowerDummyCastsAndLeaves runs the follower's dummy (29053): as it
// appears it wants to cast skill 5004 level 1 on itself, weight 1000000,
// and it leaves the world 10.2 s later.
func TestFollowerDummyCastsAndLeaves(t *testing.T) {
	t.Parallel()
	w := bootOrgan(t, foBoot{})
	if !w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(s *script.Script, _ *script.Player) {
		s.Spawner().AddSpawn(foDummyID, script.Loc{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}, false, 0)
	}) {
		t.Fatal("the spawn panicked")
	}
	w.srv.Settle(t)
	dummies := w.folks(foDummyID)
	if len(dummies) != 1 {
		t.Fatalf("dummies as it appeared = %d, want 1", len(dummies))
	}
	d := dummies[0]
	if got, want := folkDesires(d), []string{fmt.Sprintf("5004-1>%d=1e+06", d.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("desires as it appeared = %q, want %q", got, want)
	}
	var casts []int32
	for range 3 {
		casts = append(casts, folkCasts(cycle(t, w.srv), d.ObjectID(), 5004)...)
	}
	if want := []int32{d.ObjectID()}; !slices.Equal(casts, want) {
		t.Fatalf("casts of 5004 over three AI ticks = %v, want one on itself %v", casts, want)
	}
	w.srv.Advance(t, 10200*time.Millisecond-time.Millisecond)
	if got := len(w.folks(foDummyID)); got != 1 {
		t.Fatalf("dummies before 10.2 s = %d, want 1", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := len(w.folks(foDummyID)); got != 0 {
		t.Fatalf("dummies at 10.2 s = %d, want none", got)
	}
}
