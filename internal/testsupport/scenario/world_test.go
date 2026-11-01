package scenario

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const worldSample = `character Payer 20
member Fallen 20 dead 300 0 -5
member Friend 30
territory isle -100 -100 400 400 -100 100
npc vervato 32104 Folk 60 0 0 maker isle
npc picker 35600 Folk 200 0 0 walks
npc both 35601 Folk 0 10 0 maker isle walks
npc wolf 20120 Monster 0 -10 0 walks maker isle
roll vervato 0
roll player 0
gauss wolf -1.5
ground potion 20 1 500 0 0
ground zariche 8190 1 450 0 0 cursed

look picker 500 1 20 8190
look picker 500 1
walk picker
  S DeleteObject obj=potion
`

// TestParseReadsWorldLines: the member, territory, roll (the character's
// among them) and ground lines (cursed among them), an npc line's walks and maker options, on a civilian
// as on a monster, and the look and walk steps.
func TestParseReadsWorldLines(t *testing.T) {
	sc, err := Parse("world.scenario", []byte(worldSample))
	if err != nil {
		t.Fatal(err)
	}
	wantMembers := []Member{{Name: "Fallen", Level: 20, Dead: true, DX: 300, DZ: -5}, {Name: "Friend", Level: 30}}
	if !slices.Equal(sc.Members, wantMembers) {
		t.Fatalf("members = %+v", sc.Members)
	}
	wantTerritories := []Territory{{Name: "isle", DX1: -100, DY1: -100, DX2: 400, DY2: 400, DZ1: -100, DZ2: 100}}
	if !slices.Equal(sc.Territories, wantTerritories) {
		t.Fatalf("territories = %+v", sc.Territories)
	}
	wantNPCs := []NPC{
		{Role: "vervato", ID: 32104, Type: "Folk", DX: 60, Maker: "isle"},
		{Role: "picker", ID: 35600, Type: "Folk", DX: 200, Walks: true},
		{Role: "both", ID: 35601, Type: "Folk", DY: 10, Walks: true, Maker: "isle"},
		{Role: "wolf", ID: 20120, Type: "Monster", DY: -10, Walks: true, Maker: "isle"},
	}
	if !reflect.DeepEqual(sc.NPCs, wantNPCs) {
		t.Fatalf("npcs = %+v", sc.NPCs)
	}
	if want := map[string]int{"vervato": 0, "player": 0}; !maps.Equal(sc.Rolls, want) {
		t.Fatalf("rolls = %v", sc.Rolls)
	}
	if want := map[string]float64{"wolf": -1.5}; !maps.Equal(sc.Gauss, want) {
		t.Fatalf("gauss = %v", sc.Gauss)
	}
	want := []Ground{{Role: "potion", Item: 20, Count: 1, DX: 500}, {Role: "zariche", Item: 8190, Count: 1, DX: 450, Cursed: true}}
	if !slices.Equal(sc.Grounds, want) {
		t.Fatalf("grounds = %+v", sc.Grounds)
	}
	if !sc.LaysCursed() {
		t.Fatal("LaysCursed = false with a cursed ground line")
	}
	var steps []string
	for _, st := range sc.Steps {
		steps = append(steps, st.String())
	}
	if want := []string{"look picker 500 1 20 8190", "look picker 500 1", "walk picker"}; !slices.Equal(steps, want) {
		t.Fatalf("steps = %q, want %q", steps, want)
	}
	l, err := lookArgs(sc.Steps[0].Args)
	if err != nil || l.radius != 500 || l.quantity != 1 || !slices.Equal(l.items, []int32{20, 8190}) {
		t.Fatalf("look args = %+v, %v", l, err)
	}
	if !slices.Equal(sc.Steps[2].Packets, []string{"DeleteObject obj=potion"}) {
		t.Fatalf("walk packets = %q", sc.Steps[2].Packets)
	}
}

