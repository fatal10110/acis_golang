package quest

import (
	"slices"
	"testing"
)

// TestUndecodablePageShowsTheMissingNotice: a quest page file that is not
// valid UTF-8 cannot be read, so a quest event answering it shows the
// missing-page notice naming the page's path, then ActionFailed, while a
// valid page of the same quest is shown. The bad page carries a stray
// Latin-1 byte.
func TestUndecodablePageShowsTheMissingNotice(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{
		"script/quest/" + q950 + "/31961-02.htm": "<html><body>Hello \x97 there</body></html>",
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
