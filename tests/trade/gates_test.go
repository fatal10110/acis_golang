package trade

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// testGMLevel is the shipped accessLevels.xml level 2 ("Test GM"), whose
// allowTransaction="false" forbids every player-to-player transfer.
const testGMLevel = 2

// chaoticTradeRefusal is the S1 text a karma-refused trade request answers.
const chaoticTradeRefusal = "You cannot trade in a chaotic state."

// fallbackAccessLevels copies the transaction-relevant rows of the shipped
// accessLevels.xml for runs without the datapack checkout (CI): level 0
// "User" (allowTransaction="true") and level 2 "Test GM"
// (allowTransaction="false", giveDamage="false").
// TestFallbackAccessLevelsMatchDatapack keeps the copy honest.
func fallbackAccessLevels(t *testing.T) *admin.Data {
	t.Helper()
	data, err := admin.NewData([]admin.AccessLevel{
		{Level: 0, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true, GiveDamage: true},
		{Level: testGMLevel, Name: "Test GM", NameColor: "FFFFFF", TitleColor: "FFFF77", ChildLevel: 1, AllowFixedRes: true, AllowAltG: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	return data
}

// datapackAccessLevels loads the real accessLevels.xml the server reads, or
// returns nil when the datapack is not checked out next to the module.
var datapackAccessLevels = sync.OnceValues(func() (*admin.Data, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack", "data", "xml")
	if _, err := os.Stat(filepath.Join(root, "accessLevels.xml")); err != nil {
		return nil, nil
	}
	return xmldata.LoadAdminData(root)
})

// shippedAccessLevels is the access table the gate tests boot with: the
// datapack's own accessLevels.xml when it is checked out, else the copy.
func shippedAccessLevels(t *testing.T) *admin.Data {
	t.Helper()
	data, err := datapackAccessLevels()
	if err != nil {
		t.Fatalf("load datapack access levels: %v", err)
	}
	if data != nil {
		return data
	}
	return fallbackAccessLevels(t)
}

// TestFallbackAccessLevelsMatchDatapack pins the CI copy against the shipped
// rows it stands in for, so a datapack change cannot leave the gate tests
// passing against stale data.
func TestFallbackAccessLevelsMatchDatapack(t *testing.T) {
	shipped, err := datapackAccessLevels()
	if err != nil {
		t.Fatalf("load datapack access levels: %v", err)
	}
	if shipped == nil {
		t.Skip("aCis_datapack not checked out near the module root")
	}
	fallback := fallbackAccessLevels(t)
	for _, level := range []int{0, testGMLevel} {
		want, ok := shipped.AccessLevel(level)
		if !ok {
			t.Fatalf("shipped accessLevels.xml has no level %d", level)
		}
		got, _ := fallback.AccessLevel(level)
		if got != want {
			t.Fatalf("fallback level %d = %+v, shipped %+v", level, got, want)
		}
	}
	if !shipped.Resolve(0).AllowTransaction || shipped.Resolve(testGMLevel).AllowTransaction {
		t.Fatal("shipped levels no longer split transaction rights between level 0 and the test GM level")
	}
}

// setCharacterColumn rewrites one persisted character column before the
// character enters the world; selection reloads the row fresh.
func setCharacterColumn(t *testing.T, h *traders, objID int32, column string, value int) {
	t.Helper()
	query := "UPDATE characters SET " + column + " = ? WHERE obj_Id = ?"
	if _, err := h.srv.DB.ExecContext(context.Background(), query, value, objID); err != nil {
		t.Fatalf("set %s for %d: %v", column, objID, err)
	}
}

func assertGateSilent(t *testing.T, c *testsupport.ScriptedClient, who string) {
	t.Helper()
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s received opcode %#x, want silence", who, frame[0])
	}
}

// TestTradeRequestRefusedWithoutTransactionRight pins the requester-side
// access gate: a character whose access level forbids transactions is
// answered YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT and the target hears nothing.
// The gate reads only the requester, so the user-level trader can still
// request the restricted one.
func TestTradeRequestRefusedWithoutTransactionRight(t *testing.T) {
	h := bootTraders(t, gameservertest.WithAdmin(shippedAccessLevels(t)))
	setCharacterColumn(t, h, h.firstID, "accesslevel", testGMLevel)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)
	assertGateSilent(t, h.second, "target of a refused request")

	h.second.Send(encodeTradeRequest(h.firstID))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderOne")
}

// TestAnswerTradeRequestRefusedWithoutTransactionRight pins the answer-side
// access gate: a restricted character accepting a request is answered
// YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT, no trade window opens on either side,
// and the request stays pending, so the requester is still busy.
func TestAnswerTradeRequestRefusedWithoutTransactionRight(t *testing.T) {
	h := bootTraders(t, gameservertest.WithAdmin(shippedAccessLevels(t)))
	setCharacterColumn(t, h, h.secondID, "accesslevel", testGMLevel)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	h.second.Send(encodeAnswerTradeRequest(1))
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)
	assertGateSilent(t, h.second, "refused answerer")
	assertGateSilent(t, h.first, "requester of a refused answer")

	h.first.Send(encodeTradeRequest(h.secondID))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageAlreadyTrading)
}