func TestParseRejectsMalformedWorldLines(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"member usage", "character A 1\nmember B\nrelog\n", "member <name> <level>"},
		{"member stray word", "character A 1\nmember B 1 alive\nrelog\n", "member <name> <level>"},
		{"member level", "character A 1\nmember B 0\naction x\n", "not a number of 1 or more"},
		{"member is the character", "character A 1\nmember A 1\nadvance 1s\n", `member "A" named twice`},
		{"member names a role", "character A 1\nnpc b 1 Folk 0 0 0\nmember b 1\nadvance 1s\n", `member "b" named twice`},
		{"member relog", "character A 1\nmember B 1\nrelog\n", "takes no relog"},
		{"member restart", "character A 1\nmember B 1\nrestart\nenter\n", "takes no restart"},
		{"member create", "character A 1\nmember B 1\ncreate\nenter\n", "no member line"},
		{"member trace", "trace x.golden\ncharacter A 1\nmember B 1\nadvance 1s\n", "no member or ground line"},
		{"territory usage", "character A 1\nterritory t 0 0 1 1 0\nadvance 1s\n", "territory <name>"},
		{"territory no box", "character A 1\nterritory t 0 0 0 1 0 0\nadvance 1s\n", "is no box"},
		{"territory twice", "character A 1\nterritory t 0 0 1 1 0 0\nterritory t 0 0 1 1 0 0\nadvance 1s\n", "listed twice"},
		{"maker unknown", "character A 1\nnpc a 1 Folk 0 0 0 maker t\nadvance 1s\n", "no territory line above"},
		{"npc option", "character A 1\nnpc a 1 Folk 0 0 0 runs\nadvance 1s\n", "is not walks, maker"},
		{"npc walks twice", "character A 1\nnpc a 1 Folk 0 0 0 walks walks\nadvance 1s\n", "is not walks, maker"},
		{"roll unknown role", "character A 1\nroll a 0\nadvance 1s\n", "no NPC line above"},
		{"roll draw", "character A 1\nnpc a 1 Folk 0 0 0\nroll a -1\nadvance 1s\n", "not a number of 0 or more"},
		{"roll twice", "character A 1\nnpc a 1 Folk 0 0 0\nroll a 0\nroll a 1\nadvance 1s\n", "listed twice"},
		{"gauss unknown role", "character A 1\ngauss a 0\nadvance 1s\n", "no NPC line above"},
		{"gauss player", "character A 1\ngauss player 0\nadvance 1s\n", "no NPC line above"},
		{"gauss value", "character A 1\nnpc a 1 Folk 0 0 0\ngauss a x\nadvance 1s\n", "is not a number"},
		{"gauss infinite", "character A 1\nnpc a 1 Folk 0 0 0\ngauss a Inf\nadvance 1s\n", "is not a number"},
		{"gauss usage", "character A 1\nnpc a 1 Folk 0 0 0\ngauss a\nadvance 1s\n", "gauss <role> <value>"},
		{"gauss twice", "character A 1\nnpc a 1 Folk 0 0 0\ngauss a 0\ngauss a 1\nadvance 1s\n", "listed twice"},
		{"gauss trace", "trace x.golden\ncharacter A 1\nnpc a 1 Folk 0 0 0\ngauss a 0\nadvance 1s\n", "no gauss line"},
		{"ground usage", "character A 1\nground g 20 1 0 0\nadvance 1s\n", "ground <role>"},
		{"ground count", "character A 1\nground g 20 0 0 0 0\nadvance 1s\n", "not 1 or more"},
		{"ground names a role", "character A 1\nnpc g 1 Folk 0 0 0\nground g 20 1 0 0 0\nadvance 1s\n", `role "g" named twice`},
		{"npc names a ground", "character A 1\nground g 20 1 0 0 0\nnpc g 1 Folk 0 0 0\nadvance 1s\n", `role "g" named twice`},
		{"ground cursed count", "character A 1\nground g 8190 2 0 0 0 cursed\nadvance 1s\n", "lies as one item"},
		{"ground option", "character A 1\nground g 8190 1 0 0 0 blessed\nadvance 1s\n", "ground <role>"},
		{"ground trace", "trace x.golden\ncharacter A 1\nground g 20 1 0 0 0\nadvance 1s\n", "no member or ground line"},
		{"look usage", "character A 1\nnpc a 1 Folk 0 0 0\nlook a 500\n", "look takes 3"},
		{"look radius", "character A 1\nnpc a 1 Folk 0 0 0\nlook a 0 1\n", "must be 1 or more"},
		{"look ground role", "character A 1\nground g 20 1 0 0 0\nlook g 500 1\n", `unknown role "g"`},
		{"walk unknown role", "character A 1\nwalk a\n", `unknown role "a"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte(tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
