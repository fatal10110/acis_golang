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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/antharas"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The Heart of Warding, the Portal Stone it takes, and the three Antharas
// sizes it wakes, by their spawn_data rows.
const (
	heartID          = int32(13001)
	heartPortalStone = int32(3865)
	heartDirector    = "HeartDirector"
)

var heartSizes = []struct {
	id  int32
	row string
}{
	{29066, "antaras_min"},
	{29067, "antaras_normal"},
	{29068, "antaras_max"},
}

// heartCharacter is where the character enters the world: 100 due west of
// the Heart, at its shipped spot.
var heartCharacter = location.Location{X: 154595, Y: 121097, Z: -3757}

// heartSpawnlist is the lair's npcmaker with the three Antharas sizes under
// global memo keys 4, 5 and 6, the keys the Heart reads, the exit cubes'
// maker, and the Heart's shipped maker.
const heartSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="antaras_2521_01" minZ="-7712" maxZ="-6212">
		<node x="175201" y="110789"/>
		<node x="179519" y="110645"/>
		<node x="187612" y="113814"/>
		<node x="187652" y="115788"/>
		<node x="179659" y="118982"/>
		<node x="175206" y="118934"/>
		<node x="173003" y="114720"/>
	</territory>
	<npcmaker name="antaras_2521_01" territory="antaras_2521_01" maximumNpcs="89">
		<ai type="default_use_db_maker">
			<set name="maker_name" val="antaras_2521_02"/>
		</ai>
		<npc id="29066" pos="184500;114835;-7678;0" total="1" respawn="264hour" respawnRand="0sec" dbName="antaras_min" dbSaving="DEATH_TIME;PARAMETERS;POS">
			<ai>
				<set name="GM_ID" val="4"/>
			</ai>
		</npc>
		<npc id="29067" pos="184500;114835;-7678;0" total="1" respawn="264hour" respawnRand="0sec" dbName="antaras_normal" dbSaving="DEATH_TIME;PARAMETERS;POS">
			<ai>
				<set name="GM_ID" val="5"/>
			</ai>
		</npc>
		<npc id="29068" pos="184500;114835;-7678;0" total="1" respawn="264hour" respawnRand="0sec" dbName="antaras_max" dbSaving="DEATH_TIME;PARAMETERS;POS">
			<ai>
				<set name="GM_ID" val="6"/>
			</ai>
		</npc>
	</npcmaker>
	<npcmaker name="antaras_2521_02" territory="antaras_2521_01" maximumNpcs="20">
		<ai type="default_maker">
			<set name="on_start_spawn" val="0"/>
		</ai>
		<npc id="31859" pos="177615;114941;-7709;0" total="20" respawn="no"/>
	</npcmaker>
	<territory name="giran04_npc2421_03" minZ="-3856" maxZ="-3556">
		<node x="154556" y="120988"/>
		<node x="154756" y="120988"/>
		<node x="154756" y="121188"/>
		<node x="154556" y="121188"/>
	</territory>
	<npcmaker name="giran04_npc2421_0301" territory="giran04_npc2421_03" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="13001" pos="154695;121097;-3757;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// heartPages are the Heart's four pages, each naming itself, the first
// with the shipped page's entry link.
func heartPages() map[string]string {
	pages := map[string]string{}
	for i := 1; i <= 4; i++ {
		name := fmt.Sprintf("13001-%02d.htm", i)
		pages["script/ai/boss/antharas/HeartOfWarding/"+name] = "<html><body>" + name + "</body></html>"
	}
	pages["script/ai/boss/antharas/HeartOfWarding/13001-01.htm"] = `<html><body>13001-01.htm<a action="bypass -h Quest HeartOfWarding enter">Enter</a></body></html>`
	return pages
}

// heartItems is the fixture catalog with the Portal Stone.
func heartItems() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: heartPortalStone, Name: "Portal Stone", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	}))
}

