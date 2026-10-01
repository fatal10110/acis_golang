package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// announcementsFile holds two login announcements around an automatic one
// due an hour after boot and every minute after that.
const announcementsFile = `<?xml version='1.0' encoding='utf-8'?>
<list>
	<announcement message="Welcome to the server" critical="false" auto="false" />
	<announcement message="Every minute" critical="true" auto="true" initial_delay="3600" delay="60" limit="0" />
	<announcement message="Read the rules" critical="true" />
</list>`

// assertCreatureSay requires frame to be a CreatureSay of text by objectID
// called name on channel typ.
func assertCreatureSay(t *testing.T, frame []byte, objectID, typ int32, name, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeCreatureSay, "CreatureSay")
	r := wire.NewReader(frame[1:])
	gotID, gotType, gotName, gotText := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString()
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("CreatureSay = %x: read %v, %d bytes left", frame, err, r.Remaining())
	}
	if gotID != objectID || gotType != typ || gotName != name || gotText != text {
		t.Fatalf("CreatureSay = (%d, %d, %q, %q), want (%d, %d, %q, %q)", gotID, gotType, gotName, gotText, objectID, typ, name, text)
	}
}

// The entry burst reads the login announcements, in file order and each
// under the entering player's name, right after the Seven Signs period
// message and ahead of QuestList (EnterWorld.java:208-210,
// AnnouncementData.showAnnouncements). An automatic announcement is not
// read at login.
func TestEnterWorldBurstReadsLoginAnnouncements(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAnnouncements(announcementsFile),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())

	var frames [][]byte
	for {
		frame := c.Read()
		frames = append(frames, frame)
		if frame[0] == serverpackets.OpcodeQuestList {
			break
		}
	}
	n := len(frames)
	if n < 5 {
		t.Fatalf("EnterWorld burst up to QuestList = %x, want the welcome, period and two announcements first", testsupport.FrameOpcodes(frames))
	}
	assertSystemMessageID(t, frames[n-5], serverpackets.SystemMessageWelcomeToLineage)
	assertSystemMessageID(t, frames[n-4], serverpackets.SystemMessageCompetitionPeriodBegun)
	assertCreatureSay(t, frames[n-3], 0, 10, "Newbie", "Welcome to the server")
	assertCreatureSay(t, frames[n-2], 0, 18, "Newbie", "Read the rules")
}

// An automatic announcement loaded at boot is said to every player online
// once its initial delay from boot has passed, then once per delay, by no
// one (Announcement.java, World.announceToOnlinePlayers).
func TestBootAutomaticAnnouncementRepeats(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAnnouncements(`<list><announcement message="Vote for us" auto="true" initial_delay="30" delay="20" limit="2" /></list>`),
	)
	c := srv.Client
	startInWorld(t, c)

	for i := range 2 {
		frame := c.ReadWithTimeout(time.Minute)
		if frame == nil {
			t.Fatalf("automatic announcement %d never came", i)
		}
		assertCreatureSay(t, frame, 0, 10, "", "Vote for us")
	}
	if frame := c.ReadWithTimeout(time.Minute); frame != nil {
		t.Fatalf("frame %#x after the limit, want none", frame[0])
	}
}
