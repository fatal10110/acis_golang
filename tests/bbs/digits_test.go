package bbs

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The board managers read their numeric arguments with Integer.parseInt or
// Integer.valueOf (ClanBBSManager.java:37-43,64-85, MailBBSManager.java:78,89,
// FriendsBBSManager.java:39-97, RegionBBSManager.java:34), which read any
// Basic Multilingual Plane decimal digit: a Java probe on OpenJDK 21.0.11,
// recorded in #3091, prints Integer.parseInt("１２") and
// Integer.parseInt("١٢") as 12. A fullwidth letter is no decimal digit.

// fullwidth spells n's ASCII digits and sign as fullwidth digits.
func fullwidth(n int) string { return shiftDigits(strconv.Itoa(n), '０') }

// arabicIndic spells n's ASCII digits as Arabic-Indic digits.
func arabicIndic(n int) string { return shiftDigits(strconv.Itoa(n), '٠') }

func shiftDigits(s string, zero rune) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			r = zero + r - '0'
		}
		out = append(out, r)
	}
	return string(out)
}

// assertSameAnswer fails unless the board answers got exactly as want,
// and answers something.
func assertSameAnswer(t *testing.T, what string, got, want [][]byte) {
	t.Helper()
	if len(want) == 0 {
		t.Fatalf("%s: the ASCII command answered nothing", what)
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("%s answer = %x, want the ASCII command's %x", what, opcodes(got), opcodes(want))
	}
}

// TestBoardArgumentsReadUnicodeDigits sends the clan, mail and friend board
// commands with their numbers spelled in fullwidth and Arabic-Indic digits:
// each answers as the ASCII spelling does, and a fullwidth letter stays no
// number.
func TestBoardArgumentsReadUnicodeDigits(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), seedClan(t, false, "old notice"))
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.enterAll(t)

	clan := int(seededClanID)
	for _, c := range []struct{ ascii, unicode string }{
		{"_bbsclan;clan;1", "_bbsclan;clan;１"},
		{"_bbsclan;home;" + clanID, "_bbsclan;home;" + fullwidth(clan)},
		{"_bbsclan;mail;" + clanID, "_bbsclan;mail;" + arabicIndic(clan)},
		{"_bbsmail;inbox;1", "_bbsmail;inbox;١"},
		// No mail 999: the last list shows, where a non-number is silent.
		{"_bbsmail;view;999", "_bbsmail;view;٩٩٩"},
	} {
		want := command(t, p.alice, c.ascii)
		assertSameAnswer(t, c.unicode, command(t, p.alice, c.unicode), want)
	}

	// Picking a friend changes the page, so each spelling picks and drops
	// Bobby in turn.
	bobby := int(p.bobbyID)
	picked := command(t, p.alice, "_friend;select;"+strconv.Itoa(bobby))
	dropped := command(t, p.alice, "_friend;deselect;"+strconv.Itoa(bobby))
	assertSameAnswer(t, "fullwidth friend pick", command(t, p.alice, "_friend;select;"+fullwidth(bobby)), picked)
	assertSameAnswer(t, "arabic-indic friend drop", command(t, p.alice, "_friend;deselect;"+arabicIndic(bobby)), dropped)

	want := write(t, p.alice, "_bbsclan", "intro", clanID, "Hunting since dawn.", "x", "x")
	assertSameAnswer(t, "intro form by a fullwidth clan id", write(t, p.alice, "_bbsclan", "intro", fullwidth(clan), "Hunting since dawn.", "x", "x"), want)

	for _, cmd := range []string{"_bbsclan;clan;ａ", "_bbsmail;inbox;ａ", "_bbsmail;view;ａ"} {
		if frames := command(t, p.alice, cmd); len(frames) != 0 {
			t.Fatalf("%s answer = %x, want silence", cmd, opcodes(frames))
		}
	}
}

// TestRegionBoardReadsUnicodeDigits opens a castle page by a fullwidth and
// an Arabic-Indic castle id: each answers as the ASCII id does, and a
// fullwidth letter shows nothing.
func TestRegionBoardReadsUnicodeDigits(t *testing.T) {
	p := bootRegion(t, time.Date(2026, 10, 11, 20, 0, 0, 0, time.Local))
	p.enterAll(t)

	want := command(t, p.alice, "_bbsloc;1")
	assertSameAnswer(t, "_bbsloc;１", command(t, p.alice, "_bbsloc;１"), want)
	assertSameAnswer(t, "_bbsloc;١", command(t, p.alice, "_bbsloc;١"), want)
	if frames := command(t, p.alice, "_bbsloc;ａ"); len(frames) != 0 {
		t.Fatalf("_bbsloc;ａ answer = %x, want silence", opcodes(frames))
	}
}
