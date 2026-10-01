package admin

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Chat channels as the client numbers them.
const (
	sayAlliance     int32 = 9
	sayAnnouncement int32 = 10
	sayCritical     int32 = 18
)

// announcePages are the announcement panel pages //announce answers with.
var announcePages = []string{"announce.htm", "announce_list.htm"}

// pending returns the frames c received up to now.
func pending(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
}

// assertSay requires frame to be a CreatureSay of text by objectID called
// name on channel typ.
func assertSay(t *testing.T, frame []byte, objectID, typ int32, name, text string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCreatureSay {
		t.Fatalf("opcode = %#x, want CreatureSay", frame[0])
	}
	r := wire.NewReader(frame[1:])
	gotID, gotType, gotName, gotText := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString()
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("CreatureSay = %x: read %v, %d bytes left", frame, err, r.Remaining())
	}
	if gotID != objectID || gotType != typ || gotName != name || gotText != text {
		t.Fatalf("CreatureSay = (%d, %d, %q, %q), want (%d, %d, %q, %q)", gotID, gotType, gotName, gotText, objectID, typ, name, text)
	}
}

// listedAnnouncement is one announcement's row on the announcement list.
func listedAnnouncement(index, message, critical, auto string) string {
	return `<table width=260><tr><td width=240>#` + index + ` - ` + message +
		`</td><td></td></tr></table><table width=260><tr><td>Critical: ` + critical + ` | Auto: ` + auto +
		`</td><td><button value="Delete" action="bypass -h admin_announce del ` + index +
		`" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td></tr></table>`
}

// assertList requires the last page among frames to be the announcement
// list showing rows, the empty-file notice without any. Automatic
// announcements may come in between.
func assertList(t *testing.T, frames [][]byte, rows ...string) {
	t.Helper()
	var page []byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeNpcHtmlMessage {
			page = frame
		}
	}
	if page == nil {
		t.Fatalf("frames = %x, want the announcement list", testsupport.FrameOpcodes(frames))
	}
	content := "<br>" + strings.Join(rows, "")
	if len(rows) == 0 {
		content = "<br><tr><td>The XML file doesn't contain any content.</td></tr>"
	}
	// The page cache ends every page with a line break.
	template := strings.TrimSuffix(shippedAdminPages(t, "announce_list.htm")["admin/announce_list.htm"], "\n") + "\n"
	want := strings.ReplaceAll(template, "%announces%", content)
	if got := htmlBody(t, page); got != want {
		t.Fatalf("announcement list = %q, want %q", got, want)
	}
}

// assertAnnouncementFile requires the announcements.xml the server wrote
// to hold entries, in order.
func assertAnnouncementFile(t *testing.T, srv *gameservertest.Server, entries ...string) {
	t.Helper()
	raw, err := os.ReadFile(srv.AnnounceFile)
	if err != nil {
		t.Fatalf("read announcements.xml: %v", err)
	}
	want := "<?xml version='1.0' encoding='utf-8'?> \n<!-- \n" +
		"@param String message - the message to be announced \n" +
		"@param Boolean critical - type of announcement (true = critical,false = normal) \n" +
		"@param Boolean auto - when the announcement will be displayed (true = auto,false = on player login) \n" +
		"@param Integer initial_delay - time delay for the first announce (used only if auto=true;value in seconds) \n" +
		"@param Integer delay - time delay for the announces following the first announce (used only if auto=true;value in seconds) \n" +
		"@param Integer limit - limit of announces (used only if auto=true, 0 = unlimited) \n" +
		"--> \n<list> \n"
	for _, entry := range entries {
		want += entry + " \n"
	}
	want += "</list>"
	if string(raw) != want {
		t.Fatalf("announcements.xml = %q, want %q", raw, want)
	}
}