// bootHeart boots the Heart and Antharas's lair backed by the spawn_data
// table, after saving rows as live or dead rows of the Antharas sizes,
// every script draw 0, and the character entering the world beside the
// Heart holding stones Portal Stones.
func bootHeart(t *testing.T, rows map[string]*spawn.State, stones int32) *baiumWorld {
	t.Helper()
	if len(rows) > 0 {
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), rows); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	w := &baiumWorld{start: time.Now()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "heart.xml"), []byte(heartSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	templates := []*npc.Template{gameservertest.FolkTemplate("Folk", int(heartID)), gameservertest.FolkTemplate("Folk", int(antharasCubeID))}
	kinds := map[int32]script.NPCKind{heartID: script.KindFolk, antharasCubeID: script.KindFolk}
	for _, size := range heartSizes {
		boss := warriorTemplate(size.id, nil)
		boss.Type, boss.Name, boss.Level, boss.HPMax = "GrandBoss", "Antharas", 79, 1000000
		templates = append(templates, boss)
		kinds[size.id] = script.KindHostile
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.antharas.Antharas"},
		{Path: "script.ai.boss.antharas.HeartOfWarding"},
		{Path: "test." + heartDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":        defaultnpc.New,
		"script.ai.boss.antharas.Antharas":       antharas.Antharas,
		"script.ai.boss.antharas.HeartOfWarding": antharas.HeartOfWarding,
		"test." + heartDirector:                  func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{heartCharacter}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
		gameservertest.WithHTMLPages(heartPages()),
		gameservertest.WithItemTemplates(heartItems()),
	)
	if stones > 0 {
		srv.GiveItem(t, srv.SoleObjectID(t), heartPortalStone, stones)
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
	return w
}

// heartRow is a spawn_data row of the Antharas size row: alive at its home
// holding value, or dead for its whole respawn when value is -1.
func heartRow(row string, value int) *spawn.State {
	if value == -1 {
		return &spawn.State{Name: row, Status: spawn.StatusDead, RespawnTime: time.Now().Add(100 * time.Hour).UnixMilli()}
	}
	return &spawn.State{Name: row, Status: spawn.StatusAlive, CurrentHP: 1000000, CurrentMP: 1 << 20, Location: antharasHome, DBValue: value}
}

// heartNPC returns the script handle of the sole live Heart.
func heartNPC(t *testing.T, w *baiumWorld) *script.NPC {
	t.Helper()
	return script.NPCOf(soleBaiumNPC(t, w, heartID))
}

// heartDBValues returns the db value each live Antharas size holds, by id.
func heartDBValues(t *testing.T, w *baiumWorld) map[int32]int {
	t.Helper()
	out := map[int32]int{}
	w.srv.RunScript(t, w.player.ObjectID(), heartDirector, func(*script.Script, *script.Player) {
		for _, size := range heartSizes {
			for _, c := range baiumNPCs(w, size.id) {
				out[size.id] = script.NPCOf(c).DBValue()
			}
		}
	})
	return out
}

// heartCounts returns the Heart's wait mark and entrant count.
func heartCounts(t *testing.T, w *baiumWorld) (waiting, entrants int32) {
	t.Helper()
	n := heartNPC(t, w)
	w.srv.RunScript(t, w.player.ObjectID(), heartDirector, func(*script.Script, *script.Player) {
		waiting, entrants = n.Int(script.SlotIntAI0), n.Int(script.SlotIntAI1)
	})
	return waiting, entrants
}

// enterHeart talks to the Heart, which answers its first page, follows the
// page's entry link and returns the frames of the answer.
func enterHeart(t *testing.T, w *baiumWorld) [][]byte {
	t.Helper()
	if got := baiumPageNames(w.talkTo(t, soleBaiumNPC(t, w, heartID))); !slices.Equal(got, []string{"13001-01.htm"}) {
		t.Fatalf("the Heart's pages = %q, want its first page", got)
	}
	w.srv.Client.Send(encodeManagerBypass("Quest HeartOfWarding enter"))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// stonesHeld returns how many Portal Stones the character's saved items
// hold.
func stonesHeld(t *testing.T, w *baiumWorld) int {
	t.Helper()
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player.ObjectID())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row.TemplateID == heartPortalStone {
			n += row.Count
		}
	}
	return n
}

// heartSavedValues shuts the server down and returns the db value saved
// with each Antharas size's spawn_data row, by row name.
func heartSavedValues(t *testing.T, w *baiumWorld) map[string]int {
	t.Helper()
	w.srv.Shutdown(t)
	states, err := gamesql.NewSpawnStore(w.srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, size := range heartSizes {
		if state, ok := states[size.row]; ok {
			out[size.row] = state.DBValue
		}
	}
	return out
}

// TestHeartOfWardingLetsAStoneHolderIn ports the entry (phase 2): with the
// three Antharas asleep at db value 0, the first page's entry takes one
// Portal Stone, teleports the character into the lair at 179700, 113800,
// -7709 (draws 0) with no page, and starts the boss wait (30 minutes). One
// second short of it no Antharas stirs; at its end the Heart wakes the
// small one, its single entrant within 90, whose script event stores db
// value 1, and the Heart counts afresh. The saved rows hold 1 for the small
// size, 0 for the others.
func TestHeartOfWardingLetsAStoneHolderIn(t *testing.T) {
	w := bootHeart(t, nil, 2)
	frames := enterHeart(t, w)
	if got := baiumPageNames(frames); len(got) != 0 {
		t.Fatalf("the entry's pages = %q, want none", got)
	}
	if got, want := w.characterTeleports(t, frames), []string{"S TeleportToLocation obj=player loc=179700,113800,-7709 fast=0"}; !slices.Equal(got, want) {
		t.Fatalf("the entry's teleports = %q, want %q", got, want)
	}
	w.appear(t)
	if got := stonesHeld(t, w); got != 1 {
		t.Fatalf("Portal Stones held = %d, want 1", got)
	}
	if waiting, entrants := heartCounts(t, w); waiting != 1 || entrants != 1 {
		t.Fatalf("the Heart's wait mark and entrants = %d, %d, want 1, 1", waiting, entrants)
	}

	w.advance(t, 30*time.Minute-time.Second)
	if got, want := heartDBValues(t, w), map[int32]int{29066: 0, 29067: 0, 29068: 0}; !maps.Equal(got, want) {
		t.Fatalf("db values before the wait ends = %v, want %v", got, want)
	}
	w.advance(t, time.Second)
	if got, want := heartDBValues(t, w), map[int32]int{29066: 1, 29067: 0, 29068: 0}; !maps.Equal(got, want) {
		t.Fatalf("db values once the wait ends = %v, want %v", got, want)
	}
	if waiting, entrants := heartCounts(t, w); waiting != 0 || entrants != 0 {
		t.Fatalf("the Heart's wait mark and entrants after the wake = %d, %d, want 0, 0", waiting, entrants)
	}
	if got, want := heartSavedValues(t, w), map[string]int{"antaras_min": 1, "antaras_normal": 0, "antaras_max": 0}; !maps.Equal(got, want) {
		t.Fatalf("saved db values = %v, want %v", got, want)
	}
}

// TestHeartOfWardingWakesBySize ports the size the Heart wakes by its
// entrants (phase 2): with the count standing at the seed when the
// character enters, up to 90 entrants wake the small Antharas, up to 135
// the normal one, more the large one.
func TestHeartOfWardingWakesBySize(t *testing.T) {
	for _, tc := range []struct {
		seed  int32
		woken int32
	}{
		{89, 29066},
		{90, 29067},
		{134, 29067},
		{135, 29068},
	} {
		t.Run(fmt.Sprintf("%d_entrants", tc.seed+1), func(t *testing.T) {
			w := bootHeart(t, nil, 1)
			n := heartNPC(t, w)
			w.srv.RunScript(t, w.player.ObjectID(), heartDirector, func(*script.Script, *script.Player) {
				n.SetInt(script.SlotIntAI1, tc.seed)
			})
			enterHeart(t, w)
			w.appear(t)
			w.advance(t, 30*time.Minute)
			want := map[int32]int{29066: 0, 29067: 0, 29068: 0}
			want[tc.woken] = 1
			if got := heartDBValues(t, w); !maps.Equal(got, want) {
				t.Fatalf("db values = %v, want %v", got, want)
			}
		})
	}
}

// TestHeartOfWardingRefuses ports the entry's refusals (phase 2), each
// keeping the character's stones and place: no Portal Stone answers
// 13001-04.htm; an Antharas past its wake (db value 2 or 3) 13001-03.htm;
// an Antharas missing from the world (dead, its memo key unset)
// 13001-02.htm; an Antharas at a value the Heart knows no answer for
// answers nothing.
func TestHeartOfWardingRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rows   map[string]*spawn.State
		stones int32
		want   []string
	}{
		{"no_stone", nil, 0, []string{"13001-04.htm"}},
		{"moving_in", map[string]*spawn.State{"antaras_normal": heartRow("antaras_normal", 2)}, 1, []string{"13001-03.htm"}},
		{"fighting", map[string]*spawn.State{"antaras_max": heartRow("antaras_max", 3)}, 1, []string{"13001-03.htm"}},
		{"dead", map[string]*spawn.State{"antaras_min": heartRow("antaras_min", -1)}, 1, []string{"13001-02.htm"}},
		{"unknown_value", map[string]*spawn.State{"antaras_min": heartRow("antaras_min", 4)}, 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootHeart(t, tc.rows, tc.stones)
			frames := enterHeart(t, w)
			if got := baiumPageNames(frames); !slices.Equal(got, tc.want) {
				t.Fatalf("the entry's pages = %q, want %q", got, tc.want)
			}
			if got := w.characterTeleports(t, frames); len(got) != 0 {
				t.Fatalf("the entry's teleports = %q, want none", got)
			}
			if got := stonesHeld(t, w); got != int(tc.stones) {
				t.Fatalf("Portal Stones held = %d, want %d", got, tc.stones)
			}
			if waiting, entrants := heartCounts(t, w); waiting != 0 || entrants != 0 {
				t.Fatalf("the Heart's wait mark and entrants = %d, %d, want 0, 0", waiting, entrants)
			}
		})
	}
}
