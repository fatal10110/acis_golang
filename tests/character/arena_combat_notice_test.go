package character

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// arenaIndex is a zone index holding one arena over [minX, maxX].
func arenaIndex(t *testing.T, minX, maxX int) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -500, 500, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, form))
	return zones
}

// requireNoticeBeforeCompass checks frames hold system message id once and
// that it comes ahead of the compass update.
func requireNoticeBeforeCompass(t *testing.T, frames [][]byte, id int, what string) {
	t.Helper()
	notice, compass, n := -1, -1, 0
	for i, f := range frames {
		switch {
		case len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage && int(binary.LittleEndian.Uint32(f[1:5])) == id:
			n++
			notice = i
		case compass < 0 && len(f) >= 3 && f[0] == 0xfe && f[1] == 0x32 && f[2] == 0:
			compass = i
		}
	}
	if n != 1 {
		t.Fatalf("%s: %d system messages %d (system messages %v), want 1", what, n, id, systemMessageIDsIn(frames))
	}
	if compass < 0 || notice > compass {
		t.Fatalf("%s: message %d at %d, compass update at %d; want the message ahead of the compass", what, id, notice, compass)
	}
}

// TestArenaAnnouncesCombatZoneEntryAndExit pins #4002: a player walking into
// an arena is told it entered a combat zone (283) and one walking out that
// it left it (284), each once and ahead of the compass update.
func TestArenaAnnouncesCombatZoneEntryAndExit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(arenaIndex(t, 100, 500)),
	)
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	inside := location.Location{X: 300, Y: 20, Z: 30}
	outside := location.Location{X: 700, Y: 20, Z: 30}

	in := walkTo(t, srv, c, objID, spawn, inside)
	requireNoticeBeforeCompass(t, in, serverpackets.SystemMessageEnteredCombatZone, "walking in")
	if got := countID(systemMessageIDsIn(in), serverpackets.SystemMessageLeftCombatZone); got != 0 {
		t.Fatalf("walking in: %d left-combat-zone messages, want 0", got)
	}
	out := walkTo(t, srv, c, objID, inside, outside)
	requireNoticeBeforeCompass(t, out, serverpackets.SystemMessageLeftCombatZone, "walking out")
	if got := countID(systemMessageIDsIn(out), serverpackets.SystemMessageEnteredCombatZone); got != 0 {
		t.Fatalf("walking out: %d entered-combat-zone messages, want 0", got)
	}
}

// TestArenaEntryAtEnterWorld: a player logging in inside an arena is told
// it entered a combat zone where its spawn enters the zones: after the
// burst's EtcStatusUpdate and before the welcome message.
func TestArenaEntryAtEnterWorld(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(arenaIndex(t, -500, 500)),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readUntilQuiet(c)

	etc, enter, welcome, entries := -1, -1, -1, 0
	for i, f := range frames {
		switch {
		case etc < 0 && f[0] == serverpackets.OpcodeEtcStatusUpdate:
			etc = i
		case len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage:
			switch int(binary.LittleEndian.Uint32(f[1:5])) {
			case serverpackets.SystemMessageEnteredCombatZone:
				entries++
				if enter < 0 {
					enter = i
				}
			case serverpackets.SystemMessageWelcomeToLineage:
				if welcome < 0 {
					welcome = i
				}
			}
		}
	}
	if entries != 1 {
		t.Fatalf("EnterWorld sent %d entered-combat-zone messages (opcodes %x), want 1", entries, opcodes(frames))
	}
	if etc < 0 || enter <= etc || welcome <= enter {
		t.Fatalf("EtcStatusUpdate index = %d, entry message index = %d, welcome index = %d; want EtcStatusUpdate, then 283, then 34", etc, enter, welcome)
	}
}