// TestAnnounceText pins //ann and //say (AdminAnnouncements.java,
// World.announceToOnlinePlayers): the text after the command, said by no
// one, reaches every player online on the announcement channel, the
// critical one for //say; with no text nothing is said.
func TestAnnounceText(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("ann Server restart in  5 minutes"))
	if len(frames) != 1 {
		t.Fatalf("//ann frames = %x, want one CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", "Server restart in  5 minutes")
	frames = pending(t, user)
	if len(frames) != 1 {
		t.Fatalf("player frames = %x, want one CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", "Server restart in  5 minutes")

	frames = exchange(t, gm, encodeBypass("admin_say  now"))
	if len(frames) != 1 {
		t.Fatalf("//say frames = %x, want one CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayCritical, "", " now")
	assertSay(t, pending(t, user)[0], 0, sayCritical, "", " now")

	for _, command := range [][]byte{encodeBuildCmd("ann"), encodeBypass("admin_say ")} {
		if frames := exchange(t, gm, command); len(frames) != 0 {
			t.Fatalf("empty announcement frames = %x, want none", testsupport.FrameOpcodes(frames))
		}
	}
	if frames := pending(t, user); len(frames) != 0 {
		t.Fatalf("player frames after empty announcements = %x, want none", testsupport.FrameOpcodes(frames))
	}
}

// TestGMChat pins //gmchat (AdminAnnouncements.java,
// AdminData.broadcastToGMs): the game master's line reaches every game
// master online, an unlisted one too, on the alliance channel under the
// speaker's name, and no other player; with nothing after the command it is
// refused.
func TestGMChat(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	other, _ := addPlayer(t, srv, "gm2", "Other", adminLevel)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, other)
	assertTexts(t, exchange(t, other, encodeBuildCmd("gmlist")), "Removed from GMList.")
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("gmchat meet at  giran"))
	if len(frames) != 1 {
		t.Fatalf("//gmchat frames = %x, want one CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], gmID, sayAlliance, "Admin", "meet at  giran")
	frames = pending(t, other)
	if len(frames) != 1 {
		t.Fatalf("unlisted GM frames = %x, want one CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], gmID, sayAlliance, "Admin", "meet at  giran")
	if frames := pending(t, user); len(frames) != 0 {
		t.Fatalf("player frames = %x, want none", testsupport.FrameOpcodes(frames))
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("gmchat")), "Invalid //gmchat message content ; can't be null or empty.")
}

// TestAnnounceManagesLoginAnnouncements pins //announce (AdminAnnouncements.java,
// AnnouncementData.java) over login announcements: add, list, all, del and
// their refusals, the index an addition takes after a deletion, and the
// announcements.xml every change rewrites.
func TestAnnounceManagesLoginAnnouncements(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertPage(t, exchange(t, gm, encodeBuildCmd("announce")), "Announcements Menu")
	assertList(t, exchange(t, gm, encodeBuildCmd("announce list")))
	assertTexts(t, exchange(t, gm, encodeBuildCmd("announce clear")), "Possible //announce parameters : <list|all|add|add_auto|del>")

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add TRUE Server  news $1")),
		listedAnnouncement("0", "Server  news $1", "true", "false"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Server  news $1" critical="true" auto="false" initial_delay="0" delay="0" limit="0" />`)

	// A message missing outright opens the panel; an empty one is refused.
	assertPage(t, exchange(t, gm, encodeBuildCmd("announce add true")), "Announcements Menu")
	frames := exchange(t, gm, encodeBypass("admin_announce add false "))
	if len(frames) != 2 {
		t.Fatalf("empty add frames = %x, want refusal then list", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Invalid //announce message content ; can't be null or empty.")
	assertList(t, frames, listedAnnouncement("0", "Server  news $1", "true", "false"))

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add yes Welcome")),
		listedAnnouncement("0", "Server  news $1", "true", "false"),
		listedAnnouncement("1", "Welcome", "false", "false"))

	// all reads every player online the login announcements under that
	// player's own name.
	frames = exchange(t, gm, encodeBuildCmd("announce all"))
	if len(frames) != 3 {
		t.Fatalf("//announce all frames = %x, want two CreatureSay then the list", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayCritical, "Admin", "Server  news $1")
	assertSay(t, frames[1], 0, sayAnnouncement, "Admin", "Welcome")
	frames = pending(t, user)
	if len(frames) != 2 {
		t.Fatalf("player frames = %x, want two CreatureSay", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayCritical, "Player", "Server  news $1")
	assertSay(t, frames[1], 0, sayAnnouncement, "Player", "Welcome")

	// An index no announcement holds, or one that does not parse, opens the
	// panel and changes nothing.
	assertPage(t, exchange(t, gm, encodeBuildCmd("announce del 5")), "Announcements Menu")
	assertPage(t, exchange(t, gm, encodeBuildCmd("announce del 0 1")), "Announcements Menu")
	assertPage(t, exchange(t, gm, encodeBuildCmd("announce del")), "Announcements Menu")

	assertList(t, exchange(t, gm, encodeBypass("admin_announce del 0")), listedAnnouncement("1", "Welcome", "false", "false"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Welcome" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`)

	// One announcement is held, so the next takes index 1 and replaces
	// "Welcome".
	assertList(t, exchange(t, gm, encodeBuildCmd("announce add false Goodbye")), listedAnnouncement("1", "Goodbye", "false", "false"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Goodbye" critical="false" auto="false" initial_delay="0" delay="0" limit="0" />`)
}

// TestAnnounceAutomatic pins add_auto (AdminAnnouncements.java,
// Announcement.java): an automatic announcement repeats to every player
// online, the first after its initial delay and each next one a delay
// later, as many times as its limit; it is no login announcement, and
// add_auto with auto off adds a login one without schedule values.
func TestAnnounceAutomatic(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, announcePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertPage(t, exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 x Tick")), "Announcements Menu")
	assertPage(t, exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 2")), "Announcements Menu")

	start := user.Now()
	frames := exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 2 Tick tock"))
	assertList(t, frames, listedAnnouncement("0", "Tick tock", "false", "true"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Tick tock" critical="false" auto="true" initial_delay="0" delay="1" limit="2" />`)

	var heard [2]time.Duration
	for i := range heard {
		frame := user.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("automatic announcement %d never came", i)
		}
		assertSay(t, frame, 0, sayAnnouncement, "", "Tick tock")
		heard[i] = user.Now().Sub(start)
	}
	if heard[0] >= time.Second || heard[1]-heard[0] < time.Second || heard[1]-heard[0] > 1100*time.Millisecond {
		t.Fatalf("automatic announcements heard after %v, want the first at once and the second a second later", heard)
	}
	if frame := user.ReadWithTimeout(1500 * time.Millisecond); frame != nil {
		t.Fatalf("frame %#x after the limit, want none", frame[0])
	}
	drain(t, gm)

	assertList(t, exchange(t, gm, encodeBuildCmd("announce add_auto true false 5 6 7 Hello")),
		listedAnnouncement("0", "Tick tock", "false", "true"),
		listedAnnouncement("1", "Hello", "true", "false"))
	assertAnnouncementFile(t, srv,
		`<announcement message="Tick tock" critical="false" auto="true" initial_delay="0" delay="1" limit="2" />`,
		`<announcement message="Hello" critical="true" auto="false" initial_delay="0" delay="0" limit="0" />`)
	frames = exchange(t, gm, encodeBuildCmd("announce all"))
	if len(frames) != 2 {
		t.Fatalf("//announce all frames = %x, want one CreatureSay then the list", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayCritical, "Admin", "Hello")

	// all_auto starts the schedules over: the limit is counted afresh.
	drain(t, user)
	assertList(t, exchange(t, gm, encodeBuildCmd("announce all_auto")),
		listedAnnouncement("0", "Tick tock", "false", "true"),
		listedAnnouncement("1", "Hello", "true", "false"))
	for i := range 2 {
		frame := user.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("restarted automatic announcement %d never came", i)
		}
		assertSay(t, frame, 0, sayAnnouncement, "", "Tick tock")
	}

	// Deleting it stops its schedule.
	assertList(t, exchange(t, gm, encodeBuildCmd("announce add_auto false true 0 1 0 Forever")),
		listedAnnouncement("0", "Tick tock", "false", "true"),
		listedAnnouncement("1", "Hello", "true", "false"),
		listedAnnouncement("2", "Forever", "false", "true"))
	assertSay(t, user.ReadWithTimeout(3*time.Second), 0, sayAnnouncement, "", "Forever")
	exchange(t, gm, encodeBuildCmd("announce del 2"))
	drain(t, user)
	if frame := user.ReadWithTimeout(1500 * time.Millisecond); frame != nil {
		t.Fatalf("frame %#x after deletion, want none", frame[0])
	}
}
