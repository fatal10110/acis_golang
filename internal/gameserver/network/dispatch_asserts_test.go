package network

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func skipInventoryRemainder(r *wire.Reader) {
	r.ReadUint16()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadInt32()
}

func skipPackageSendableRemainder(r *wire.Reader) {
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
}

type safeLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func waitForLog(t *testing.T, logs *safeLogBuffer, needle string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := logs.String()
		if strings.Contains(got, needle) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log containing %q; logs=%s", needle, logs.String())
	return ""
}
func assertTargetHPStatus(t *testing.T, frame []byte, objectID int32, maxHP, curHP int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("status opcode = %#x, want StatusUpdate (%#x)", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, objectID)
	}
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("StatusUpdate attribute count = %d, want 2", count)
	}
	if typ, val := r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.StatusMaxHP) || val != int32(maxHP) {
		t.Fatalf("StatusUpdate first attr = (%d,%d), want MAX_HP=%d", typ, val, maxHP)
	}
	if typ, val := r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.StatusCurrentHP) || val != int32(curHP) {
		t.Fatalf("StatusUpdate second attr = (%d,%d), want CUR_HP=%d", typ, val, curHP)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("StatusUpdate read: %v", err)
	}
}

// abnormalStatusEntry is one decoded AbnormalStatusUpdate icon entry.
type abnormalStatusEntry struct {
	SkillID  int32
	Level    uint16
	Duration int32
}

// readAbnormalStatusUpdateFrame asserts the next frame is AbnormalStatusUpdate
// and returns its decoded icon entries in wire order.
func readAbnormalStatusUpdateFrame(t *testing.T, c *fakeGameClient) []abnormalStatusEntry {
	t.Helper()
	frame := c.read()
	if frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("opcode = %#x, want AbnormalStatusUpdate (%#x)", frame[0], serverpackets.OpcodeAbnormalStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	count := r.ReadUint16()
	entries := make([]abnormalStatusEntry, 0, count)
	for range count {
		entries = append(entries, abnormalStatusEntry{SkillID: r.ReadInt32(), Level: r.ReadUint16(), Duration: r.ReadInt32()})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AbnormalStatusUpdate: %v", err)
	}
	return entries
}

func assertSystemMessageSkillFrame(t *testing.T, frame []byte, messageID int, skillID, level int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName {
		t.Fatalf("SystemMessage param type = %d, want skill name", typ)
	}
	if id := r.ReadInt32(); id != skillID {
		t.Fatalf("SystemMessage skill id = %d, want %d", id, skillID)
	}
	if got := r.ReadInt32(); got != level {
		t.Fatalf("SystemMessage skill level = %d, want %d", got, level)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func assertSPStatus(t *testing.T, frame []byte, objectID int32, sp int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("StatusUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("StatusUpdate count = %d, want 1", count)
	}
	if typ := r.ReadInt32(); typ != int32(serverpackets.StatusSP) {
		t.Fatalf("StatusUpdate type = %d, want SP", typ)
	}
	if got := r.ReadInt32(); got != int32(sp) {
		t.Fatalf("StatusUpdate SP = %d, want %d", got, sp)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

func assertStatusAttrs(t *testing.T, frame []byte, objectID int32, attrs []serverpackets.StatusAttribute) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("StatusUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != int32(len(attrs)) {
		t.Fatalf("StatusUpdate count = %d, want %d", count, len(attrs))
	}
	for _, attr := range attrs {
		if typ := r.ReadInt32(); typ != int32(attr.Type) {
			t.Fatalf("StatusUpdate type = %d, want %d", typ, attr.Type)
		}
		if got := r.ReadInt32(); got != int32(attr.Value) {
			t.Fatalf("StatusUpdate value = %d, want %d", got, attr.Value)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}