// TestUserAccessLevelTrades pins that the access table does not get in the
// way of ordinary characters: two user-level traders open a trade as usual.
func TestUserAccessLevelTrades(t *testing.T) {
	h := bootTraders(t, gameservertest.WithAdmin(shippedAccessLevels(t)))
	h.enterAll(t)
	h.startTrade(t)
}

// TestTradeRequestRefusedWithKarma pins the karma gate with
// KarmaPlayerCanTrade off: karma on either side refuses the request with the
// chaotic-state text to the requester, and the target hears nothing.
func TestTradeRequestRefusedWithKarma(t *testing.T) {
	for _, tc := range []struct {
		name    string
		chaotic func(h *traders) int32
	}{
		{"requester", func(h *traders) int32 { return h.firstID }},
		{"target", func(h *traders) int32 { return h.secondID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := bootTraders(t, gameservertest.WithKarmaTrade(false))
			setCharacterColumn(t, h, tc.chaotic(h), "karma", 240)
			h.enterAll(t)

			h.first.Send(encodeTradeRequest(h.secondID))
			assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageS1, chaoticTradeRefusal)
			assertGateSilent(t, h.second, "target of a karma-refused request")
		})
	}
}

// TestTradeWithKarmaAllowedByDefault pins the shipped KarmaPlayerCanTrade =
// True: a chaotic target can still be traded with.
func TestTradeWithKarmaAllowedByDefault(t *testing.T) {
	h := bootTraders(t)
	setCharacterColumn(t, h, h.secondID, "karma", 240)
	h.enterAll(t)
	h.startTrade(t)
}

// TestAcceptOutOfRangeOpensTrade pins that accepting a request has no
// distance gate: the requester walks 200 units away before the answer, both
// windows still open, and only the confirm at that distance cancels the
// trade for both players.
func TestAcceptOutOfRangeOpensTrade(t *testing.T) {
	h := bootTraders(t)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	farX := int32(spawnX + 200)
	h.first.Send(encodeMoveBackwardToLocation(farX, spawnY, spawnZ, spawnX, spawnY, spawnZ))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeMoveToLocation, "first MoveToLocation")
	waitForArrival(t, h, h.firstID, farX)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.second.Send(encodeAnswerTradeRequest(1))
	for _, who := range []struct {
		name    string
		client  *testsupport.ScriptedClient
		partner string
		id      int32
	}{
		{"first", h.first, "TraderTwo", h.secondID},
		{"second", h.second, "TraderOne", h.firstID},
	} {
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageBeginTradeWithS1, who.partner)
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeTradeStart, who.name+" TradeStart")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != who.id {
			t.Fatalf("%s TradeStart partner id = %d, want %d", who.name, got, who.id)
		}
	}

	h.second.Send(encodeTradeDone(1))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
	}{
		{"first", h.first},
		{"second", h.second},
	} {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
	}
}
