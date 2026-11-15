package quest

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestRequestLinkHTMLQuestLinkRefused: a quest event link on a page the
// player opens by name is not one the player may send: the event is
// dropped and the quest gains no state. The same link offered by a
// dialog runs.
func TestRequestLinkHTMLQuestLinkRefused(t *testing.T) {
	t.Parallel()
	pages := q001Pages(loadContractPages(t))
	pages["test/accept.htm"] = `<html><body><a action="bypass -h Quest Q001_LettersOfLove 30048-02.htm">Go</a></body></html>`
	w := bootDialog(t, pages, nil)
	w.spawnGoldenNPCs(t, 0)
	w.onCharacter(t, func(c *player.Character) { c.SetLastQuestNPC(w.npcs["darin"]) })

	w.srv.Client.Send(encodeLinkHTML("test/accept.htm"))
	if got := w.lines(t); len(got) != 1 || !strings.HasPrefix(got[0], "S NpcHtmlMessage obj=0 ") {
		t.Fatalf("link page = %q, want one window with no NPC", got)
	}
	if got := w.bypass(t, "Quest Q001_LettersOfLove 30048-02.htm"); len(got) != 0 {
		t.Fatalf("event from a link page = %q, want nothing", got)
	}
	var started bool
	w.onCharacter(t, func(c *player.Character) { started = c.Quests().State(q001) != nil })
	if started {
		t.Fatal("an event from a link page started the quest")
	}

	w.offerAll(t)
	if got := w.bypass(t, "Quest Q001_LettersOfLove 30048-02.htm"); len(got) != 2 || !strings.HasPrefix(got[0], "S NpcHtmlMessage obj=darin") {
		t.Fatalf("offered event = %q, want Darin's page", got)
	}
}

// TestScriptPageAnswerConfined: a quest answering with the event text a
// player typed shows only a page file of its own directory: a name that
// climbs out of it, or carries markup, is no answer at all, so neither a
// foreign page nor a notice echoing the markup is sent.
func TestScriptPageAnswerConfined(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{
		"script/quest/Q950_GuardQuest/next.htm":      "<html><body>next %objectId%</body></html>",
		"script/quest/Q001_LettersOfLove/secret.htm": "<html><body>secret</body></html>",
		"test/secret.htm":                            "<html><body>secret</body></html>",
	}, nil)
	w.spawnHostile(t, "guard", guardID, "Guard", 20)
	w.offerAll(t)
	w.bypass(t, w.expand("npc_{guard}_Quest"))

	for _, event := range []string{
		"../Q001_LettersOfLove/secret.htm",
		"../../../test/secret.htm",
		`next.htm"><a action="bypass -h npc_1_Chat 1">x</a>.htm`,
		"./next.htm",
		`..\..\..\test\secret.htm`,
	} {
		w.offerAll(t)
		if got := w.bypass(t, "Quest Q950_GuardQuest "+event); len(got) != 0 {
			t.Fatalf("answer %q = %q, want nothing", event, got)
		}
	}
	w.offerAll(t)
	if got, want := w.bypass(t, "Quest Q950_GuardQuest next.htm"), []string{`S NpcHtmlMessage obj=guard item=0 html="<html><body>next {guard}</body></html>\n"`, "S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("own page:\n got %q\nwant %q", got, want)
	}
}
