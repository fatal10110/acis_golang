package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// olympiadTradeRefusal is the S1 text an Olympiad-refused trade request
// answers.
const olympiadTradeRefusal = "You cannot trade during Olympiad."

// TestTradeRequestRefusedInOlympiad pins the Olympiad gate: a competitor of
// an Olympiad match on either side refuses the request with the Olympiad
// text to the requester, and the target hears nothing. The gate comes
// before the karma one: a chaotic competitor is told about the Olympiad.
func TestTradeRequestRefusedInOlympiad(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		competitor func(h *traders) int32
		karma      bool
	}{
		{"requester", func(h *traders) int32 { return h.firstID }, false},
		{"target", func(h *traders) int32 { return h.secondID }, false},
		{"chaotic requester", func(h *traders) int32 { return h.firstID }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootTraders(t, gameservertest.WithKarmaTrade(false))
			if tc.karma {
				setCharacterColumn(t, h, tc.competitor(h), "karma", 240)
			}
			h.enterAll(t)
			h.srv.SetPlayerOlympiadMode(t, tc.competitor(h), true)

			h.first.Send(encodeTradeRequest(h.secondID))
			assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageS1, olympiadTradeRefusal)
			assertSilent(t, h.second, "target of an Olympiad-refused request")
		})
	}
}

// TestTradeAfterOlympiadMatch pins that the gate lasts only for the match:
// once the competitor is out of it, the two players trade as usual.
func TestTradeAfterOlympiadMatch(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	h.srv.SetPlayerOlympiadMode(t, h.secondID, true)
	h.srv.SetPlayerOlympiadMode(t, h.secondID, false)
	h.startTrade(t)
}
