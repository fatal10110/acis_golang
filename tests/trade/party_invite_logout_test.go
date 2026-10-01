package trade

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLogoutHoldingPartyInviteCancelsTrade pins Player.deleteMe with an
// active requester: a trader who logs out while a party invitation waits
// for its answer cancels its open trade, so the partner gets SendTradeDone(0)
// and S1_CANCELED_TRADE naming the one who left. A trader's invitation
// outlives its expiry while its trade window is open, so one that timed out
// cancels the same way.
func TestLogoutHoldingPartyInviteCancelsTrade(t *testing.T) {
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
	}{
		{"pending", 0},
		{"past expiry", trade.RequestTimeout + time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var clock atomic.Int64
			clock.Store(time.Unix(1_000_000, 0).UnixNano())
			now := func() time.Time { return time.Unix(0, clock.Load()) }

			h := bootTraders(t, gameservertest.WithTradeClock(now))
			h.enterAll(t)
			inviter, _ := h.third(t, "player3", "Inviter")
			h.startTrade(t)

			w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
			w.WriteString("TraderTwo")
			w.WriteInt32(0)
			inviter.Send(w.Bytes())
			assertSystemMessageText(t, inviter.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "TraderTwo")
			assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
			clock.Add(int64(tc.elapsed))

			h.second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
			h.awaitOffline(t, h.secondID)
			frames := drainFrames(t, h.first)
			done := indexOfOpcode(frames, serverpackets.OpcodeSendTradeDone)
			if done < 0 || done+1 >= len(frames) || tradeDones(frames) != 1 {
				t.Fatalf("partner frames = %x, want one SendTradeDone then the canceled-trade message", opcodes(frames))
			}
			if got := wire.NewReader(frames[done][1:]).ReadInt32(); got != 0 {
				t.Fatalf("SendTradeDone success = %d, want 0", got)
			}
			assertSystemMessageText(t, frames[done+1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
		})
	}
}
