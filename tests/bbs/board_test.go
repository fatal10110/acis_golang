package bbs

import (
	"testing"

	gamebbs "github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBoardOffline pins the board switched off, the shipped default: the
// board key, a board link and a board form each answer CB_OFFLINE alone.
func TestBoardOffline(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.alice.Send(encodeShowBoard())
	frames := drainFrames(t, p.alice)
	if len(frames) != 1 {
		t.Fatalf("RequestShowBoard answer = %x, want one SystemMessage", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageCommunityBoardOffline)

	for _, cmd := range []string{"_bbshome", "_maillist_0_1_0_", "_friendlist_0_", "bbs_default"} {
		frames := command(t, p.alice, cmd)
		if len(frames) != 1 {
			t.Fatalf("%s answer = %x, want one SystemMessage", cmd, opcodes(frames))
		}
		assertSystemMessage(t, frames[0], serverpackets.SystemMessageCommunityBoardOffline)
	}

	frames = write(t, p.alice, "Mail", "Send", "0", "Bobby", "Hi", "There")
	if len(frames) != 1 {
		t.Fatalf("RequestBBSwrite answer = %x, want one SystemMessage", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageCommunityBoardOffline)
	if rows := mailRows(t, p.srv); len(rows) != 0 {
		t.Fatalf("bbs_mail rows = %+v, want none", rows)
	}
}

// TestBoardHome pins the board key and the home board: RequestShowBoard
// opens the configured home command, the home index or a named home page
// shows in one part with the two unused parts carrying "null", and a home
// command naming no page or a missing page shows nothing.
func TestBoardHome(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	p.alice.Send(encodeShowBoard())
	assertPage(t, drainFrames(t, p.alice), "HOME\n")
	assertPage(t, command(t, p.alice, "_bbshome"), "HOME\n")
	assertPage(t, command(t, p.alice, "_bbshome;news.htm"), "NEWS\n")

	if frames := command(t, p.alice, "_bbshome;"); len(frames) != 0 {
		t.Fatalf("_bbshome; answer = %x, want silence", opcodes(frames))
	}
	if frames := command(t, p.alice, "_bbshome;missing.htm"); len(frames) != 0 {
		t.Fatalf("missing page answer = %x, want silence", opcodes(frames))
	}
}

// TestBoardHomeCommandConfigured opens the board key on BBSDefault.
func TestBoardHomeCommandConfigured(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(gamebbs.Config{Enabled: true, Home: "_bbshome;news.htm"}))
	p.enterAll(t)

	p.alice.Send(encodeShowBoard())
	assertPage(t, drainFrames(t, p.alice), "NEWS\n")
}

// TestBoardUnknownCommands pins the page an unknown command or form shows,
// naming it: a board-prefixed command no board takes, an unknown form url,
// a home command that names no page, and the region board's form (named by
// its first argument).
func TestBoardUnknownCommands(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	notImplemented := func(name string) string {
		return "<html><body><br><br><center>The command: " + name + " isn't implemented.</center></body></html>"
	}
	for _, cmd := range []string{"bbs_default", "_bbshomepage", "_mailbox"} {
		assertPage(t, command(t, p.alice, cmd), notImplemented(cmd))
	}
	assertPage(t, write(t, p.alice, "Unknown", "a"), notImplemented("Unknown"))
	assertPage(t, write(t, p.alice, "_bbsloc", "first", "second"), notImplemented("first"))
	assertPage(t, write(t, p.alice, "Topic", "Bogus"), notImplemented("Bogus"))
	assertPage(t, write(t, p.alice, "Mail", "Bogus"), notImplemented("Bogus"))
	assertPage(t, write(t, p.alice, "_bbsclan", "Bogus"), notImplemented("Bogus"))
	assertPage(t, write(t, p.alice, "_friend", "Bogus"), notImplemented("Bogus"))
}
