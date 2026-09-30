package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// ReadLoginWeightRefresh consumes the carried-weight refresh a login with a
// weighted inventory receives between the EnterWorld burst's UserInfo and its
// ItemList: one StatusUpdate carrying only CUR_LOAD, then, when that load
// lands in a weight penalty band, the band's UserInfo and EtcStatusUpdate.
//
// frame is the first frame read after the burst's UserInfo. The result is the
// first frame after the refresh, which a well-formed burst makes its ItemList.
// A weightless login has no refresh, so frame comes back unchanged.
func ReadLoginWeightRefresh(tb testing.TB, c *testsupport.ScriptedClient, frame []byte) []byte {
	tb.Helper()
	if len(frame) == 0 || frame[0] != serverpackets.OpcodeStatusUpdate {
		return frame
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // object id
	if n := r.ReadInt32(); n != 1 {
		tb.Fatalf("login weight StatusUpdate carries %d attributes, want CUR_LOAD alone", n)
	}
	if typ := serverpackets.StatusType(r.ReadInt32()); typ != serverpackets.StatusCurrentLoad {
		tb.Fatalf("login weight StatusUpdate attribute = %d, want CUR_LOAD (%d)", typ, serverpackets.StatusCurrentLoad)
	}
	frame = c.Read()
	if len(frame) == 0 || frame[0] != serverpackets.OpcodeUserInfo {
		return frame
	}
	if etc := c.Read(); len(etc) == 0 || etc[0] != serverpackets.OpcodeEtcStatusUpdate {
		tb.Fatalf("login weight band UserInfo followed by %x, want EtcStatusUpdate", etc)
	}
	return c.Read()
}
