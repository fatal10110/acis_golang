package scenario

import (
	"strings"
	"testing"
)

func TestParseReadsTheSubclassLine(t *testing.T) {
	sc, err := Parse("sub.scenario", []byte("character A 76\nclass 2\nsubclass 12 50\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Level != 76 || sc.Class != 2 || sc.Subclass != 12 || sc.SubclassLevel != 50 {
		t.Fatalf("level %d class %d subclass %d at %d", sc.Level, sc.Class, sc.Subclass, sc.SubclassLevel)
	}
	// A third occupation is a subclass too, and the base class may be the
	// default one, declared after the subclass line or not at all.
	sc, err = Parse("sub.scenario", []byte("character A 40\nsubclass 94 80\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Class != 0 || sc.Subclass != 94 || sc.SubclassLevel != 80 {
		t.Fatalf("class %d subclass %d at %d", sc.Class, sc.Subclass, sc.SubclassLevel)
	}
}

func TestParseDefaultsWithoutASubclassLine(t *testing.T) {
	sc, err := Parse("plain.scenario", []byte("character A 1\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Subclass != 0 || sc.SubclassLevel != 0 {
		t.Fatalf("subclass %d at %d", sc.Subclass, sc.SubclassLevel)
	}
}

func TestParseRejectsMalformedSubclassLines(t *testing.T) {
	for _, tc := range []struct{ name, lines, want string }{
		{"no level", "subclass 12", "one subclass line"},
		{"twice", "subclass 12 50\nsubclass 13 50", "one subclass line"},
		{"class not a number", "subclass sorcerer 50", `subclass "sorcerer"`},
		{"unknown class", "subclass 9999 50", "subclass 9999 is not a second or third occupation"},
		{"base profession", "subclass 10 50", "subclass 10 is not a second or third occupation"},
		{"first occupation", "subclass 1 50", "subclass 1 is not a second or third occupation"},
		{"level below 40", "subclass 12 39", `subclass level "39" is not 40 or more`},
		{"level not a number", "subclass 12 x", `subclass level "x" is not 40 or more`},
		{"the base class", "class 12\nsubclass 12 50", "the subclass is the base class 12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("bad.scenario", []byte("character A 40\n"+tc.lines+"\nadvance 1s\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
