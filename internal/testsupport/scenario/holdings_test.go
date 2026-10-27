package scenario

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

const holdings = `character Ally 75
hold 7213 1
hold 57 5000
equip 1
standing 3
pk 12
cabal dusk gnosis
festival 0 100
tick
tick x4
`

func TestParseReadsHoldingLines(t *testing.T) {
	sc, err := Parse("holdings.scenario", []byte(holdings))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Hold{{Item: 7213, Count: 1}, {Item: 57, Count: 5000}}; !slices.Equal(sc.Holds, want) {
		t.Fatalf("holds = %+v, want %+v", sc.Holds, want)
	}
	if !slices.Equal(sc.Equips, []int32{1}) || sc.Standing != 3 || sc.PK != 12 {
		t.Fatalf("equips %v standing %d pk %d", sc.Equips, sc.Standing, sc.PK)
	}
	if sc.Cabal != sevensigns.Dusk || sc.Seal != sevensigns.Gnosis {
		t.Fatalf("cabal %v seal %v", sc.Cabal, sc.Seal)
	}
	if sc.Festival == nil || *sc.Festival != [2]int{0, 100} {
		t.Fatalf("festival = %v", sc.Festival)
	}
	var steps []string
	for _, st := range sc.Steps {
		steps = append(steps, st.String())
	}
	if want := []string{"tick", "tick x4"}; !slices.Equal(steps, want) {
		t.Fatalf("steps = %q, want %q", steps, want)
	}
}

func TestParseReadsAVarkaStanding(t *testing.T) {
	sc, err := Parse("varka.scenario", []byte("character A 75\nstanding -5\ncabal dawn strife\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Standing != -5 || sc.Cabal != sevensigns.Dawn || sc.Seal != sevensigns.Strife || sc.Festival != nil {
		t.Fatalf("standing %d cabal %v seal %v festival %v", sc.Standing, sc.Cabal, sc.Seal, sc.Festival)
	}
}

func TestParseRejectsMalformedHoldingLines(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"hold without count", "hold 57", "hold <item id> <count>"},
		{"hold zero", "hold 57 0", `hold count "0" is not a number of 1 or more`},
		{"hold not a number", "hold gold 1", `hold item "gold"`},
		{"hold twice", "hold 57 1\nhold 57 2", "item 57 held twice"},
		{"equip without id", "equip", "equip <item id>"},
		{"equip twice", "equip 1\nequip 1", "item 1 worn twice"},
		{"standing zero", "standing 0", "is not 1 to 5 (Ketra) or -1 to -5 (Varka)"},
		{"standing too high", "standing 6", "is not 1 to 5"},
		{"standing too low", "standing -6", "is not 1 to 5"},
		{"standing twice", "standing 1\nstanding 2", "one standing line"},
		{"pk zero", "pk 0", "is not a number of 1 or more"},
		{"pk twice", "pk 1\npk 2", "one pk line"},
		{"cabal without seal", "cabal dawn", "one cabal line"},
		{"cabal none", "cabal normal gnosis", `cabal "normal" is not dawn or dusk`},
		{"seal none", "cabal dawn none", `seal "none" is not avarice, gnosis or strife`},
		{"cabal twice", "cabal dawn gnosis\ncabal dusk gnosis", "one cabal line"},
		{"festival one score", "festival 100", "one festival line"},
		{"festival negative", "festival -1 0", "is not a number of 0 or more"},
		{"festival twice", "festival 1 0\nfestival 0 1", "one festival line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 1\n"+tc.line+"\nadvance 1s\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsHoldingsOnACreatedCharacter(t *testing.T) {
	for _, line := range []string{"hold 57 1", "equip 1", "standing 1", "pk 1", "noble", "cabal dawn gnosis"} {
		t.Run(line, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 1\n"+line+"\ncreate\nenter\n"))
			if err == nil || !strings.Contains(err.Error(), "no hold, equip, standing, pk, noble or cabal line") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	// The festival scores are the server's, not the character's.
	if _, err := Parse("ok.scenario", []byte("character A 1\nfestival 100 0\ncreate\nenter\n")); err != nil {
		t.Fatal(err)
	}
}

func TestParseRejectsABadTickCount(t *testing.T) {
	for _, text := range []string{"character A 1\ntick 3\n", "character A 1\ntick x0\n", "character A 1\ntick x1 x2\n"} {
		if _, err := Parse("bad.scenario", []byte(text)); err == nil {
			t.Errorf("Parse(%q) accepted a bad tick", text)
		}
	}
}
