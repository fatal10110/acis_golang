package scenario

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

const seeds = `character Pilgrim 40
sealowner avarice dusk
sealowner strife dawn
memo MonsterRaceTrack_Npc 30059
memo Other 7
npc ant 20079 Monster 40 0 0
npc other 20080 Monster 50 0 0
spoil ant
spoil other other
kill ant
  live 20079 0
advance 15m
  live 31112=1
`

func TestParseReadsSeedLines(t *testing.T) {
	// The last expectation is misspelled on purpose: a live line takes a space.
	_, err := Parse("seeds.scenario", []byte(seeds))
	if err == nil || !strings.Contains(err.Error(), "live <npc id> <count>") {
		t.Fatalf("err = %v, want the live line rejected", err)
	}
	sc, err := Parse("seeds.scenario", []byte(strings.Replace(seeds, "31112=1", "31112 1", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if want := [3]sevensigns.Cabal{sevensigns.Dusk, sevensigns.NoCabal, sevensigns.Dawn}; sc.SealOwners != want {
		t.Fatalf("seal owners = %v, want %v", sc.SealOwners, want)
	}
	if want := []Memo{{"MonsterRaceTrack_Npc", "30059"}, {"Other", "7"}}; !slices.Equal(sc.Memos, want) {
		t.Fatalf("memos = %+v, want %+v", sc.Memos, want)
	}
	if want := []Spoil{{"ant", false}, {"other", true}}; !slices.Equal(sc.Spoils, want) {
		t.Fatalf("spoils = %+v, want %+v", sc.Spoils, want)
	}
	if got := sc.Steps[0].Lives; !slices.Equal(got, []string{"20079=0"}) {
		t.Fatalf("lives = %q", got)
	}
	if got := sc.Steps[1].Lives; !slices.Equal(got, []string{"31112=1"}) {
		t.Fatalf("lives = %q", got)
	}
}

func TestParseRejectsMalformedSeedLines(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"sealowner one field", "sealowner avarice", "sealowner <avarice|gnosis|strife> <dawn|dusk>"},
		{"sealowner no seal", "sealowner none dawn", `seal "none" is not avarice, gnosis or strife`},
		{"sealowner no owner", "sealowner gnosis normal", `owner "normal" is not dawn or dusk`},
		{"sealowner twice", "sealowner gnosis dawn\nsealowner gnosis dusk", "seal gnosis owned twice"},
		{"memo one field", "memo Var", "memo <var> <value>"},
		{"memo twice", "memo A 1\nmemo A 2", "memo A seeded twice"},
		{"spoil unknown role", "spoil ant", `unknown role "ant"`},
		{"spoil by a third", "npc ant 20079 Monster 1 0 0\nspoil ant player", "spoil <role> [other]"},
		{"spoil twice", "npc ant 20079 Monster 1 0 0\nspoil ant\nspoil ant other", `role "ant" spoiled twice`},
		{"live one field", "npc ant 20079 Monster 1 0 0\nadvance 1s\n  live 20079", "live <npc id> <count>"},
		{"live negative", "advance 1s\n  live 20079 -1", "is not a number of 0 or more"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "character A 1\n" + tc.text
			if !strings.Contains(tc.text, "advance") {
				text += "\nadvance 1s"
			}
			_, err := Parse("bad.scenario", []byte(text+"\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsAMemoOnACreatedCharacter(t *testing.T) {
	_, err := Parse("bad.scenario", []byte("character A 1\nmemo A 1\ncreate\nenter\n"))
	if err == nil || !strings.Contains(err.Error(), "no class, subclass, clan, quest or memo line") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Parse("ok.scenario", []byte("character A 1\nsealowner avarice dawn\ncreate\nenter\n")); err != nil {
		t.Fatal(err)
	}
}
