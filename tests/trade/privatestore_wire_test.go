package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Action-bar store commands.
const (
	actionStoreSell   = 10
	actionStoreBuy    = 28
	actionPackageSell = 61
)

func encodeRequestActionUse(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeAction(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(spawnX)
	w.WriteInt32(spawnY)
	w.WriteInt32(spawnZ)
	w.WriteUint8(0)
	return w.Bytes()
}

// sellRow is one SetPrivateStoreListSell or RequestPrivateStoreBuy row:
// object, count, price.
type sellRow struct{ objectID, count, price int32 }

func encodeSetPrivateStoreListSell(packaged bool, rows ...sellRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSetPrivateStoreListSell)
	w.WriteInt32(int32(wire.BoolByte(packaged)))
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.count)
		w.WriteInt32(r.price)
	}
	return w.Bytes()
}

func encodeRequestPrivateStoreBuy(storeID int32, rows ...sellRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreBuy)
	w.WriteInt32(storeID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.count)
		w.WriteInt32(r.price)
	}
	return w.Bytes()
}

// buyRow is one SetPrivateStoreListBuy row: item, enchant, count, price.
type buyRow struct{ itemID, enchant, count, price int32 }

func encodeSetPrivateStoreListBuy(rows ...buyRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSetPrivateStoreListBuy)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.itemID)
		w.WriteUint16(uint16(r.enchant))
		w.WriteUint16(0)
		w.WriteInt32(r.count)
		w.WriteInt32(r.price)
	}
	return w.Bytes()
}

// saleRow is one RequestPrivateStoreSell row: object, item, enchant, count,
// price.
type saleRow struct{ objectID, itemID, enchant, count, price int32 }

func encodeRequestPrivateStoreSell(storeID int32, rows ...saleRow) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreSell)
	w.WriteInt32(storeID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.objectID)
		w.WriteInt32(r.itemID)
		w.WriteUint16(uint16(r.enchant))
		w.WriteUint16(0)
		w.WriteInt32(r.count)
		w.WriteInt32(r.price)
	}
	return w.Bytes()
}

func encodeStoreText(opcode byte, text string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(text)
	return w.Bytes()
}

// msgParam is one decoded SystemMessage parameter: text for a text
// parameter, value for the rest.
type msgParam struct {
	kind  int32
	text  string
	value int32
}

func textP(s string) msgParam { return msgParam{kind: serverpackets.SystemMessageParamText, text: s} }

func numberP(v int32) msgParam {
	return msgParam{kind: serverpackets.SystemMessageParamNumber, value: v}
}

func itemNameP(v int32) msgParam {
	return msgParam{kind: serverpackets.SystemMessageParamItemName, value: v}
}

// assertMessage requires frame to be SystemMessage id with exactly params.
func assertMessage(t *testing.T, frame []byte, id int, params ...msgParam) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(id) {
		t.Fatalf("system message id = %d, want %d", got, id)
	}
	n := r.ReadInt32()
	got := make([]msgParam, n)
	for i := range got {
		got[i].kind = r.ReadInt32()
		if got[i].kind == serverpackets.SystemMessageParamText {
			got[i].text = r.ReadString()
		} else {
			got[i].value = r.ReadInt32()
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage %d: %v", id, err)
	}
	if len(got) != len(params) {
		t.Fatalf("message %d params = %+v, want %+v", id, got, params)
	}
	for i := range params {
		if got[i] != params[i] {
			t.Fatalf("message %d params = %+v, want %+v", id, got, params)
		}
	}
}

// framesWith returns the frames among frames carrying opcode.
func framesWith(frames [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == opcode {
			out = append(out, f)
		}
	}
	return out
}

// soleFrame requires exactly one frame of opcode among frames and returns
// it.
func soleFrame(t *testing.T, frames [][]byte, opcode byte, what string) []byte {
	t.Helper()
	got := framesWith(frames, opcode)
	if len(got) != 1 {
		t.Fatalf("%s: %d frames of %#x among %x, want 1", what, len(got), opcode, opcodeList(frames))
	}
	return got[0]
}

func opcodeList(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}

// assertOrder requires the opcodes in want to appear in frames in that
// order (other frames may sit between them).
func assertOrder(t *testing.T, frames [][]byte, what string, want ...byte) {
	t.Helper()
	next := 0
	for _, f := range frames {
		if next < len(want) && f[0] == want[next] {
			next++
		}
	}
	if next != len(want) {
		t.Fatalf("%s: frames %x do not carry %x in order", what, opcodeList(frames), want)
	}
}

// charInfoFor returns the CharInfo among frames describing objectID: its
// fifth field after the position and heading.
func charInfoFor(t *testing.T, frames [][]byte, objectID int32) []byte {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
		if r.ReadInt32() == objectID {
			return f
		}
	}
	t.Fatalf("no CharInfo for %d among %x", objectID, opcodeList(frames))
	return nil
}

// openSellStore takes the store owner through the sell command and the
// list, and returns every frame each side received.
func openSellStore(t *testing.T, h *traders, owner, other *testsupport.ScriptedClient, action int32, packaged bool, rows ...sellRow) (ownerFrames, otherFrames [][]byte) {
	t.Helper()
	owner.Send(encodeRequestActionUse(action))
	assertFrameOpcode(t, owner.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
	owner.Send(encodeSetPrivateStoreListSell(packaged, rows...))
	h.srv.Settle(t)
	return drainFrames(t, owner), drainFrames(t, other)
}

// clickStore selects the store owner and clicks it again, the interact
// that opens its window, returning what the clicker received.
func clickStore(t *testing.T, c *testsupport.ScriptedClient, ownerID int32) [][]byte {
	t.Helper()
	c.Send(encodeAction(ownerID))
	drainUntilQuiet(t, c)
	c.Send(encodeAction(ownerID))
	return drainFrames(t, c)
}
