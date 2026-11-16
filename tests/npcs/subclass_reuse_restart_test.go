package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSubclassReuseGateOutlivesARestart pins #4914: the subclass actions'
// reuse gate belongs to the connection, so a restart and a new selection
// of the character do not reset it. Without a delay the same change goes
// through.
func TestSubclassReuseGateOutlivesARestart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		delay   time.Duration
		refused bool
	}{
		{"inside the delay", time.Hour, true},
		{"no delay", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No selection reuse delay: the scenario selects the character twice.
			w := bootSubclassWorld(t, gameservertest.WithSubclassRules(true, tc.delay), gameservertest.WithReuseDelays(0, 0))
			w.addSpellhowler(t)

			w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
			drainFrames(t, w.c)
			w.c.Send(encodeRequestGameStart(0))
			if reply := w.c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
				t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
			}
			if reply := w.c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
				t.Fatalf("opcode = %#x, want CharSelected", reply[0])
			}
			enterWorld(t, w.srv, w.c)
			w.selectFolk(t, w.folk)

			frames := w.changeTo(t, 0)
			refused := len(frames) == 1 && frames[0][0] == serverpackets.OpcodeActionFailed
			if refused != tc.refused {
				t.Fatalf("change after the restart = %x, refused %v, want refused %v", opcodes(frames), refused, tc.refused)
			}
		})
	}
}
