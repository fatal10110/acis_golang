package combat

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const actedNotice = "As you acted, you are no longer under spawn protection."

// rawFrame is head followed by fields, each a little-endian int32.
func rawFrame(head []byte, fields ...int32) []byte {
	out := append([]byte(nil), head...)
	for _, v := range fields {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// bootProtected boots a player back into play spawn protected. The window
// is long enough that it never runs out while a scenario runs.
func bootProtected(t *testing.T, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(time.Minute),
	}, opts...)...)
	enterAfterRestart(t, srv)
	drainUntilQuiet(t, srv.Client)
	return srv
}

// fenceWithItemList sends RequestItemList, a request that answers ItemList
// and ends spawn protection, and returns every frame read before its
// ItemList. Frames are served in order, so a frame sent ahead of the fence
// that ended the protection shows its notice among them.
func fenceWithItemList(t *testing.T, c *scriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	var before [][]byte
	for range 50 {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatal("ItemList never arrived")
		}
		if frame[0] == serverpackets.OpcodeItemList {
			return before
		}
		before = append(before, frame)
	}
	t.Fatal("no ItemList within 50 frames")
	return nil
}

// TestSpawnProtectionEndsOnReadRequests pins the requests that are read and
// then answered with nothing: each still ends spawn protection, with the
// acted notice and its UserInfo as the only frames it causes. That holds for
// requests that read and ignore their body and for list requests whose
// contents are refused once read.
func TestSpawnProtectionEndsOnReadRequests(t *testing.T) {
	t.Parallel()
	ex := func(sub uint16) []byte { return []byte{clientpackets.OpcodeExtended, byte(sub), byte(sub >> 8)} }
	for name, frame := range map[string][]byte{
		"snoop quit":                {clientpackets.OpcodeSnoopQuit, 0, 0, 0, 0},
		"pc cafe coupon":            {clientpackets.OpcodeExtended, 0x20, 0x00, 0x00, 0x00},
		"time check":                rawFrame([]byte{clientpackets.OpcodeSendTimeCheck}, 1, 2),
		"buy item without rows":     rawFrame([]byte{clientpackets.OpcodeRequestBuyItem}, 1, 1),
		"buy item naming no item":   rawFrame([]byte{clientpackets.OpcodeRequestBuyItem}, 1, 1, 0, 1),
		"sell item without rows":    rawFrame([]byte{clientpackets.OpcodeRequestSellItem}, 1, 1),
		"preview of 101 items":      rawFrame([]byte{clientpackets.OpcodeRequestPreviewItem}, 0, 1, 101),
		"deposit of no items":       rawFrame([]byte{clientpackets.OpcodeSendWarehouseDeposit}, 0),
		"withdraw without rows":     rawFrame([]byte{clientpackets.OpcodeSendWarehouseWithdraw}, 1),
		"package send of -1 items":  rawFrame([]byte{clientpackets.OpcodeRequestPackageSend}, 1, -1),
		"seed setting naming no id": rawFrame(ex(clientpackets.OpcodeRequestSetSeed), 1, 1, 0, 0, 0),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := bootProtected(t)
			srv.Client.Send(frame)
			before := fenceWithItemList(t, srv.Client)
			if len(before) != 2 {
				t.Fatalf("frames before ItemList = %x, want the acted notice and its UserInfo", before)
			}
			if text, ok := protectionText(before[0]); !ok || text != actedNotice {
				t.Fatalf("first frame = %x, want the acted notice", before[0])
			}
			assertFrameOpcode(t, before[1], serverpackets.OpcodeUserInfo, "acted UserInfo")
			drainUntilQuiet(t, srv.Client)
		})
	}
}

// TestSpawnProtectionKeptOnUnreadRequests pins the frames no request runs
// for: opcodes nothing is built for, a request that cannot be read (too
// short, a refused row count, a workshop recipe that is not loaded), and a
// warehouse deposit while warehouses are disabled. None answers anything or
// ends spawn protection, so the fence request after them is answered first
// and its own action ends the protection.
func TestSpawnProtectionKeptOnUnreadRequests(t *testing.T) {
	t.Parallel()
	frames := [][]byte{
		{clientpackets.OpcodeRequestDismissParty},
		{clientpackets.OpcodeMessengerDialog},
		{clientpackets.OpcodeRequestSkillCoolTime},
		{clientpackets.OpcodeRequestBuyItem, 0, 0, 0},
		rawFrame([]byte{clientpackets.OpcodeExtended, byte(clientpackets.OpcodeRequestSetSeed), 0}, 1, 0),
		rawFrame([]byte{clientpackets.OpcodeRequestBuyProcure}, 1, 0),
		rawFrame([]byte{clientpackets.OpcodeRequestRecipeShopListSet}, 1, 987654, 1),
	}
	for name, opts := range map[string][]gameservertest.Option{
		"warehouse enabled":  nil,
		"warehouse disabled": {gameservertest.WithWarehouseDisabled()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := bootProtected(t, opts...)
			c := srv.Client
			for _, frame := range frames {
				c.Send(frame)
			}
			if opts != nil {
				c.Send(rawFrame([]byte{clientpackets.OpcodeSendWarehouseDeposit}, 1, 1, 1))
			}
			if before := fenceWithItemList(t, c); len(before) != 0 {
				t.Fatalf("frames before ItemList = %x, want none", before)
			}
			if got := readProtectionMessage(t, c); got != actedNotice {
				t.Fatalf("protection message after the fence = %q, want %q", got, actedNotice)
			}
			drainUntilQuiet(t, c)
		})
	}
}
