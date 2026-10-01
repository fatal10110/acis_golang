package trade

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAnswerAfterRequestExpired pins AnswerTradeRequest once the request has
// timed out: Player.getActiveRequester drops the expired requester, so the
// answer, accepted or refused, takes the partner-not-found branch. The
// answering side gets SendTradeDone(0) and TARGET_IS_NOT_FOUND_IN_THE_GAME;
// the requester hears nothing, no S1_DENIED_TRADE_REQUEST and no trade. Both
// are free to trade again afterwards.
func TestAnswerAfterRequestExpired(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		response int32
	}{
		{"accept", 1},
		{"refuse", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var clock atomic.Int64
			clock.Store(time.Unix(1_000_000, 0).UnixNano())
			now := func() time.Time { return time.Unix(0, clock.Load()) }

			h := bootTraders(t, gameservertest.WithTradeClock(now))
			h.enterAll(t)
			h.sendRequest(t)

			clock.Add(int64(trade.RequestTimeout))

			h.second.Send(encodeAnswerTradeRequest(tc.response))
			frame := h.second.Read()
			assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, "SendTradeDone")
			if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
				t.Fatalf("SendTradeDone success = %d, want 0", got)
			}
			assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageTargetNotFound)
			assertSilent(t, h.second, "target after its late answer")
			assertSilent(t, h.first, "requester after the target's late answer")

			h.first.Send(encodeTradeRequest(h.secondID))
			assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest after expiry")
			assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")
		})
	}
}
