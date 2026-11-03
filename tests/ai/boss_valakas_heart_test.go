package ai

import (
	"context"
	"fmt"
	"maps"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/valakas"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The Heart of Volcano and its spawn_data row.
const (
	volcanoID  = int32(31385)
	volcanoRow = "heart_of_volcano"
)

// volcanoHome is the Heart's shipped spot; the character enters the world
// 100 due west of it.
var (
	volcanoHome      = location.Location{X: 189872, Y: -105152, Z: -724}
	volcanoCharacter = location.Location{X: 189772, Y: -105152, Z: -724}
)

// volcanoSpawnlist is the Heart's shipped territory and database-tracked
// npcmaker, with its TelPos parameters.
const volcanoSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="godard10_npc2514_07" minZ="-804" maxZ="-604">
		<node x="189772" y="-105236"/>
		<node x="189972" y="-105236"/>
		<node x="189972" y="-105036"/>
		<node x="189772" y="-105036"/>
	</territory>
	<npcmaker name="godard10_npc2514_0701" territory="godard10_npc2514_07" maximumNpcs="1">
		<ai type="default_use_db_maker"/>
		<npc id="31385" pos="189872;-105152;-724;49000" total="1" respawn="1min" dbName="heart_of_volcano">
			<ai>
				<set name="TelPosX" val="204187"/>
				<set name="TelPosY" val="-111864"/>
				<set name="TelPosZ" val="34"/>
			</ai>
		</npc>
	</npcmaker>
</list>`

// volcanoPages are the Heart's four pages, each naming itself, the first
// with the shipped page's entry link.
func volcanoPages() map[string]string {
	pages := map[string]string{}
	for i := 1; i <= 4; i++ {
		name := fmt.Sprintf("heart_of_volcano%03d.htm", i)
		pages["script/ai/boss/valakas/HeartOfVolcano/"+name] = "<html><body>" + name + "</body></html>"
	}
	pages["script/ai/boss/valakas/HeartOfVolcano/heart_of_volcano001.htm"] = `<html><body>heart_of_volcano001.htm<a action="bypass -h Quest HeartOfVolcano enter">Enter</a></body></html>`
	return pages
}

// bootVolcano boots the Heart and Valakas's lair backed by the spawn_data
// table, after saving rows, Valakas respawning after 1 hour, and the
// character entering the world beside the Heart.
func bootVolcano(t *testing.T, rows map[string]*spawn.State) *baiumWorld {
	t.Helper()
	if len(rows) > 0 {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), rows); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	w := &baiumWorld{start: time.Now()}
	dir := t.TempDir()
	for name, text := range map[string]string{"valakas.xml": fmt.Sprintf(valakasSpawnlist, "1hour"), "heart.xml": volcanoSpawnlist} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := warriorTemplate(valakasID, nil)
	boss.Type, boss.Name, boss.Level, boss.HPMax = "GrandBoss", "Valakas", 85, 1000000
	lavasaurus := warriorTemplate(valakasPrivateID, nil)
	lavasaurus.Name = "Lavasaurus Elder"
	templates := []*npc.Template{boss, lavasaurus, gameservertest.FolkTemplate("Folk", int(valakasCubeID)), gameservertest.FolkTemplate("Folk", int(volcanoID))}
	kinds := map[int32]script.NPCKind{valakasID: script.KindHostile, valakasPrivateID: script.KindHostile, valakasCubeID: script.KindFolk, volcanoID: script.KindFolk}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.valakas.HeartOfVolcano"},
		{Path: "script.ai.boss.valakas.Valakas"},
		{Path: "test." + valakasDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":       defaultnpc.New,
		"script.ai.boss.valakas.HeartOfVolcano": valakas.HeartOfVolcano,
		"script.ai.boss.valakas.Valakas":        valakas.Valakas,
		"test." + valakasDirector:               func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{volcanoCharacter}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(valakasSkills()),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(valakasQuietDraws),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
		gameservertest.WithHTMLPages(volcanoPages()),
	)
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
	return w
}

// volcanoHeartRow is a live spawn_data row of the Heart at its spot holding
// value.
func volcanoHeartRow(value int) *spawn.State {
	return &spawn.State{Name: volcanoRow, Status: spawn.StatusAlive, CurrentHP: 2444, CurrentMP: 1000, Location: volcanoHome, DBValue: value}
}

// volcanoDeadValakas is a spawn_data row of Valakas dead for its whole
// respawn.
func volcanoDeadValakas() *spawn.State {
	return &spawn.State{Name: valakasRow, Status: spawn.StatusDead, RespawnTime: time.Now().Add(100 * time.Hour).UnixMilli()}
}

// volcanoState is what the live Heart holds: whether its row was loaded,
// its db value and its entrant count; and Valakas's db value, -1 with no
// Valakas in the world.
type volcanoState struct {
	loaded         bool
	value, count   int
	valakasDBValue int
}

// volcanoNow reads the live Heart and Valakas on the character's queue.
func volcanoNow(t *testing.T, w *baiumWorld) volcanoState {
	t.Helper()
	heart := script.NPCOf(soleBaiumNPC(t, w, volcanoID))
	var out volcanoState
	w.srv.RunScript(t, w.player.ObjectID(), valakasDirector, func(*script.Script, *script.Player) {
		out = volcanoState{loaded: heart.DBLoaded(), value: heart.DBValue(), count: int(heart.Int(script.SlotIntAI0)), valakasDBValue: -1}
		for _, c := range baiumNPCs(w, valakasID) {
			out.valakasDBValue = script.NPCOf(c).DBValue()
		}
	})
	return out
}

// volcanoSaved shuts the server down and returns the db values saved with
// the Heart's and Valakas's spawn_data rows, by row name.
func volcanoSaved(t *testing.T, w *baiumWorld) map[string]int {
	t.Helper()
	w.srv.Shutdown(t)
	states, err := gamesql.NewSpawnStore(w.srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, row := range []string{volcanoRow, valakasRow} {
		if state, ok := states[row]; ok {
			out[row] = state.DBValue
		}
	}
	return out
}

// enterVolcano talks to the Heart, which answers its first page, follows
// the page's entry link and returns the frames of the answer.
func enterVolcano(t *testing.T, w *baiumWorld) [][]byte {
	t.Helper()
	if got := baiumPageNames(w.talkTo(t, soleBaiumNPC(t, w, volcanoID))); !slices.Equal(got, []string{"heart_of_volcano001.htm"}) {
		t.Fatalf("the Heart's pages = %q, want its first page", got)
	}
	w.srv.Client.Send(encodeManagerBypass("Quest HeartOfVolcano enter"))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// TestHeartOfVolcanoKeepsItsCount ports the Heart's creation (phase 2): a
// fresh spawn stores db value 0 and counts 0 entrants; one restored from
// its live row counts the entrants its row holds. Each saves its value.
func TestHeartOfVolcanoKeepsItsCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows map[string]*spawn.State
		want volcanoState
	}{
		{"fresh", nil, volcanoState{loaded: false, value: 0, count: 0, valakasDBValue: 0}},
		{"restored", map[string]*spawn.State{volcanoRow: volcanoHeartRow(7)}, volcanoState{loaded: true, value: 7, count: 7, valakasDBValue: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootVolcano(t, tc.rows)
			if got := volcanoNow(t, w); got != tc.want {
				t.Fatalf("the Heart = %+v, want %+v", got, tc.want)
			}
			if got, want := volcanoSaved(t, w), map[string]int{volcanoRow: tc.want.value, valakasRow: 0}; !maps.Equal(got, want) {
				t.Fatalf("saved db values = %v, want %v", got, want)
			}
		})
	}
}

// TestHeartOfVolcanoLetsACharacterIn ports the entry (phase 2): with
// Valakas asleep (db value 0) or waiting (1) and fewer than 200 entrants,
// the entry teleports the character to the Heart's TelPos 204187, -111864,
// 34 with no page and counts it, saving the count as the Heart's db value.
// Asleep, Valakas receives script event 0 and stores 1.
func TestHeartOfVolcanoLetsACharacterIn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    map[string]*spawn.State
		count   int
		valakas int
	}{
		{"asleep", nil, 1, 1},
		{"waiting", map[string]*spawn.State{valakasRow: valakasLiveRow(1)}, 1, 1},
		{"last_place", map[string]*spawn.State{volcanoRow: volcanoHeartRow(199)}, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootVolcano(t, tc.rows)
			frames := enterVolcano(t, w)
			if got := baiumPageNames(frames); len(got) != 0 {
				t.Fatalf("the entry's pages = %q, want none", got)
			}
			if got, want := w.characterTeleports(t, frames), []string{"S TeleportToLocation obj=player loc=204187,-111864,34 fast=0"}; !slices.Equal(got, want) {
				t.Fatalf("the entry's teleports = %q, want %q", got, want)
			}
			w.appear(t)
			if got := volcanoNow(t, w); got.count != tc.count || got.value != tc.count || got.valakasDBValue != tc.valakas {
				t.Fatalf("the Heart = %+v, want count and value %d, Valakas %d", got, tc.count, tc.valakas)
			}
			if got, want := volcanoSaved(t, w), map[string]int{volcanoRow: tc.count, valakasRow: tc.valakas}; !maps.Equal(got, want) {
				t.Fatalf("saved db values = %v, want %v", got, want)
			}
		})
	}
}

// TestHeartOfVolcanoRefuses ports the entry's refusals (phase 2), each
// keeping the character in place and the Heart's count: 200 entrants
// answer heart_of_volcano004.htm, though asleep Valakas still receives
// script event 0 and stores 1; Valakas past its wait (db value 2 or 3)
// heart_of_volcano003.htm; no Valakas in the world (dead, its memo key
// unset) heart_of_volcano002.htm; Valakas at a value the Heart knows no
// answer for answers nothing.
func TestHeartOfVolcanoRefuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    map[string]*spawn.State
		want    []string
		count   int
		valakas int
	}{
		{"full", map[string]*spawn.State{volcanoRow: volcanoHeartRow(200)}, []string{"heart_of_volcano004.htm"}, 200, 1},
		{"moving_in", map[string]*spawn.State{valakasRow: valakasLiveRow(2)}, []string{"heart_of_volcano003.htm"}, 0, 2},
		{"fighting", map[string]*spawn.State{valakasRow: valakasLiveRow(3)}, []string{"heart_of_volcano003.htm"}, 0, 3},
		{"dead", map[string]*spawn.State{valakasRow: volcanoDeadValakas()}, []string{"heart_of_volcano002.htm"}, 0, -1},
		{"unknown_value", map[string]*spawn.State{valakasRow: valakasLiveRow(4)}, nil, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootVolcano(t, tc.rows)
			frames := enterVolcano(t, w)
			if got := baiumPageNames(frames); !slices.Equal(got, tc.want) {
				t.Fatalf("the entry's pages = %q, want %q", got, tc.want)
			}
			if got := w.characterTeleports(t, frames); len(got) != 0 {
				t.Fatalf("the entry's teleports = %q, want none", got)
			}
			if got := volcanoNow(t, w); got.count != tc.count || got.value != tc.count || got.valakasDBValue != tc.valakas {
				t.Fatalf("the Heart = %+v, want count and value %d, Valakas %d", got, tc.count, tc.valakas)
			}
		})
	}
}

// TestHeartOfVolcanoForgetsItsCount ports the Heart's minute timer (phase
// 2): every 60 s from its creation, a Heart counting entrants keeps its
// count while Valakas lives past its sleep (db value not 0), and otherwise
// (Valakas asleep again or gone) stores db value 0 and counts afresh.
func TestHeartOfVolcanoForgetsItsCount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		valakas *spawn.State
		want    int
	}{
		{"waiting", valakasLiveRow(1), 5},
		{"asleep", nil, 0},
		{"dead", volcanoDeadValakas(), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string]*spawn.State{volcanoRow: volcanoHeartRow(5)}
			if tc.valakas != nil {
				rows[valakasRow] = tc.valakas
			}
			w := bootVolcano(t, rows)
			w.advance(t, time.Minute-time.Millisecond)
			if got := volcanoNow(t, w); got.count != 5 || got.value != 5 {
				t.Fatalf("the Heart a minute less 1 ms on = %+v, want count and value 5", got)
			}
			w.advance(t, time.Millisecond)
			if got := volcanoNow(t, w); got.count != tc.want || got.value != tc.want {
				t.Fatalf("the Heart a minute on = %+v, want count and value %d", got, tc.want)
			}
			if got := volcanoSaved(t, w)[volcanoRow]; got != tc.want {
				t.Fatalf("saved db value = %d, want %d", got, tc.want)
			}
		})
	}
}
