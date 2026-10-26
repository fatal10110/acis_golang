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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The tomb's halls (#198 PR 5): the alarm device and the two portraits.
const (
	fhAlarmID    = int32(18328)
	fhEvilateAID = int32(29049)
	fhEvilateBID = int32(29048)
	fhSpiritID   = int32(29050)
	fhWingedID   = int32(29051)
	fhDewdropID  = modelskill.ID(2276)
	fhOtherID    = modelskill.ID(1177)
)

// fhAlarmMakers are the npcmakers the alarm signals, in the order a
// de-activation runs maker event 1000 on them.
var fhAlarmMakers = []string{
	"godard32_2515_22m1", "godard32_2515_19m1", "godard32_2515_20m1", "godard32_2515_21m1",
	"godard32_2515_07m1", "godard32_2515_06m1", "godard32_2515_05m1",
	"godard32_2515_01m1", "godard32_2515_02m1", "godard32_2515_03m1", "godard32_2515_04m1", "godard32_2515_08m1",
	"godard32_2515_09m1",
}

// fhAlarmDoors are the doors a de-activation opens, in its order.
var fhAlarmDoors = []string{
	"grave_pathway_1", "grave_pathway_2",
	"wall_door_a_center_1", "wall_door_a_center_2", "wall_door_a_center_3", "wall_door_a_center_4",
	"wall_door_a_center_5", "wall_door_a_center_6", "wall_door_a_center_7", "wall_door_a_center_8",
}

// fhDoorID is the door id of the fixture door at index i of fhAlarmDoors.
func fhDoorID(i int) int { return 25150101 + i }

// fhDoors are the alarm's doors as fixtures, every one closed, spread out
// away from the character.
func fhDoors() []*door.Template {
	var out []*door.Template
	for i, name := range fhAlarmDoors {
		at := location.Location{X: 3000 + i*300, Y: 3000, Z: 30}
		out = append(out, &door.Template{
			ID: fhDoorID(i), Name: name, Kind: door.KindDoor, Level: 1,
			Position:    at,
			Coordinates: []location.Point{{X: at.X - 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y + 100}, {X: at.X - 50, Y: at.Y + 100}},
			HP:          100000, PDef: 600, MDef: 500, Height: 1024,
		})
	}
	return out
}

