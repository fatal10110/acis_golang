package scenario

import (
	"strings"
	"testing"
)

func TestParseReadsTheSiegeLine(t *testing.T) {
	for _, side := range siegeSides {
		sc, err := Parse("siege.scenario", []byte("character A 20\nsiege 1 "+side+"\nclan 5\nadvance 1s\n"))
		if err != nil {
			t.Fatal(err)
		}
		if sc.Siege == nil || *sc.Siege != (Siege{Castle: 1, Side: side}) {
			t.Fatalf("siege = %+v, want castle 1 %s", sc.Siege, side)
		}
	}
	sc, err := Parse("plain.scenario", []byte("character A 1\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Siege != nil {
		t.Fatalf("siege = %+v without a siege line", sc.Siege)
	}
}

func TestParseRejectsMalformedSiegeLines(t *testing.T) {
	for _, tc := range []struct{ name, lines, want string }{
		{"no clan line", "siege 1 defender", "a siege line needs a clan line"},
		{"no side", "clan 1\nsiege 1", "one siege line"},
		{"twice", "clan 1\nsiege 1 defender\nsiege 2 defender", "one siege line"},
		{"castle not a number", "clan 1\nsiege gludio defender", `castle id "gludio"`},
		{"castle 0", "clan 1\nsiege 0 defender", `castle id "0"`},
		{"pending", "clan 1\nsiege 1 pending", `siege side "pending"`},
		{"upper case", "clan 1\nsiege 1 DEFENDER", `siege side "DEFENDER"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 20\n"+tc.lines+"\nadvance 1s\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
