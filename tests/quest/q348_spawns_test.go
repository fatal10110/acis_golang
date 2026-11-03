package quest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q348"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The ark guardians of An Arrogant Search (Q348_AnArrogantSearch onTalk,
// onCreated, onAttacked, onMyDying, onDecayed): the guardian corpse and the
// holy arks call the Angel Killer, Ark Guardian Elberoth and Shadowfang
// one at a time, each speaks as it appears and as it is first hit, and the
// guardians give their keys as they die. A scenario cannot show a
// script-spawned NPC appearing, nor act on one (#4020 item 5), cannot show a
// kill's fresh monster speaking as it appears in the step of the kill, and
// its fixture monster has other hit points in the reference probe and in the
// run, so the Angel Killer's health threshold cannot be shared between them:
// these Boot tests pin the calls, the lines and the keys; the scenarios play
// the dialogs, the items and the Platinum Tribe.
const (
	q348Name = "Q348_AnArrogantSearch"
	q348Path = "quest.Q348_AnArrogantSearch"

	q348Corpse     = 30980
	q348Ark2       = 30978
	q348Ark3       = 30979
	q348Elberoth   = 27182
	q348ShadowFang = 27183
	q348Angel      = 27184

	q348FirstKey   = 4291
	q348SecondKey  = 4292
	q348ThirdKey   = 4293
	q348BloodSaint = 4398
	q348BookSaint  = 4397
	q348Fabric     = 4295
)

// q348World boots a character of level 60 whose quest is STARTED at cond
// (none when cond is 0) with the quest variables rows, holding holds, with
// the guardian corpse and the second and third holy arks standing 40, 50 and
// 60 east of it as "corpse", "ark2" and "ark3". The script random answers
// draws.
func q348World(t *testing.T, draws *scriptDraws, cond int, rows map[string]string, holds map[int32]int32) *dialogWorld {
	t.Helper()
	quest := q348.New()
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, id := range quest.Bind[script.EventTalked] {
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
		kinds[id] = script.KindFolk
	}
	monsters := slices.Concat(quest.Bind[script.EventMyDying], quest.Bind[script.EventAttacked], quest.Bind[script.EventCreated])
	for _, id := range monsters {
		if _, ok := kinds[id]; ok {
			continue
		}
		templates = append(templates, proofMonster(int(id)))
		kinds[id] = script.KindHostile
	}
	pages := map[string][]string{}
	for _, p := range []string{"30980-01.htm", "30980-02.htm", "30980-03.htm", "30978-01.htm", "30978-02.htm", "30978-03.htm", "30979-01.htm", "30979-02.htm", "30979-03.htm"} {
		pages[p] = nil
	}
	w := bootSeamQuest(t, 60, chatPages(questPages(q348Name, pages), q348Corpse, q348Ark2, q348Ark3), func(srv *gameservertest.Server, objID int32) {
		if cond > 0 {
			insertJournal(t, srv.DB,
				journalRow{objID, q348Name, "<state>", val("STARTED")},
				journalRow{objID, q348Name, "<cond>", val(strconv.Itoa(cond))})
		}
		for k, v := range rows {
			insertJournal(t, srv.DB, journalRow{objID, q348Name, k, val(v)})
		}
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items(append(slices.Clone(quest.Items), q348Fabric)),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q348Path}}, script.Catalog{q348Path: q348.New}),
		gameservertest.WithScriptRand(draws.rand),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "corpse", q348Corpse, 40, 0, 0)
	w.spawnFolk(t, "ark2", q348Ark2, 50, 0, 0)
	w.spawnFolk(t, "ark3", q348Ark3, 60, 0, 0)
	return w
}

// q348Said is the rendered NpcSay of the script-spawned n.
func q348Said(n *script.NPC, id int32) string {
	return say(strconv.Itoa(int(n.ObjectID())), n.NpcID(), id)
}

