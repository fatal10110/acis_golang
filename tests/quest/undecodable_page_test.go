package quest

import (
	"os"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestUndecodablePageShowsTheMissingNotice: a quest page file that is not
// valid UTF-8 cannot be read, so a quest event answering it shows the
// missing-page notice naming the page's path, then ActionFailed, while a
// valid page of the same quest is shown. The page is Q123's 31961-02.htm
// from the datapack, byte for byte.
func TestUndecodablePageShowsTheMissingNotice(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "quest", "Q123_TheLeaderAndTheFollower", "31961-02.htm"))
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(raw) {
		t.Fatal("Q123 31961-02.htm is valid UTF-8; the datapack changed")
	}
	w := bootDialog(t, map[string]string{
		"script/quest/" + q950 + "/31961-02.htm": string(raw),
		"script/quest/" + q950 + "/next.htm":     "<html><body>next</body></html>",
	}, nil)
	w.spawnHostile(t, "guard", guardID, "Guard", 20)
	w.offerAll(t)
	if got := w.bypass(t, w.expand("npc_{guard}_Quest")); len(got) == 0 {
		t.Fatal("guard quest window answered nothing")
	}

	for _, tc := range []struct {
		file string
		want []string
	}{
		{"31961-02.htm", []string{
			`S NpcHtmlMessage obj=guard item=0 html="<html><body>My html is missing:<br>./data/html/script/quest/` + q950 + `/31961-02.htm</body></html>"`,
			"S ActionFailed",
		}},
		{"next.htm", []string{`S NpcHtmlMessage obj=guard item=0 html="<html><body>next</body></html>\n"`, "S ActionFailed"}},
	} {
		w.offerAll(t)
		if got := w.bypass(t, "Quest "+q950+" "+tc.file); !slices.Equal(got, tc.want) {
			t.Fatalf("event %s:\n got %q\nwant %q", tc.file, got, tc.want)
		}
	}
}