// fhSpawnlist declares the alarm's npcmakers, every one an empty probe.
func fhSpawnlist() string {
	out := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fh_hall" minZ="0" maxZ="100"><node x="-400" y="-400"/><node x="400" y="-400"/><node x="400" y="400"/><node x="-400" y="400"/></territory>
`
	for _, m := range fhAlarmMakers {
		out += fmt.Sprintf("\t<npcmaker name=%q territory=\"fh_hall\" maximumNpcs=\"1\"><ai type=\"rb5_probe\"/></npcmaker>\n", m)
	}
	return out + "</list>"
}

// fhSkills are the character's skills: the dewdrop (2276) and another
// attack, each one MDAM hit of power 50.
func fhSkills() []modelskill.Definition {
	hit := func(id modelskill.ID) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, Magic: true, SkillType: "MDAM", CastRange: 1200, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Power: 50,
		}
	}
	return []modelskill.Definition{hit(fhDewdropID), hit(fhOtherID)}
}

// fhSpiritAt is where the fixture portraits raise their spirits.
var fhSpiritAt = [4]int{1500, 1600, 30, 3368}

// bootTombHalls boots the character knowing fhSkills beside the alarm and the
// portraits, the portraits naming fhSpiritAt, with draws as the script
// draws.
func bootTombHalls(t *testing.T, draws *ind1Draws) *frWorld {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fh.xml"), []byte(fhSpawnlist()), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":      defaultnpc.New,
		"script.ai.boss.frintezza.AlarmDevice": frintezza.AlarmDevice,
		"script.ai.boss.frintezza.EvilateA":    frintezza.EvilateA,
		"script.ai.boss.frintezza.EvilateB":    frintezza.EvilateB,
		"test." + frDirector:                   func() script.Script { return script.Script{} },
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.frintezza.AlarmDevice"},
		{Path: "script.ai.boss.frintezza.EvilateA"},
		{Path: "script.ai.boss.frintezza.EvilateB"},
		{Path: "test." + frDirector},
	}
	params := npc.AIParams{
		"SpawnPosX": fmt.Sprint(fhSpiritAt[0]), "SpawnPosY": fmt.Sprint(fhSpiritAt[1]),
		"SpawnPosZ": fmt.Sprint(fhSpiritAt[2]), "SpawnAngle": fmt.Sprint(fhSpiritAt[3]),
	}
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range []*npc.Template{
		warriorTemplate(fhAlarmID, nil), warriorTemplate(fhEvilateAID, params), warriorTemplate(fhEvilateBID, params),
		warriorTemplate(fhSpiritID, nil), warriorTemplate(fhWingedID, nil),
	} {
		tmpl.CanMove, tmpl.HPMax, tmpl.PDef, tmpl.MDef = false, 100000, 500, 500
		templates = append(templates, tmpl)
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	db := sqltest.SharedDB(t)
	defs := fhSkills()
	skills := modelskill.NewTable(defs)
	makers := &rb5Makers{}
	w := bootRaid(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithScriptRand(draws.rand),
			gameservertest.WithNPCScripts(kinds, list, catalog),
			gameservertest.WithNpcSpawns(table),
			gameservertest.WithMakers(makers.catalog()),
			gameservertest.WithDoors(fhDoors()...),
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, def := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", def.ID, err)
				}
			}
		},
	}, templates...)
	return &frWorld{warriorWorld: w, draws: draws, makers: makers}
}

// alarmDoorsOpen reports which of the alarm's doors stand open.
func (w *frWorld) alarmDoorsOpen(t *testing.T) []bool {
	t.Helper()
	got := make([]bool, len(fhAlarmDoors))
	for i := range fhAlarmDoors {
		_, got[i] = w.srv.DoorState(t, fhDoorID(i))
	}
	return got
}

// TestAlarmDeviceSoundsAndDeactivates runs the hall alarm (18328): it
// shouts 1010630 as it appears, and script event 10025 sets the hall it
// guards. Dying in hall 1, 2 or 3 it draws against 10, 20 or 30 in 100;
// a hit, or hall 4 with no draw, shouts 1010631, runs maker event 1000 on
// the thirteen hall makers in order and opens grave pathways 1 and 2 and
// the eight center walls; a miss runs maker event 1001 on the hall's
// captain, self-destruct or guard maker only. Unset (0) it does nothing.
func TestAlarmDeviceSoundsAndDeactivates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hall  int32
		draws []int
		hit   bool
		miss  string
	}{
		{0, nil, false, ""},
		{1, []int{9}, true, ""},
		{1, []int{10}, false, "godard32_2515_07m1 1001"},
		{2, []int{19}, true, ""},
		{2, []int{20}, false, "godard32_2515_09m1 1001"},
		{3, []int{29}, true, ""},
		{3, []int{30}, false, "godard32_2515_06m1 1001"},
		{4, nil, true, ""},
	} {
		t.Run(fmt.Sprintf("hall %d draw %v", tc.hall, tc.draws), func(t *testing.T) {
			t.Parallel()
			draws := &ind1Draws{}
			draws.set(0, tc.draws...)
			w := bootTombHalls(t, draws)
			w.makers.take()
			alarm, frames := w.spawnSeen(t, fhAlarmID, location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
			if got, want := shouts(t, frames, alarm.ObjectID()), []string{npcstringText(t, 1010630)}; !slices.Equal(got, want) {
				t.Fatalf("shouts as the alarm appeared = %q, want %q", got, want)
			}
			if got := script.NPCOf(alarm).Int(script.SlotIntAI0); got != 0 {
				t.Fatalf("hall after the created hook = %d, want 0", got)
			}
			w.send(t, alarm, 10024, 3)
			if got := script.NPCOf(alarm).Int(script.SlotIntAI0); got != 0 {
				t.Fatalf("hall after event 10024 = %d, want 0", got)
			}
			w.send(t, alarm, 10025, tc.hall)
			if got := script.NPCOf(alarm).Int(script.SlotIntAI0); got != tc.hall {
				t.Fatalf("hall after event 10025 = %d, want %d", got, tc.hall)
			}
			w.srv.ReadQueued(t, w.srv.Client)
			w.onQueue(t, alarm, func() { script.NPCOf(alarm).DoDie(nil) })
			w.srv.Advance(t, 3*time.Second)
			frames = w.srv.ReadQueued(t, w.srv.Client)

			var wantShouts, wantMakers []string
			wantDoors := make([]bool, len(fhAlarmDoors))
			if tc.hit {
				wantShouts = []string{npcstringText(t, 1010631)}
				for _, m := range fhAlarmMakers {
					wantMakers = append(wantMakers, m+" 1000")
				}
				for i := range wantDoors {
					wantDoors[i] = true
				}
			} else if tc.miss != "" {
				wantMakers = []string{tc.miss}
			}
			if got := shouts(t, frames, alarm.ObjectID()); !slices.Equal(got, wantShouts) {
				t.Fatalf("shouts as it died = %q, want %q", got, wantShouts)
			}
			if got := w.makers.take(); !slices.Equal(got, wantMakers) {
				t.Fatalf("maker events as it died = %q, want %q", got, wantMakers)
			}
			if got := w.alarmDoorsOpen(t); !slices.Equal(got, wantDoors) {
				t.Fatalf("doors open after its death = %v, want %v", got, wantDoors)
			}
		})
	}
}

// npcstringText is the client text of NpcString id.
func npcstringText(t *testing.T, id int32) string {
	t.Helper()
	text, ok := npcstring.Text(id)
	if !ok {
		t.Fatalf("no npc string %d", id)
	}
	return text
}

// TestEvilatesRaiseTheirSpirits runs both portraits' created and timer
// hooks: 45 s after it appears, then every 20 s, EvilateA (29049) raises a
// portrait spirit (29050) as its private, four in all, and EvilateB
// (29048) a winged one (29051), two in all, each at the point and heading
// the portrait's SpawnPos and SpawnAngle AI parameters name.
func TestEvilatesRaiseTheirSpirits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, spirit int32
		count      int
	}{
		{fhEvilateAID, fhSpiritID, 4},
		{fhEvilateBID, fhWingedID, 2},
	} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			t.Parallel()
			w := bootTombHalls(t, &ind1Draws{})
			portrait := w.spawn(t, tc.id, 100, 0)
			spirits := func(when string, want int) {
				t.Helper()
				live := w.liveNPCs(tc.spirit)
				if len(live) != want {
					t.Fatalf("spirits %s = %d, want %d", when, len(live), want)
				}
				for _, s := range live {
					if frAt(s) != fhSpiritAt {
						t.Fatalf("spirit %s at %v, want %v", when, frAt(s), fhSpiritAt)
					}
					if m := s.Master(); m == nil || m.ObjectID() != portrait.ObjectID() {
						t.Fatalf("spirit %s has master %v, want the portrait", when, m)
					}
				}
			}
			w.srv.Advance(t, 45*time.Second-time.Millisecond)
			spirits("before 45 s", 0)
			w.srv.Advance(t, time.Millisecond)
			spirits("at 45 s", 1)
			for i := 2; i <= tc.count; i++ {
				w.srv.Advance(t, 20*time.Second-time.Millisecond)
				spirits(fmt.Sprintf("before spirit %d", i), i-1)
				w.srv.Advance(t, time.Millisecond)
				spirits(fmt.Sprintf("at spirit %d", i), i)
			}
			w.srv.Advance(t, time.Minute)
			spirits("a minute after the last", tc.count)
		})
	}
}

// TestEvilateFallsToTheDewdrop runs both portraits' attacked hook: a hit
// with skill 2276 kills the portrait; a hit with another skill, or a plain
// hit, leaves it alive.
func TestEvilateFallsToTheDewdrop(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{fhEvilateAID, fhEvilateBID} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootTombHalls(t, &ind1Draws{})
			portrait := w.spawn(t, id, 100, 0)
			w.hit(t, portrait, 1)
			if portrait.Dead() {
				t.Fatal("the portrait died to a plain hit")
			}
			w.selectTarget(t, portrait)
			w.cast(t, fhOtherID)
			if portrait.Dead() {
				t.Fatal("the portrait died to skill 1177")
			}
			if portrait.CurrentHP() >= portrait.MaxHP()-1 {
				t.Fatalf("skill 1177 did not land: HP %d of %d", portrait.CurrentHP(), portrait.MaxHP())
			}
			w.cast(t, fhDewdropID)
			if !portrait.Dead() {
				t.Fatal("the portrait is alive after skill 2276")
			}
		})
	}
}

// TestEvilateSpiritsStayDead runs EvilateA's party-died hook: a spirit
// dying while its portrait lives asks no respawn, and neither does one
// dying after the portrait died, which let its spirits go.
func TestEvilateSpiritsStayDead(t *testing.T) {
	t.Parallel()
	w := bootTombHalls(t, &ind1Draws{})
	portrait := w.spawn(t, fhEvilateAID, 100, 0)
	w.srv.Advance(t, 65*time.Second)
	spirits := w.liveNPCs(fhSpiritID)
	if len(spirits) != 2 {
		t.Fatalf("spirits at 65 s = %d, want 2", len(spirits))
	}
	armed := func(when string) {
		t.Helper()
		for i := 1; i <= 64; i++ {
			if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
				t.Fatalf("%s: private respawn %s pending, want none", when, key)
			}
		}
	}
	w.kill(t, spirits[0])
	armed("a spirit died beside its live portrait")
	w.kill(t, portrait)
	w.kill(t, spirits[1])
	armed("a spirit died after its portrait")
}
