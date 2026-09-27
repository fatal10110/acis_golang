package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

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
