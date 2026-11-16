package character

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// clockLinkStart is a driven start whose in-game time is 05:00, the last
// night hour: the in-game clock runs six times faster than real time from
// the local midnight before its start, so 50 real minutes after midnight
// read 300 in-game minutes. Nothing here reads the wall clock.
var clockLinkStart = time.Date(2026, 1, 1, 0, 50, 0, 0, time.UTC)

const clockLinkMinuteOfDay = 5 * 60

// charSelectedGameTime decodes the in-game minute of day of a CharSelected
// frame: it follows the six base attributes and 32 reserved ints.
func charSelectedGameTime(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadString()
	r.ReadInt32()
	r.ReadString()
	for range 3 + 3 + 1 + 3 {
		r.ReadInt32()
	}
	r.ReadFloat64()
	r.ReadFloat64()
	r.ReadInt32()
	r.ReadInt64()
	for range 3 + 6 + 30 + 2 {
		r.ReadInt32()
	}
	return r.ReadInt32()
}

// selectGameTime sends the game start for the sole character and returns
// the in-game minute of day its CharSelected carries.
func selectGameTime(t *testing.T, srv *gameservertest.Server) int32 {
	t.Helper()
	srv.Client.Send(encodeRequestGameStart(0))
	srv.Client.Read() // SSQInfo
	return charSelectedGameTime(t, srv.Client.Read())
}

// bootClockLink boots one character on a driven clock at clockLinkStart.
func bootClockLink(t *testing.T) *gameservertest.Server {
	t.Helper()
	return gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithDrivenClockStart(clockLinkStart),
	)
}

// TestGameClockLinkReadsTheBootClock pins that the network link reads the
// same in-game clock the harness drives: /time and CharSelected report the
// driven start, and CrossDayNight moves both together, whatever the wall
// clock reads.
func TestGameClockLinkReadsTheBootClock(t *testing.T) {
	t.Parallel()

	timeMessage := func(t *testing.T, srv *gameservertest.Server) systemMessage {
		t.Helper()
		frames := userCommandFrames(t, srv.Client, cmdTime)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSystemMessage {
			t.Fatalf("/time = %x, want one SystemMessage", opcodes(frames))
		}
		return parseSystemMessage(t, frames[0])
	}
	wantTime := func(t *testing.T, got systemMessage, id int32, hour int32) {
		t.Helper()
		if got.id != id || !slices.Equal(got.number, []int32{hour}) || !slices.Equal(got.texts, []string{"00"}) {
			t.Fatalf("/time = %+v, want message %d at %d:00", got, id, hour)
		}
	}

	t.Run("time command", func(t *testing.T) {
		t.Parallel()
		srv := bootClockLink(t)
		enterWorld(t, srv.Client)
		drainQuiet(t, srv.Client)

		wantTime(t, timeMessage(t, srv), int32(serverpackets.SystemMessageTimeS1S2InTheNight), 5)
		if srv.CrossDayNight(t) {
			t.Fatal("CrossDayNight at 05:00 reported night falling, want sunrise")
		}
		wantTime(t, timeMessage(t, srv), int32(serverpackets.SystemMessageTimeS1S2InTheDay), 6)
	})

	t.Run("char selected before the crossing", func(t *testing.T) {
		t.Parallel()
		srv := bootClockLink(t)
		if got := selectGameTime(t, srv); got != clockLinkMinuteOfDay {
			t.Fatalf("CharSelected game time = %d, want %d", got, clockLinkMinuteOfDay)
		}
	})

	t.Run("char selected after the crossing", func(t *testing.T) {
		t.Parallel()
		srv := bootClockLink(t)
		if srv.CrossDayNight(t) {
			t.Fatal("CrossDayNight at 05:00 reported night falling, want sunrise")
		}
		if got, want := selectGameTime(t, srv), int32(6*60); got != want {
			t.Fatalf("CharSelected game time = %d, want %d", got, want)
		}
	})
}