// q348Lines renders what the character was sent since the last read: the
// NPC lines and the radar's markers.
func (w *dialogWorld) q348Lines(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		if f[0] != serverpackets.OpcodeNpcSay && f[0] != serverpackets.OpcodeRadarControl {
			continue
		}
		line, err := scriptcontract.Packet(f, w.roles)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// q348Count checks that count NPCs of id stand in the world.
func q348Count(t *testing.T, w *dialogWorld, step string, id int32, count int) []*script.NPC {
	t.Helper()
	got := npcsOf(w.srv, id)
	if len(got) != count {
		t.Fatalf("%s: %d NPCs %d, want %d", step, len(got), id, count)
	}
	return got
}

// q348Row asserts the quest variable key.
func q348Row(t *testing.T, w *dialogWorld, key, want string) {
	t.Helper()
	if got := questRows(t, w.srv, w.player, q348Name)[key]; got != want {
		t.Fatalf("quest variable %s = %q, want %q", key, got, want)
	}
}

// q348Items asserts the character's count of item.
func q348Items(t *testing.T, w *dialogWorld, item int32, want int) {
	t.Helper()
	if n := w.srv.PlayerItemCount(t, w.player, item); n != want {
		t.Fatalf("item %d held %d times, want %d", item, n, want)
	}
}

// TestQ348CallsOneGuardianAtATime: the guardian corpse at the first stage of
// the search for the first key, and the second and third holy arks with
// their keys not yet held, call the Angel Killer, Elberoth and Shadowfang
// within 100 of them; each says its line as it appears, and a second talk
// calls no other while it stands. It is gone after ten minutes,
// and the next talk calls another.
func TestQ348CallsOneGuardianAtATime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		role     string
		npcID    int32
		guardian int32
		page     string
		cond     int
		said     int32
	}{
		{"corpse", "corpse", q348Corpse, q348Angel, "30980-01.htm", 18, 34831},
		{"second ark", "ark2", q348Ark2, q348Elberoth, "30978-01.htm", 5, 34837},
		{"third ark", "ark3", q348Ark3, q348ShadowFang, "30979-01.htm", 5, 34838},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q348World(t, &scriptDraws{}, 5, nil, nil)
			got := w.talk(t, tc.role)
			if !shows(got, tc.page) {
				t.Fatalf("talk: got %q, want %s", got, tc.page)
			}
			called := q348Count(t, w, "after the talk", tc.guardian, 1)
			wantLines(t, "the call", got, q348Said(called[0], tc.said))
			if loc := called[0].SpawnLocation(); loc.X < w.at.X+40-100 || loc.X > w.at.X+60+100 || loc.Y < w.at.Y-100 || loc.Y > w.at.Y+100 {
				t.Fatalf("the guardian stands at %+v, want within 100 of the ark beside %+v", loc, w.at)
			}
			q348Row(t, w, "<cond>", strconv.Itoa(tc.cond))

			got = w.talk(t, tc.role)
			if !shows(got, tc.page) {
				t.Fatalf("second talk: got %q, want %s", got, tc.page)
			}
			saysNone(t, "second talk", got)
			q348Count(t, w, "after a second talk", tc.guardian, 1)

			w.advance(t, 599*time.Second)
			q348Count(t, w, "599 s after the call", tc.guardian, 1)
			w.advance(t, 2*time.Second)
			q348Count(t, w, "after the ten minutes", tc.guardian, 0)

			got = w.talk(t, tc.role)
			again := q348Count(t, w, "after the next talk", tc.guardian, 1)
			wantLines(t, "the next call", got, q348Said(again[0], tc.said))
		})
	}
}

// TestQ348ArksCallNothingOnceTheKeyIsHeld: a holy ark whose key the character
// holds turns it in for the relic instead of calling its guardian.
func TestQ348ArksCallNothingOnceTheKeyIsHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		role     string
		guardian int32
		key      int32
		relic    int32
		page     string
		cond     string
	}{
		{"second ark", "ark2", q348Elberoth, q348SecondKey, q348BookSaint, "30978-02.htm", "12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q348World(t, &scriptDraws{}, 11, nil, map[int32]int32{tc.key: 1})
			got := w.talk(t, tc.role)
			if !shows(got, tc.page) {
				t.Fatalf("talk: got %q, want %s", got, tc.page)
			}
			q348Count(t, w, "after the talk", tc.guardian, 0)
			q348Items(t, w, tc.key, 0)
			q348Items(t, w, tc.relic, 1)
			q348Row(t, w, "<cond>", tc.cond)
		})
	}
}

// TestQ348GuardiansSpeakAtTheFirstHit: Elberoth and Shadowfang say their
// line at the first hit of a character whose quest is started, and not
// again; a hit of a character with no quest is not answered.
func TestQ348GuardiansSpeakAtTheFirstHit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		id    int32
		cond  int
		said  int32
		first bool
	}{
		{"Elberoth", q348Elberoth, 5, 34833, true},
		{"Shadowfang", q348ShadowFang, 5, 34836, true},
		{"Elberoth without quest", q348Elberoth, 0, 34833, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q348World(t, &scriptDraws{}, tc.cond, nil, nil)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(tc.id)), location.Location{X: w.at.X + 80, Y: w.at.Y, Z: w.at.Z})
			n := q348Count(t, w, "spawned", tc.id, 1)[0]
			hit(t, w.srv, w.player, h, 5)
			got := w.advance(t, 500*time.Millisecond)
			if !tc.first {
				saysNone(t, "hit without quest", got)
				return
			}
			wantLines(t, "first hit", got, q348Said(n, tc.said))
			hit(t, w.srv, w.player, h, 5)
			saysNone(t, "second hit", w.advance(t, 500*time.Millisecond))
		})
	}
}

