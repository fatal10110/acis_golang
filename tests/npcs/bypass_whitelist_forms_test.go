package npcs

import (
	"strconv"
	"testing"
)

// TestBypassWhitelistLinkForms pins how the links of a validated page
// become the commands the player may send back (NpcHtmlMessage.runImpl),
// through the whole dialog flow: the page is shown, then the command is
// sent. Each row shows a one-link page, then sends a command that a link
// of that page admits and one that it does not.
//
//   - "bypass " is followed by "-h" and one skipped character; whatever that
//     character is (a space or not), the command starts after it;
//   - a link without "$" admits exactly its text, trimmed;
//   - a link with "$" in its own text admits any command starting with the
//     text before it, trimmed; a "$" past the link's closing quote, in the
//     page's text, makes no prefix;
//   - a link with nothing before its "$" admits nothing here. The reference
//     keeps that empty prefix, which admits every command; the port drops
//     it on purpose, as no page of the datapack needs it and any page with
//     one would open every command to the player (owner decision, #4976).
func TestBypassWhitelistLinkForms(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	id := strconv.Itoa(int(f.ObjectID()))
	chat1 := "npc_" + id + "_Chat 1"

	for _, tc := range []struct {
		name    string
		link    string
		admit   string
		refused string
	}{
		{"exact without -h", `<a action="bypass ` + chat1 + `">x</a>`, chat1, chat1 + "0"},
		{"exact with -h", `<a action="bypass -h ` + chat1 + `">x</a>`, chat1, chat1 + "0"},
		{"-h skips one character, any character", `<a action="bypass -hZ` + chat1 + `">x</a>`, chat1, "Z" + chat1},
		{"exact text is trimmed", `<a action="bypass -h   ` + chat1 + `  ">x</a>`, chat1, " " + chat1},
		{"prefix before $", `<a action="bypass -h npc_` + id + `_Chat $n">x</a>`, chat1, "npc_" + id + "_Link merchant/info.htm"},
		{"prefix is trimmed", `<a action="bypass -h  npc_` + id + `_Chat  $n">x</a>`, chat1, "npc_" + id + "_Link merchant/info.htm"},
		{"$ after the closing quote makes no prefix", `<a action="bypass -h ` + chat1 + `">cost $5</a>`, chat1, chat1 + "0"},
		{"empty prefix is dropped", `<a action="bypass -h $n">x</a>`, "", chat1},
		{"empty prefix without -h is dropped", `<a action="bypass $n">x</a>`, "", chat1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w.srv.ShowDialogPage(t, w.player, `<html><body>`+tc.link+`</body></html>`)
			drainFrames(t, w.c)
			if tc.admit != "" {
				assertAnswer(t, w.bypass(t, tc.admit), chatWindowAnswer, f, wantChatPage(dialogPage1, f))
				w.srv.ShowDialogPage(t, w.player, `<html><body>`+tc.link+`</body></html>`)
				drainFrames(t, w.c)
			}
			assertAnswer(t, w.bypass(t, tc.refused), nil, f, "")
		})
	}
}

// TestRequestLinkHTMLRefusalsAreSilent: a link containing ".." or lacking
// ".htm" is dropped without an answer, a page of the cache or not; the
// next valid link is answered as usual.
func TestRequestLinkHTMLRefusalsAreSilent(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	for _, link := range []string{
		"../test/any.htm",
		"test/../test/any.htm",
		"test/any",
		"test/any.ht",
		"test/any.html/..",
		"",
	} {
		w.c.Send(encodeLinkHTML(link))
		if frames := drainFrames(t, w.c); len(frames) != 0 {
			t.Fatalf("link %q answered %x, want silence", link, opcodes(frames))
		}
	}
	if got := w.linkPage(t, "test/any.htm"); got != anyNpcPage+"\n" {
		t.Fatalf("link page after the refusals = %q, want %q", got, anyNpcPage)
	}
	// ".html" holds ".htm": a page of that name is looked up, and missing:
	// the notice names the link as sent.
	const missing = "<html><body>My html is missing:<br>test/any.html</body></html>"
	if got := w.linkPage(t, "test/any.html"); got != missing {
		t.Fatalf("test/any.html answered %q, want the missing-page notice %q", got, missing)
	}
}
