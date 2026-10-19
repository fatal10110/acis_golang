package character

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The system messages the shipped elf Mother Tree zones announce on entry
// and exit (enterMsgId / leaveMsgId).
const (
	msgMotherTreeEnter = 114
	msgMotherTreeLeave = 115
)

// motherTreeIndex is a zone index holding one Mother Tree zone over
// [minX, maxX] admitting race (-1: every race), with the shipped elf zones'
// entry and exit messages.
func motherTreeIndex(t *testing.T, minX, maxX, race int) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -500, 500, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	mt := zone.NewMotherTree(1, form)
	mt.EnterMessage, mt.LeaveMessage, mt.AffectedRace = msgMotherTreeEnter, msgMotherTreeLeave, race
	zones := zone.NewIndex()
	zones.Add(mt)
	return zones
}

// systemMessageIDsIn returns the ids of the system messages among frames,
// in order.
func systemMessageIDsIn(frames [][]byte) []int {
	var ids []int
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, int(binary.LittleEndian.Uint32(f[1:5])))
		}
	}
	return ids
}

// countID counts id among ids.
func countID(ids []int, id int) int {
	n := 0
	for _, v := range ids {
		if v == id {
			n++
		}
	}
	return n
}

// walkTo walks the player from `from` to `to` and returns what it saw once
// it arrived.
func walkTo(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, from, to location.Location) [][]byte {
	t.Helper()
	c.Send(encodeMoveBackwardToLocation(to, from, 1))
	expectGroundClickAck(t, c)
	if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, objID, to)
	return readUntilQuiet(c)
}

// TestMotherTreeAnnouncesEntryAndExit: a player whose race the Mother Tree
// admits hears the entry message as it walks in and the exit message as it
// walks out, once each; a player of another race hears neither.
func TestMotherTreeAnnouncesEntryAndExit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		race     int // the zone's admitted race; the player is human (0)
		affected bool
	}{
		{name: "every race", race: -1, affected: true},
		{name: "player's race", race: 0, affected: true},
		{name: "another race", race: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 1, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithZones(motherTreeIndex(t, 100, 500, tc.race)),
			)
			c := srv.Client
			enterWorld(t, c)
			drainQuiet(t, c)
			objID := srv.SoleObjectID(t)

			spawn := location.Location{X: 10, Y: 20, Z: 30}
			inside := location.Location{X: 300, Y: 20, Z: 30}
			outside := location.Location{X: 700, Y: 20, Z: 30}

			in := systemMessageIDsIn(walkTo(t, srv, c, objID, spawn, inside))
			out := systemMessageIDsIn(walkTo(t, srv, c, objID, inside, outside))

			wantIn, wantOut := 0, 0
			if tc.affected {
				wantIn, wantOut = 1, 1
			}
			if got := countID(in, msgMotherTreeEnter); got != wantIn {
				t.Errorf("walking in: %d entry messages (system messages %v), want %d", got, in, wantIn)
			}
			if got := countID(in, msgMotherTreeLeave); got != 0 {
				t.Errorf("walking in: %d exit messages (system messages %v), want 0", got, in)
			}
			if got := countID(out, msgMotherTreeLeave); got != wantOut {
				t.Errorf("walking out: %d exit messages (system messages %v), want %d", got, out, wantOut)
			}
			if got := countID(out, msgMotherTreeEnter); got != 0 {
				t.Errorf("walking out: %d entry messages (system messages %v), want 0", got, out)
			}
		})
	}
}

// TestMotherTreeEntryAtEnterWorld: a player logging in inside a Mother Tree
// hears the entry message where its spawn enters the zones: after the
// burst's EtcStatusUpdate and before the welcome message.
func TestMotherTreeEntryAtEnterWorld(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(motherTreeIndex(t, -500, 500, -1)),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readUntilQuiet(c)

	etc, enter, welcome := -1, -1, -1
	entries := 0
	for i, f := range frames {
		switch {
		case etc < 0 && f[0] == serverpackets.OpcodeEtcStatusUpdate:
			etc = i
		case len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage:
			switch int(binary.LittleEndian.Uint32(f[1:5])) {
			case msgMotherTreeEnter:
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
		t.Fatalf("EnterWorld sent %d Mother Tree entry messages (opcodes %x), want 1", entries, opcodes(frames))
	}
	if etc < 0 || enter <= etc || welcome <= enter {
		t.Fatalf("EtcStatusUpdate index = %d, entry message index = %d, welcome index = %d; want EtcStatusUpdate, then 114, then 34", etc, enter, welcome)
	}
}