// TestQ348GuardiansGiveTheirKeys: Elberoth and Shadowfang killed at the
// stages of the search (5 to 22) by a character without their key say their
// dying line and give the key; at another stage, or with the key held,
// nothing happens.
func TestQ348GuardiansGiveTheirKeys(t *testing.T) {
	t.Parallel()
	for _, g := range []struct {
		name  string
		id    int32
		key   int32
		cond  string
		said  int32
		other int32
	}{
		{"Elberoth", q348Elberoth, q348SecondKey, "11", 34832, q348ThirdKey},
		{"Shadowfang", q348ShadowFang, q348ThirdKey, "15", 34835, q348SecondKey},
	} {
		for _, tc := range []struct {
			name  string
			cond  int
			holds map[int32]int32
			given bool
		}{
			{"first stage of the search", 5, nil, true},
			{"last stage of the search", 22, nil, true},
			{"before the search", 4, nil, false},
			{"after the search", 23, nil, false},
			{"key held", 5, map[int32]int32{g.key: 1}, false},
		} {
			t.Run(g.name+" "+tc.name, func(t *testing.T) {
				t.Parallel()
				w := q348World(t, &scriptDraws{}, tc.cond, nil, tc.holds)
				h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(g.id)), location.Location{X: w.at.X + 80, Y: w.at.Y, Z: w.at.Z})
				n := q348Count(t, w, "spawned", g.id, 1)[0]
				killOnPlayerQueue(t, w.srv, w.player, h)
				got := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
				held := 0
				if tc.holds != nil {
					held = 1
				}
				if !tc.given {
					if slices.Contains(got, q348Said(n, g.said)) {
						t.Fatalf("the death: got %q, want no dying line", got)
					}
					q348Items(t, w, g.key, held)
					q348Row(t, w, "<cond>", strconv.Itoa(tc.cond))
					return
				}
				wantLines(t, "the death", got, q348Said(n, g.said))
				q348Items(t, w, g.key, 1)
				q348Row(t, w, "<cond>", g.cond)
			})
		}
	}
}

// TestQ348AngelKillerLeaves: a hit that finds the Angel Killer below 30% of
// its health, by a character that holds neither the first key nor the blood
// of the saint and has not beaten it, makes it say it is beaten and leave,
// sends the character to the corpse and moves the quest to stage 19; at 30%
// or more it does nothing. A hit by a character that holds a key or has
// beaten it makes it say its farewell and leave whatever its health, and
// changes nothing.
func TestQ348AngelKillerLeaves(t *testing.T) {
	t.Parallel()
	const corpseMarker = "S RadarControl show=2 type=2 loc=-2908,44128,-2712"
	for _, tc := range []struct {
		name  string
		rows  map[string]string
		holds map[int32]int32
		hp    int // per thousand of its health when the hit of 1 finds it
		said  int32
		cond  string
	}{
		{"above 30%", nil, nil, 301, 0, "18"},
		{"exactly 30%", nil, nil, 300, 0, "18"},
		{"below 30%", nil, nil, 299, 34830, "19"},
		{"low health", nil, nil, 10, 34830, "19"},
		{"key held", nil, map[int32]int32{q348FirstKey: 1}, 1000, 34839, "18"},
		{"blood held", nil, map[int32]int32{q348BloodSaint: 1}, 1000, 34839, "18"},
		{"already beaten", map[string]string{"angelkiller": "1"}, nil, 1000, 34839, "18"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q348World(t, &scriptDraws{}, 18, tc.rows, tc.holds)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q348Angel), location.Location{X: w.at.X + 80, Y: w.at.Y, Z: w.at.Z})
			n := q348Count(t, w, "spawned", q348Angel, 1)[0]
			onPlayerQueue(t, w.srv, w.player, func() { h.SetCurrentHP(h.MaxHP() * tc.hp / 1000) })
			hit(t, w.srv, w.player, h, 1)
			w.srv.Advance(t, 500*time.Millisecond)
			got := w.q348Lines(t)
			if tc.said == 0 {
				saysNone(t, "the hit", got)
				q348Count(t, w, "after the hit", q348Angel, 1)
				q348Row(t, w, "<cond>", tc.cond)
				return
			}
			wantLines(t, "the hit", got, q348Said(n, tc.said))
			q348Count(t, w, "after the hit", q348Angel, 0)
			q348Row(t, w, "<cond>", tc.cond)
			beaten := tc.said == 34830
			if beaten {
				q348Row(t, w, "angelkiller", "1")
				wantLines(t, "the marker", got, corpseMarker)
			} else if slices.Contains(got, corpseMarker) {
				t.Fatalf("the farewell: got %q, want no marker", got)
			}
		})
	}
}
