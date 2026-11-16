package bbs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// homeTokenPages are pages a home page token must not reach: a page outside
// the board, a page in a subfolder of the home folder, and the two shipped
// home pages the shared harness does not serve.
var homeTokenPages = map[string]string{
	"admin/secret.htm":                "SECRET",
	"CommunityBoard/secret.htm":       "BOARDROOT",
	"CommunityBoard/top/sub/page.htm": "NESTED",
	"CommunityBoard/top/events.htm":   "EVENTS",
	"CommunityBoard/top/rules.htm":    "RULES",
}

// TestBoardHomeTokenConfined pins the home page token to a file name in the
// home folder (owner-approved narrowing, #4708): every shipped home page
// still shows, while a token that is a path (a separator of either kind or a
// parent step) shows nothing, the same silence as a missing page, even when
// the page it points at is loaded.
func TestBoardHomeTokenConfined(t *testing.T) {
	p := bootPair(t,
		gameservertest.WithCommunityBoard(boardOn),
		gameservertest.WithHTMLPages(homeTokenPages),
	)
	p.enterAll(t)

	for token, want := range map[string]string{
		"index.htm":  "HOME\n",
		"news.htm":   "NEWS\n",
		"events.htm": "EVENTS\n",
		"rules.htm":  "RULES\n",
	} {
		assertPage(t, command(t, p.alice, "_bbshome;"+token), want)
	}

	for _, token := range []string{
		"../../admin/secret.htm",
		`..\..\admin\secret.htm`,
		"../secret.htm",
		"..",
		".",
		"sub/page.htm",
		`sub\page.htm`,
		"/news.htm",
		`\news.htm`,
		"./news.htm",
		"a/b",
	} {
		if frames := command(t, p.alice, "_bbshome;"+token); len(frames) != 0 {
			t.Fatalf("_bbshome;%s answer = %x, want silence", token, opcodes(frames))
		}
	}

	// The board still answers after the refused tokens.
	assertPage(t, command(t, p.alice, "_bbshome"), "HOME\n")
}
