package scenario

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const makerSeedSample = `character Leader 40
npcmaker boss_maker sailren_maker max 2 spawn 29065 GrandBoss -113091 -243942 -15536 respawn 60 db sailren
npcmaker other_maker default_maker
spawnrow sailren alive -113091 -243942 -15536
started other_maker 11047 0 0

advance 1s
  live 29065 1
`

func TestParseReadsMakerSeeds(t *testing.T) {
	sc, err := Parse("makers.scenario", []byte(makerSeedSample))
	if err != nil {
		t.Fatal(err)
	}
	wantMakers := []NpcMaker{
		{Name: "boss_maker", AIType: "sailren_maker", Max: 2, Spawn: &MakerSpawn{NPC: 29065, Type: "GrandBoss", X: -113091, Y: -243942, Z: -15536, Respawn: time.Minute, DB: "sailren"}},
		{Name: "other_maker", AIType: "default_maker"},
	}
	if !reflect.DeepEqual(sc.NpcMakers, wantMakers) {
		t.Fatalf("npcmakers = %+v", sc.NpcMakers)
	}
	if want := []SpawnRow{{Name: "sailren", X: -113091, Y: -243942, Z: -15536}}; !reflect.DeepEqual(sc.SpawnRows, want) {
		t.Fatalf("spawnrows = %+v", sc.SpawnRows)
	}
	if want := []MakerEvent{{Maker: "other_maker", Event: "11047"}}; !reflect.DeepEqual(sc.Started, want) {
		t.Fatalf("started = %+v", sc.Started)
	}
	if !sc.StartsMakers() {
		t.Fatal("a maker with a spawn does not start the makers")
	}
}

func TestParseRejectsBadMakerSeeds(t *testing.T) {
	const spawn = "npcmaker m default_maker spawn 1 Monster 0 0 0 db row\n"
	tests := []struct{ name, lines, want string }{
		{"max without a spawn", "npcmaker m default_maker max 2\n", "max needs a spawn"},
		{"zero max", "npcmaker m default_maker max 0 spawn 1 Monster 0 0 0\n", "max"},
		{"unknown spawn option", "npcmaker m default_maker spawn 1 Monster 0 0 0 weight 3\n", "not respawn or db"},
		{"short spawn", "npcmaker m default_maker spawn 1 Monster 0 0\n", "npcmaker <name>"},
		{"row of no spawn", spawn + "spawnrow other alive 0 0 0\n", "no npcmaker line's spawn keeps that row"},
		{"dead without respawn", spawn + "spawnrow row dead 0 0 0\n", "no respawn ahead"},
		{"alive with respawn", spawn + "spawnrow row alive 0 0 0 respawn 5\n", "takes no respawn"},
		{"row listed twice", spawn + "spawnrow row alive 0 0 0\nspawnrow row alive 1 1 1\n", "listed twice"},
		{"row kept twice", spawn + "npcmaker n default_maker spawn 2 Monster 0 0 0 db row\n", "keep the row"},
		{"started with no spawn", "npcmaker m default_maker\nstarted m 1 0 0\n", "no maker is started"},
		{"started of no maker", spawn + "started nobody 1 0 0\n", "no npcmaker line names it"},
		{"traced spawn", "trace some.golden\n" + spawn, "traced scenario takes no npcmaker spawn"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character Leader 40\n"+tc.lines+"\nadvance 1s\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
