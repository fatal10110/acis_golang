package scenario

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const eventsScenario = `character A 20
clock 00:49:57
npc owner 100 Folk 0 0 0
npc pet 200 Monster 10 0 0 ai TakeSocial=3,Other=x params character owner 7
event pet 10022 character 5
event pet 10022 owner 0
`

func TestParseReadsClockParamsAndEvent(t *testing.T) {
	sc, err := Parse("events.scenario", []byte(eventsScenario+"advance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !sc.HasClock || sc.Clock != 49*time.Minute+57*time.Second {
		t.Fatalf("clock = %v (%v), want 49m57s", sc.Clock, sc.HasClock)
	}
	pet := sc.NPCs[1]
	if pet.Params != [3]string{"character", "owner", "7"} || pet.AI["TakeSocial"] != "3" || pet.AI["Other"] != "x" {
		t.Fatalf("pet = %+v, want params character owner 7 and two AI parameters", pet)
	}
	if got := sc.Steps[0]; got.Verb != "event" || len(got.Args) != 4 {
		t.Fatalf("first step = %+v, want an event with four arguments", got)
	}
}

func TestParseRejectsBadClockParamsAndEvent(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"clock hour", "character A 1\nclock 24:00\nadvance 1s\n", "is not hh:mm"},
		{"clock twice", "character A 1\nclock 01:00\nclock 02:00\nadvance 1s\n", "one clock line"},
		{"param role below", "character A 1\nnpc a 1 Folk 0 0 0 params b 0 0\nnpc b 2 Folk 0 0 0\nadvance 1s\n", "npc line above"},
		{"param word", "character A 1\nnpc a 1 Folk 0 0 0 params x 0 0\nadvance 1s\n", "npc line above"},
		{"param count", "character A 1\nnpc a 1 Folk 0 0 0 params 1 2\nadvance 1s\n", "npc option"},
		{"ai pair", "character A 1\nnpc a 1 Folk 0 0 0 ai oops\nadvance 1s\n", "<name>=<value>"},
		{"event role", "character A 1\nevent a 3 0 0\n", "unknown role"},
		{"event id", "character A 1\nnpc a 1 Folk 0 0 0\nevent a x 0 0\n", "event id"},
		{"traced clock", "trace x.golden\ncharacter A 1\nclock 01:00\nadvance 1s\n", "traced scenario takes no"},
		{"traced event", "trace x.golden\ncharacter A 1\nnpc a 1 Folk 0 0 0\nevent a 3 0 0\n", "traced scenario takes no event step"},
		{"event argument", "character A 1\nnpc a 1 Folk 0 0 0\nevent a 3 b 0\n", "npc line above"},
	} {
		if _, err := Parse("bad.scenario", []byte(c.text)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestRendererComparesWhatAScriptDoes(t *testing.T) {
	const player, placed, spawned = int32(0x10000001), int32(0x10000002), int32(0x10000003)
	rd := newRenderer(map[int32]string{player: "player", placed: "unicorn"})
	rd.placed[placed] = true
	rd.byNPC[7] = "pure"
	info := func(oid, npcID int32) []byte {
		b := []byte{serverpackets.OpcodeNPCInfo}
		for _, v := range []int32{oid, npcID + 1000000, 0, 0} {
			b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		}
		return b
	}
	del := func(oid int32) []byte {
		return []byte{serverpackets.OpcodeDeleteObject, byte(oid), byte(oid >> 8), byte(oid >> 16), byte(oid >> 24), 1, 0, 0, 0}
	}
	social := func(oid int32) []byte {
		return []byte{serverpackets.OpcodeSocialAction, byte(oid), byte(oid >> 8), byte(oid >> 16), byte(oid >> 24), 3, 0, 0, 0}
	}
	for _, c := range []struct {
		name    string
		payload []byte
		talk    bool
		want    string
	}{
		{"placed NpcInfo", info(placed, 7), false, ""},
		{"script-spawned NpcInfo", info(spawned, 7), false, "NpcInfo obj=pure npc=7"},
		{"its DeleteObject", del(spawned), false, "DeleteObject obj=pure"},
		{"placed DeleteObject", del(placed), false, "DeleteObject obj=unicorn"},
		{"unnamed template", info(0x10000009, 8), false, ""},
		{"stranger DeleteObject", del(0x10000009), false, ""},
		{"player social", social(player), true, "SocialAction obj=player action=3"},
		{"NPC social", social(placed), false, "SocialAction obj=unicorn action=3"},
		{"NPC social while talking", social(placed), true, ""},
		{"unnamed social", social(0x10000009), false, ""},
	} {
		rd.talk = c.talk
		got, ok, err := rd.render(c.payload)
		if err != nil || ok != (c.want != "") || got != c.want {
			t.Errorf("%s: render = %q, %v, %v; want %q", c.name, got, ok, err, c.want)
		}
	}
	// A corpse's decay is not compared; a repeated NpcInfo and the order of a run of DeleteObjects do not count.
	rd.render([]byte{serverpackets.OpcodeDie, 3, 0, 0, 0x10})
	if _, ok, _ := rd.render(del(spawned)); ok {
		t.Error("the DeleteObject of a dead NPC rendered")
	}
	lines := appendRendered(nil, "NpcInfo obj=a npc=1")
	lines = appendRendered(lines, "NpcInfo obj=a npc=1")
	lines = appendRendered(lines, "DeleteObject obj=b")
	lines = appendRendered(lines, "DeleteObject obj=a")
	if got := strings.Join(lines, ","); got != "NpcInfo obj=a npc=1,DeleteObject obj=a,DeleteObject obj=b" {
		t.Errorf("appendRendered = %s", got)
	}
}
